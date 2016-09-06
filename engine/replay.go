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
	"os/exec"
	"fmt"
	"github.com/kr/pty"
	"log"
	"time"
	"bufio"
	"strings"
	"io"
	"os"
	"github.com/fatih/color"
	"github.com/cyrus-and/gdb"
	"encoding/json"
	"net"
	"strconv"
)

const (
	numFilesSentinel = "//&&& Number of Files:"
	maxStackDepthSentinel = "//&&& Max Stack Depth:"
	phpFilenameSentinel = "//###"
	levelSentinel = "//$$$"
)

func DoReplay(extDir, traceDir, rr_executable, gdb_executable string, replayPort int, targetExtendedRemotePort int) {
	bpMap, levelAr, maxStackDepth := constructBreakpointLocMap(extDir)
	engineState := startReplayInRR(traceDir, rr_executable, gdb_executable, bpMap, levelAr, maxStackDepth, targetExtendedRemotePort)
	debuggerIdeCmdLoop(engineState, replayPort)
	engineState.rrCmd.Wait()
}

func startReplayInRR(traceDir string, rr_executable, gdb_executable string, bpMap map[string]int, levelAr []int, maxStackDepth int, targetExtendedRemotePort int) *engineState {
	if rr_executable != "rr" {
		_, err := os.Stat(rr_executable)
		if err != nil {
			log.Fatalf("Could not find rr executable. Error: %v", err)
		}
	}

	if gdb_executable != "gdb" {
		_, err := os.Stat(gdb_executable)
		if err != nil {
			log.Fatalf("Could not find gdb executable. Error: %v", err)
		}
	}

	absTraceDir := ""
	if len(traceDir) > 0 {
		absTraceDir = getDirAbsPath(traceDir)
	}

	// Start an rr replay session
	replayCmd := exec.Command(rr_executable, "replay", "-s", strconv.Itoa(targetExtendedRemotePort), absTraceDir)
	fmt.Println("dontbug: Using rr at:", replayCmd.Path)
	f, err := pty.Start(replayCmd)
	if err != nil {
		log.Fatal(err)
	}
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
			return startGdbAndInitDebugEngineState(gdb_executable, hardlinkFile, bpMap, levelAr, maxStackDepth, f, replayCmd)
		}

		if err != nil {
			log.Fatal("Could not find gdb connection string that is given by rr")
		}

		fmt.Print(line)
	}

	return nil
}

