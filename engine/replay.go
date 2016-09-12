// Copyright © 2016 Sidharth Kshatriya
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package engine

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/chzyer/readline"
	"github.com/cyrus-and/gdb"
	"github.com/fatih/color"
	"github.com/kr/pty"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	numFilesSentinel      = "//&&& Number of Files:"
	maxStackDepthSentinel = "//&&& Max Stack Depth:"
	phpFilenameSentinel   = "//###"
	levelSentinel         = "//$$$"

	// @TODO improve this
	gHelpText = `
h        display this help text
q        quit
r        debug in reverse mode
f        debug in forward (normal) mode
t        toggle between reverse and forward modes
v        toggle between verbose and quiet modes
n        toggle between showing and not showing gdb notifications
<enter>  will tell you whether you are in forward or reverse mode

Debugging in reverse mode can be confusing but here is a cheat sheet:
The buttons in your PHP IDE debugger will have the following new (and opposite) meanings in reverse debugging mode:

         step-into     becomes: step-into a php statement in the reverse direction

         step-over     becomes: step-over one php statement backwards. As usual, stop if you encounter
                                a breakpoint while doing this operation.

         step-out      becomes: run backwards until you come out of the current function and are about to enter it.
                                As usual, stop if you encounter a breakpoint while doing this operation.

         run/continue  becomes: run backwards until you hit a breakpoint

         run to cursor becomes: run backwards until you hit the cursor (need to place cursor before current line)

Expert Usage:
* For commands to be sent to GDB-MI prefix command with "-" e.g. -thread-info
* For dbgp commands to be sent to PHP, prefix command with "#" e.g. #stack_get -i 0
  Note: only a subset of dbgp commands may issued in this way.
`
)

func getTraceDirFromSnapshotName(snapshotTagnamePortion string) (string, string) {
	currentUser, err := user.Current()
	fatalIf(err)

	rrTraceDir := currentUser.HomeDir + "/.local/share/rr"
	snapshotDirsGlob := fmt.Sprintf("%v/*/*%v*", rrTraceDir, snapshotTagnamePortion)
	matches, err := filepath.Glob(snapshotDirsGlob)
	fatalIf(err)

	whitledMatches := make([]string, 0, 10)
	for _, match := range matches {
		if strings.Contains(match, "latest-trace") {
			continue
		}

		if strings.HasPrefix(filepath.Base(match), "dontbug-snapshot") {
			whitledMatches = append(whitledMatches, match)
		}
	}

	if len(whitledMatches) == 0 {
		log.Fatalf("Could not find %v", snapshotTagnamePortion)
	} else if len(whitledMatches) > 1 {
		log.Fatalf("Multiple matching snapshots found: %v", whitledMatches)
	}

	traceDir, snapshotTagname := path.Dir(whitledMatches[0]), filepath.Base(whitledMatches[0])
	return traceDir, snapshotTagname
}

func DoReplay(extDir, snapshotTagnamePortion, rrPath, gdbPath string, replayPort int, targetExtendedRemotePort int) {
	bpMap, levelAr, maxStackDepth := constructBreakpointLocMap(extDir)
	traceDir := ""
	if snapshotTagnamePortion != "" {
		var snapshotTagname string
		traceDir, snapshotTagname = getTraceDirFromSnapshotName(snapshotTagnamePortion)
		color.Green("dontbug: Found tag %v corresponding to %v", snapshotTagname, traceDir)
	}

	engineState := startReplayInRR(
		traceDir,
		rrPath,
		gdbPath,
		bpMap,
		levelAr,
		maxStackDepth,
		targetExtendedRemotePort,
	)
	debuggerLoop(engineState, replayPort)
}

func startReplayInRR(traceDir string, rrPath, gdbPath string, bpMap map[string]int, levelAr []int, maxStackDepth int, targetExtendedRemotePort int) *engineState {

	rrCmdAr := []string{
		rrPath,
		"replay",
		"-s", strconv.Itoa(targetExtendedRemotePort),
		traceDir,
	}

	// Start an rr replay session
	replayCmd := exec.Command(rrCmdAr[0], rrCmdAr[1:]...)

	Verbosef("dontbug: Issuing command: %v\n", strings.Join(rrCmdAr, " "))

	f, err := pty.Start(replayCmd)
	fatalIf(err)
	color.Green("dontbug: Successfully started replay session")

	// Abort if we are not able to get the gdb connection string within 5 sec
	cancel := make(chan bool, 1)
	go func() {
		time.Sleep(5 * time.Second)
		select {
		case <-cancel:
			return
		default:
			log.Fatal("Could not find gdb connection string that is given by rr")
		}
	}()

	// Get hardlink filename which will be needed for gdb debugging
	buf := bufio.NewReader(f)
	for {
		line, err := buf.ReadString('\n')
		if strings.Contains(line, "target extended-remote") {
			cancel <- true
			close(cancel)
			fmt.Print(line)

			go io.Copy(os.Stdout, f)
			slashAt := strings.Index(line, "/")

			hardlinkFile := strings.TrimSpace(line[slashAt:])
			return startGdbAndInitDebugEngineState(gdbPath, hardlinkFile, bpMap, levelAr, maxStackDepth, f, replayCmd)
		}

		if err != nil {
			log.Fatal("Could not find gdb connection string that is given by rr")
		}

		fmt.Print(line)
	}
}

// Starts gdb and creates a new DebugEngineState object
func startGdbAndInitDebugEngineState(gdb_executable string, hardlinkFile string, bpMap map[string]int, levelAr []int, maxStackDepth int, rrFile *os.File, rrCmd *exec.Cmd) *engineState {
	// @TODO what if 9999 is occupied?
	gdbArgs := []string{
		gdb_executable,
		"-l", "-1",
		"-ex", "target extended-remote :9999",
		"--interpreter", "mi",
		hardlinkFile,
	}

	Verboseln("dontbug: Issuing command: ", strings.Join(gdbArgs, " "))

	var gdbSession *gdb.Gdb
	var err error

	stopEventChan := make(chan string)
	started := false

	gdbSession, err = gdb.NewCmd(gdbArgs,
		func(notification map[string]interface{}) {
			if ShowGdbNotifications {
				jsonResult, err := json.MarshalIndent(notification, "", "  ")
				fatalIf(err)
				fmt.Println(string(jsonResult))
			}

			id, ok := breakpointStopGetId(notification)
			if ok {
				// Don't send the very first stopped notification
				if started {
					stopEventChan <- id
				}

				started = true
			}
		})

	fatalIf(err)

	go io.Copy(os.Stdout, gdbSession)

	// This is our usual steppping breakpoint. Initially disabled.
	miArgs := fmt.Sprintf("-f -d --source dontbug.c --line %v", dontbugCstepLineNum)
	result := sendGdbCommand(gdbSession, "break-insert", miArgs)

	// Note that this is a temporary breakpoint, just to get things started
	miArgs = fmt.Sprintf("-t -f --source dontbug.c --line %v", dontbugCstepLineNumTemp)
	sendGdbCommand(gdbSession, "break-insert", miArgs)

	// Unlimited print length in gdb so that results from gdb are not "chopped" off
	sendGdbCommand(gdbSession, "gdb-set", "print elements 0")

	// Should break on line: dontbugCstepLineNumTemp of dontbug.c
	sendGdbCommand(gdbSession, "exec-continue")

	result = sendGdbCommand(gdbSession, "data-evaluate-expression", "filename")
	payload := result["payload"].(map[string]interface{})
	filename := payload["value"].(string)
	properFilename, err := parseGdbStringResponse(filename)
	fatalIf(err)

	es := &engineState{
		gdbSession:      gdbSession,
		breakStopNotify: stopEventChan,
		featureMap:      initFeatureMap(),
		entryFilePHP:    properFilename,
		status:          statusStarting,
		reason:          reasonOk,
		sourceMap:       bpMap,
		lastSequenceNum: 0,
		levelAr:         levelAr,
		rrCmd:           rrCmd,
		maxStackDepth:   maxStackDepth,
		breakpoints:     make(map[string]*engineBreakPoint, 10),
		rrFile:          rrFile,
	}

	// "1" is always the first breakpoint number in gdb
	// Its used for stepping
	es.breakpoints["1"] = &engineBreakPoint{
		id:        "1",
		lineno:    dontbugCstepLineNum,
		filename:  "dontbug.c",
		state:     breakpointStateDisabled,
		temporary: false,
		bpType:    breakpointTypeInternal,
	}

	return es
}