// Starts gdb and creates a new DebugEngineState object
func startGdbAndInitDebugEngineState(gdb_executable string, hardlinkFile string, bpMap map[string]int, levelAr []int, maxStackDepth int, rrFile *os.File, rrCmd *exec.Cmd) *engineState {
	gdbArgs := []string{gdb_executable, "-l", "-1", "-ex", "target extended-remote :9999", "--interpreter", "mi", hardlinkFile}
	fmt.Println("dontbug: Starting gdb with the following string:", strings.Join(gdbArgs, " "))

	var gdbSession *gdb.Gdb
	var err error

	stopEventChan := make(chan string)
	started := false

	gdbSession, err = gdb.NewCmd(gdbArgs,
		func(notification map[string]interface{}) {
			if ShowGdbNotifications {
				jsonResult, err := json.MarshalIndent(notification, "", "  ")
				if err != nil {
					log.Fatal(err)
				}
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

	if err != nil {
		log.Fatal(err)
	}

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

	if err != nil {
		log.Fatal(properFilename)
	}

	es := &engineState{
		gdbSession: gdbSession,
		breakStopNotify: stopEventChan,
		featureMap:initFeatureMap(),
		entryFilePHP:properFilename,
		status:statusStarting,
		reason:reasonOk,
		sourceMap:bpMap,
		lastSequenceNum:0,
		levelAr:levelAr,
		rrCmd: rrCmd,
		maxStackDepth:maxStackDepth,
		breakpoints:make(map[string]*engineBreakPoint, 10),
		rrFile:rrFile,
	}

	// "1" is always the first breakpoint number in gdb
	// Its used for stepping
	es.breakpoints["1"] = &engineBreakPoint{
		id:"1",
		lineno:dontbugCstepLineNum,
		filename:"dontbug.c",
		state:breakpointStateDisabled,
		temporary:false,
		bpType:breakpointTypeInternal,
	}

	return es
}

func debuggerIdeCmdLoop(es *engineState, replayPort int) {
	color.Yellow("dontbug: Trying to connect to debugger IDE")
	conn, err := net.Dial("tcp", fmt.Sprintf(":%v", replayPort))
	if err != nil {
		log.Fatal(err)
	}

	es.ideConnection = conn

	// send the init packet
	payload := fmt.Sprintf(gInitXmlResponseFormat, es.entryFilePHP, os.Getpid())
	packet := constructDbgpPacket(payload)
	_, err = conn.Write(packet)
	if err != nil {
		log.Fatal(err)
	}

	color.Green("dontbug: Connected to debugger IDE (aka \"client\")")
	fmt.Print("(dontbug) ") // prompt

	reverse := false

	// @TODO add a more sophisticated command line with command completion, history and so forth
	go func() {
		buf := bufio.NewReader(os.Stdin)
		for {
			userResponse, err := buf.ReadString('\n')

			if strings.HasPrefix(userResponse, "t") {
				reverse = !reverse
				if reverse {
					color.Red("In reverse mode")
				} else {
					color.Green("In forward mode")
				}
			} else if strings.HasPrefix(userResponse, "-") {
				command := strings.TrimSpace(userResponse[1:])
				result := sendGdbCommand(es.gdbSession, command);

				jsonResult, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					log.Fatal(err)
				}
				fmt.Println(string(jsonResult))
			} else if strings.HasPrefix(userResponse, "v") {
				Verbose = !Verbose
				if Verbose {
					color.Red("Noisy mode")
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
				xmlResult := diversionSessionCmd(es, command);
				fmt.Println(xmlResult)
			} else if strings.HasPrefix(userResponse, "q") {
				color.Yellow("Exiting.")
				es.gdbSession.Exit()
				es.rrFile.Write([]byte{3}) // send rr Ctrl+C.
			} else {
				if reverse {
					color.Red("In reverse mode")
				} else {
					color.Green("In forward mode")
				}
			}

			if err == io.EOF {
				fmt.Println("Received EOF")
				es.gdbSession.Exit()
				es.rrFile.Write([]byte{3}) // send rr Ctrl+C.
			} else if err != nil {
				log.Fatal(err)
			}

			fmt.Print("(dontbug) ") // prompt
		}
	}()

	go func() {
		for es.status != statusStopped {
			buf := bufio.NewReader(conn)
			command, err := buf.ReadString(byte(0))
			command = strings.TrimRight(command, "\x00")
			if err == io.EOF {
				color.Yellow("Received EOF from IDE")
				break
			} else if err != nil {
				log.Fatal(err)
			}

			if Verbose {
				color.Cyan("\nide -> dontbug: %v", command)
			}

			payload = dispatchIdeRequest(es, command, reverse)
			conn.Write(constructDbgpPacket(payload))

			if Verbose {
				continued := ""
				if len(payload) > 300 {
					continued = "..."
				}
				color.Green("dontbug -> ide:\n%.300v%v", payload, continued)
				fmt.Print("(dontbug) ")
			}
		}

		color.Yellow("\nClosing connection with IDE")
		conn.Close()
		fmt.Print("(dontbug) ")
	}()
}

func dispatchIdeRequest(es *engineState, command string, reverse bool) string {
	dbgpCmd := parseCommand(command)
	if es.lastSequenceNum > dbgpCmd.Sequence {
		log.Fatal("Sequence number", dbgpCmd.Sequence, "has already been seen")
	}

	es.lastSequenceNum = dbgpCmd.Sequence
	switch(dbgpCmd.Command) {
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
		log.Fatal("Unimplemented command:", command)
	}

	return ""
}

func constructBreakpointLocMap(extensionDir string) (map[string]int, []int, int) {
	absExtDir := getDirAbsPath(extensionDir)
	dontbugBreakFilename := absExtDir + "/dontbug_break.c"
	fmt.Println("dontbug: Looking for dontbug_break.c in", absExtDir)

	file, err := os.Open(dontbugBreakFilename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	fmt.Println("dontbug: Found", dontbugBreakFilename)
	bpLocMap := make(map[string]int, 1000)
	buf := bufio.NewReader(file)

	level := 0
	lineno := 0

	line, err := buf.ReadString('\n')
	lineno++
	if err != nil {
		log.Fatal(err)
	}
	indexNumFiles := strings.Index(line, numFilesSentinel)
	if indexNumFiles == -1 {
		log.Fatal("Could not find the marker: ", numFilesSentinel)
	}
	numFiles, err := strconv.Atoi(strings.TrimSpace(line[indexNumFiles + len(numFilesSentinel):]))
	if err != nil {
		log.Fatal(err)
	}

	line, err = buf.ReadString('\n')
	lineno++
	if err != nil {
		log.Fatal(err)
	}
	indexMaxStackDepth := strings.Index(line, maxStackDepthSentinel)
	if indexMaxStackDepth == -1 {
		log.Fatal("Could not find the marker: ", maxStackDepthSentinel)
	}
	maxStackDepth, err := strconv.Atoi(strings.TrimSpace(line[indexMaxStackDepth + len(maxStackDepthSentinel):]))
	if err != nil {
		log.Fatal(err)
	}
	levelLocAr := make([]int, maxStackDepth)

	for {
		line, err := buf.ReadString('\n')
		lineno++
		if err == io.EOF {
			break
		} else if (err != nil) {
			log.Fatal(err)
		}

		indexB := strings.Index(line, phpFilenameSentinel)
		indexL := strings.Index(line, levelSentinel)
		if indexB != -1 {
			filename := strings.TrimSpace("file://" + line[indexB + dontbugCpathStartsAt:])
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

	fmt.Println("dontbug: Completed building association of filename => linenumbers and levels => linenumbers for breakpoints")
	return bpLocMap, levelLocAr, maxStackDepth
}