func debuggerLoop(es *engineState, replayPort int) {
	defer func() {
		es.rrFile.Close()
		err := es.rrCmd.Wait()
		fatalIf(err)
	}()
	defer es.gdbSession.Exit()

	reverse := false
	mutex := &sync.Mutex{}
	closeConChan := make(chan bool, 1)
	defer func() {
		closeConChan <- true
	}()
	go debuggerIdeLoop(es, closeConChan, mutex, &reverse, replayPort)

	fmt.Print("(dontbug) ") // prompt
	currentUser, err := user.Current()
	fatalIf(err)

	historyFile := currentUser.HomeDir + "/.dontbug.history"
	rdline, err := readline.NewEx(
		&readline.Config{
			Prompt:      "(dontbug) ",
			HistoryFile: historyFile,
		})

	fatalIf(err)
	defer rdline.Close()

	color.Yellow("h <enter> for help")
	for {
		userResponse, err := rdline.Readline()
		if err == io.EOF || err == readline.ErrInterrupt {
			color.Yellow("Exiting.")
			return
		} else if err != nil {
			log.Fatal(err)
		}

		if strings.HasPrefix(userResponse, "t") {
			mutex.Lock()
			reverse = !reverse
			mutex.Unlock()
			if reverse {
				color.Red("In reverse mode")
			} else {
				color.Green("In forward mode")
			}
		} else if strings.HasPrefix(userResponse, "r") {
			mutex.Lock()
			reverse = true
			mutex.Unlock()
			color.Red("In reverse mode")
		} else if strings.HasPrefix(userResponse, "f") {
			mutex.Lock()
			reverse = false
			mutex.Unlock()
			color.Green("In forward mode")
		} else if strings.HasPrefix(userResponse, "-") {
			command := strings.TrimSpace(userResponse[1:])
			result := sendGdbCommand(es.gdbSession, command)

			jsonResult, err := json.MarshalIndent(result, "", "  ")
			fatalIf(err)

			fmt.Println(string(jsonResult))
		} else if strings.HasPrefix(userResponse, "v") {
			VerboseFlag = !VerboseFlag
			if VerboseFlag {
				color.Red("Verbose mode")
			} else {
				color.Green("Quiet mode")
			}
		} else if strings.HasPrefix(userResponse, "n") {
			ShowGdbNotifications = !ShowGdbNotifications
			if ShowGdbNotifications {
				color.Red("Will show gdb notifications")
			} else {
				color.Green("Wont show gdb notifications")
			}
		} else if strings.HasPrefix(userResponse, "#") {
			command := strings.TrimSpace(userResponse[1:])

			// @TODO blacklist commands that are handled in gdb or dontbug instead
			xmlResult := recoverableDiversionSessionCmd(es, command)
			fmt.Println(xmlResult)
		} else if strings.HasPrefix(userResponse, "q") {
			color.Yellow("Exiting.")
			return
		} else if strings.HasPrefix(userResponse, "h") {
			fmt.Println(gHelpText)
		} else {
			if reverse {
				color.Red("In reverse mode")
			} else {
				color.Green("In forward mode")
			}
		}
	}
}

func debuggerIdeLoop(es *engineState, closeConnChan chan bool, mutex *sync.Mutex, reverse *bool, replayPort int) {
	color.Yellow("dontbug: Trying to connect to debugger IDE")
	conn, err := net.Dial("tcp", fmt.Sprintf(":%v", replayPort))
	if err != nil {
		log.Fatalf("%v: Is your IDE listening for debugging connections from PHP?", err)
	}
	es.ideConnection = conn
	defer func() {
		Verboseln("dontbug: Closing TCP connection to IDE")
		conn.Close()
		es.ideConnection = nil
		fmt.Print("(dontbug) ")
	}()

	// send the init packet
	payload := fmt.Sprintf(gInitXmlResponseFormat, es.entryFilePHP, os.Getpid())
	packet := constructDbgpPacket(payload)
	_, err = conn.Write(packet)
	fatalIf(err)

	color.Green("dontbug: Connected to debugger IDE (aka \"client\")")
	buf := bufio.NewReader(conn)

	go func() {
		defer func() {
			r := recover()
			if r != nil {
				fmt.Println(r)
				fmt.Println("Recovering from panic....")
				color.Yellow("dontbug: Initiating shutdown of IDE connection. The dontbug prompt will be still operable")
			}
			closeConnChan <- true
		}()

		for es.status != statusStopped {
			command, err := buf.ReadString(byte(0))
			command = strings.TrimRight(command, "\x00")
			if err == io.EOF {
				Verboseln("dontbug: EOF Received on tcp connection to IDE")
				break
			} else if err != nil {
				Verboseln("dontbug: IDE TCP connection was terminated")
				break
			}

			if VerboseFlag {
				color.Cyan("\nide -> dontbug: %v", command)
			}

			mutex.Lock()
			reverseVal := *reverse
			mutex.Unlock()

			payload = dispatchIdeRequest(es, command, reverseVal)
			conn.Write(constructDbgpPacket(payload))

			if VerboseFlag {
				continued := ""
				if len(payload) > 300 {
					continued = "..."
				}
				color.Green("dontbug -> ide:\n%.300v%v", payload, continued)
				fmt.Print("(dontbug) ")
			}
		}
	}()
	<-closeConnChan
}

func dispatchIdeRequest(es *engineState, command string, reverse bool) string {
	dbgpCmd := parseCommand(command)
	if es.lastSequenceNum > dbgpCmd.seqNum {
		panicIf(errors.New(fmt.Sprint("Sequence number", dbgpCmd.seqNum, "has already been seen")))
	}

	es.lastSequenceNum = dbgpCmd.seqNum
	switch dbgpCmd.command {
	case "feature_set":
		return handleFeatureSet(es, dbgpCmd)
	case "status":
		return handleStatus(es, dbgpCmd)
	case "breakpoint_set":
		return handleBreakpointSet(es, dbgpCmd)
	case "breakpoint_remove":
		return handleBreakpointRemove(es, dbgpCmd)
	case "breakpoint_update":
		return handleBreakpointUpdate(es, dbgpCmd)
	case "step_into":
		return handleStepInto(es, dbgpCmd, reverse)
	case "step_over":
		return handleStepOverOrOut(es, dbgpCmd, reverse, false)
	case "step_out":
		return handleStepOverOrOut(es, dbgpCmd, reverse, true)
	case "eval":
		return handleInDiversionSessionWithNoGdbBpts(es, dbgpCmd)
	case "stdout":
		return handleStdFd(es, dbgpCmd, "stdout")
	case "stdin":
		return handleStdFd(es, dbgpCmd, "stdin")
	case "stderr":
		return handleStdFd(es, dbgpCmd, "stderr")
	case "property_set":
		return handlePropertySet(es, dbgpCmd)
	case "property_get":
		return handleInDiversionSessionWithNoGdbBpts(es, dbgpCmd)
	case "context_get":
		return handleInDiversionSessionWithNoGdbBpts(es, dbgpCmd)
	case "run":
		return handleRun(es, dbgpCmd, reverse)
	case "stop":
		color.Yellow("IDE sent 'stop' command")
		return handleStop(es, dbgpCmd)
	// All these are dealt with in handleInDiversionSessionStandard()
	case "stack_get":
		return handleInDiversionSessionStandard(es, dbgpCmd)
	case "stack_depth":
		return handleInDiversionSessionStandard(es, dbgpCmd)
	case "context_names":
		return handleInDiversionSessionStandard(es, dbgpCmd)
	case "typemap_get":
		return handleInDiversionSessionStandard(es, dbgpCmd)
	case "source":
		return handleInDiversionSessionStandard(es, dbgpCmd)
	case "property_value":
		return handleInDiversionSessionStandard(es, dbgpCmd)
	default:
		es.sourceMap = nil // Just to reduce size of map dump to stdout
		fmt.Println(es)
		panicIf(errors.New(fmt.Sprint("Unimplemented command:", command)))
	}

	return ""
}

func constructBreakpointLocMap(extensionDir string) (map[string]int, []int, int) {
	absExtDir := getAbsPathOrFatal(extensionDir)
	dontbugBreakFilename := absExtDir + "/dontbug_break.c"
	Verboseln("dontbug: Looking for dontbug_break.c in", absExtDir)

	file, err := os.Open(dontbugBreakFilename)
	fatalIf(err)
	defer file.Close()

	Verboseln("dontbug: Found", dontbugBreakFilename)
	bpLocMap := make(map[string]int, 1000)
	buf := bufio.NewReader(file)

	level := 0
	lineno := 0

	line, err := buf.ReadString('\n')
	lineno++
	fatalIf(err)

	indexNumFiles := strings.Index(line, numFilesSentinel)
	if indexNumFiles == -1 {
		log.Fatal("Could not find the marker: ", numFilesSentinel)
	}
	numFiles, err := strconv.Atoi(strings.TrimSpace(line[indexNumFiles+len(numFilesSentinel):]))
	fatalIf(err)

	line, err = buf.ReadString('\n')
	lineno++
	fatalIf(err)

	indexMaxStackDepth := strings.Index(line, maxStackDepthSentinel)
	if indexMaxStackDepth == -1 {
		log.Fatal("Could not find the marker: ", maxStackDepthSentinel)
	}
	maxStackDepth, err := strconv.Atoi(strings.TrimSpace(line[indexMaxStackDepth+len(maxStackDepthSentinel):]))
	fatalIf(err)

	levelLocAr := make([]int, maxStackDepth)

	for {
		line, err := buf.ReadString('\n')
		lineno++
		if err == io.EOF {
			break
		} else if err != nil {
			log.Fatal(err)
		}

		indexB := strings.Index(line, phpFilenameSentinel)
		indexL := strings.Index(line, levelSentinel)
		if indexB != -1 {
			filename := strings.TrimSpace("file://" + line[indexB+dontbugCpathStartsAt:])
			_, ok := bpLocMap[filename]
			if ok {
				log.Fatal("dontbug: Sanity check failed. Duplicate entry for filename: ", filename)
			}
			bpLocMap[filename] = lineno
		}

		if indexL != -1 {
			levelLocAr[level] = lineno
			level++
		}
	}

	if len(bpLocMap) != numFiles {
		log.Fatal("dontbug: Consistency check failed. dontbug_break.c file says ", numFiles, " files. However ", len(bpLocMap), " files were found")
	}

	Verboseln("dontbug: Completed building association of filename => linenumbers and levels => linenumbers for breakpoints")
	return bpLocMap, levelLocAr, maxStackDepth
}
