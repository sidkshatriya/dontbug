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
	"github.com/cyrus-and/gdb"
	"strings"
	"log"
	"bytes"
	"os"
	"os/exec"
	"fmt"
	"encoding/json"
	"io"
	"github.com/kr/pty"
	"time"
	"bufio"
	"net"
	"path/filepath"
	"github.com/fatih/color"
	"errors"
	"strconv"
)

const maxLevels int = 128

var (
	Noisy bool
	GdbNotifications bool
)

const (
	dontbugCstepLineNumTemp int = 91
	dontbugCstepLineNum int = 99
	dontbugCpathStartsAt int = 6
	dontbugMasterBp = "1"
)

type DebugEngineState struct {
	BreakStopNotify chan string
	GdbSession      *gdb.Gdb
	IdeConnection   net.Conn
	RRFile          *os.File
	RRCmd           *exec.Cmd
	EntryFilePHP    string
	LastSequenceNum int
	Status          DebugEngineStatus
	Reason          DebugEngineReason
	FeatureMap      map[string]FeatureValue
	Breakpoints     map[string]*DebugEngineBreakPoint
	SourceMap       map[string]int
	LevelAr         [maxLevels]int
}

type DebugEngineStatus string
type DebugEngineReason string

type DbgpCmd struct {
	Command     string // only the command name eg. stack_get
	FullCommand string // just the options after the command name
	Options     map[string]string
	Sequence    int
}

const (
	statusStarting DebugEngineStatus = "starting"
	statusStopping DebugEngineStatus = "stopping"
	statusStopped DebugEngineStatus = "stopped"
	statusRunning DebugEngineStatus = "running"
	statusBreak DebugEngineStatus = "break"
)

const (
	reasonOk DebugEngineReason = "ok"
	reasonError DebugEngineReason = "error"
	reasonAborted DebugEngineReason = "aborted"
	reasonExeception DebugEngineReason = "exception"
)

func sendGdbCommand(gdbSession *gdb.Gdb, command string, arguments ...string) map[string]interface{} {
	if (Noisy) {
		color.Green("dontbug -> gdb: %v %v", command, strings.Join(arguments, " "))
	}
	result, err := gdbSession.Send(command, arguments...)
	if err != nil {
		log.Fatal(err)
	}

	if (Noisy) {
		continued := ""
		if (len(result) > 300) {
			continued = "..."
		}
		color.Cyan("gdb -> dontbug: %.300v%v", result, continued)
	}
	return result
}

func sendGdbCommandNoisy(gdbSession *gdb.Gdb, command string, arguments ...string) map[string]interface{} {
	originalNoisy := Noisy
	Noisy = true
	result := sendGdbCommand(gdbSession, command, arguments...)
	Noisy = originalNoisy
	return result
}

// a gdb string response looks like '0x7f261d8624e8 "some string here"'
// empty string looks '0x7f44a33a9c1e ""'
func parseGdbStringResponse(gdbResponse string) (string, error) {
	first := strings.Index(gdbResponse, "\"")
	last := strings.LastIndex(gdbResponse, "\"")

	if (first == last || first == -1 || last == -1) {
		return "", errors.New("Improper gdb data-evaluate-expression string response to: " + gdbResponse)
	}

	unquote := unquoteGdbStringResult(gdbResponse[first + 1:last])
	return unquote, nil
}

func unquoteGdbStringResult(input string) string {
	l := len(input)
	var buf bytes.Buffer
	skip := false
	for i, c := range input {
		if skip {
			skip = false
			continue
		}

		if c == '\\' && i < l && input[i + 1] == '"' {
			buf.WriteRune('"')
			skip = true
		} else {
			buf.WriteRune(c)
		}
	}

	return buf.String()
}

// Starts gdb and creates a new DebugEngineState object
func startGdbAndInitDebugEngineState(hardlinkFile string, bpMap map[string]int, levelAr [maxLevels]int, rrFile *os.File, rrCmd *exec.Cmd) *DebugEngineState {
	gdbArgs := []string{"gdb", "-l", "-1", "-ex", "target extended-remote :9999", "--interpreter", "mi", hardlinkFile}
	fmt.Println("dontbug: Starting gdb with the following string:", strings.Join(gdbArgs, " "))

	var gdbSession *gdb.Gdb
	var err error

	stopEventChan := make(chan string)
	started := false

	gdbSession, err = gdb.NewCmd(gdbArgs,
		func(notification map[string]interface{}) {
			if GdbNotifications {
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

	es := &DebugEngineState{
		GdbSession: gdbSession,
		BreakStopNotify: stopEventChan,
		FeatureMap:initFeatureMap(),
		EntryFilePHP:properFilename,
		Status:statusStarting,
		Reason:reasonOk,
		SourceMap:bpMap,
		LastSequenceNum:0,
		LevelAr:levelAr,
		RRCmd: rrCmd,
		Breakpoints:make(map[string]*DebugEngineBreakPoint, 10),
		RRFile:rrFile,
	}

	// "1" is always the first breakpoint number in gdb
	// Its used for stepping
	es.Breakpoints["1"] = &DebugEngineBreakPoint{
		Id:"1",
		Lineno:dontbugCstepLineNum,
		Filename:"dontbug.c",
		State:breakpointStateDisabled,
		Temporary:false,
		Type:breakpointTypeInternal,
	}

	return es
}

func StartReplayInRR(traceDir string, bpMap map[string]int, levelAr [maxLevels]int) *DebugEngineState {
	absTraceDir := ""
	if len(traceDir) > 0 {
		absTraceDir = getDirAbsPath(traceDir)
	}

	// Start an rr replay session
	replayCmd := exec.Command("rr", "replay", "-s", "9999", absTraceDir)
	fmt.Println("dontbug: Using rr at:", replayCmd.Path)
	f, err := pty.Start(replayCmd)
	if err != nil {
		log.Fatal(err)
	}
	color.Green("dontbug: Successfully started replay session")

	// Abort if we are not able to get the gdb connection string within 10 sec
	cancel := make(chan bool, 1)
	go func() {
		time.Sleep(10 * time.Second)
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
		line, _ := buf.ReadString('\n')
		fmt.Println(line)
		if strings.Contains(line, "target extended-remote") {
			cancel <- true
			close(cancel)

			go io.Copy(os.Stdout, f)
			slashAt := strings.Index(line, "/")

			hardlinkFile := strings.TrimSpace(line[slashAt:])
			return startGdbAndInitDebugEngineState(hardlinkFile, bpMap, levelAr, f, replayCmd)
		}
	}

	return nil
}

func parseCommand(fullCommand string) DbgpCmd {
	components := strings.Fields(fullCommand)
	flags := make(map[string]string)
	command := components[0]
	for i, v := range components[1:] {
		if (i % 2 == 1) {
			continue
		}

		// Also remove the leading "-" in the flag via [1:]
		if i + 2 < len(components) {
			flags[strings.TrimSpace(v)[1:]] = strings.TrimSpace(components[i + 2])
		} else {
			flags[strings.TrimSpace(v)[1:]] = ""
		}
	}

	seq, ok := flags["i"]
	if !ok {
		log.Fatal("Could not find sequence number in command")
	}

	seqInt, err := strconv.Atoi(seq)
	if err != nil {
		log.Fatal(err)
	}

	return DbgpCmd{command, fullCommand, flags, seqInt}
}

func xSlashSgdb(gdbSession *gdb.Gdb, expression string) string {
	resultString := xGdbCmdValue(gdbSession, expression)
	finalString, err := parseGdbStringResponse(resultString)
	if err != nil {
		log.Fatal(finalString)
	}
	return finalString

}

func xSlashDgdb(gdbSession *gdb.Gdb, expression string) int {
	resultString := xGdbCmdValue(gdbSession, expression)
	intResult, err := strconv.Atoi(resultString)
	if err != nil {
		log.Fatal(err)
	}
	return intResult
}

func xGdbCmdValue(gdbSession *gdb.Gdb, expression string) string {
	result := sendGdbCommand(gdbSession, "data-evaluate-expression", expression)
	class, ok := result["class"]

	commandWas := "data-evaluate-expression " + expression
	if !ok {
		sendGdbCommand(gdbSession, "thread-info")
		log.Fatal("Could not execute the gdb/mi command: ", commandWas)
	}

	if class != "done" {
		sendGdbCommand(gdbSession, "thread-info")
		log.Fatal("Could not execute the gdb/mi command: ", commandWas)
	}

	payload := result["payload"].(map[string]interface{})
	resultString := payload["value"].(string)

	return resultString
}

// Returns breakpoint id, true if stopped on a PHP breakpoint
func continueExecution(es *DebugEngineState, reverse bool) (string, bool) {
	es.Status = statusRunning
	if (reverse) {
		sendGdbCommand(es.GdbSession, "exec-continue", "--reverse")
	} else {
		sendGdbCommand(es.GdbSession, "exec-continue")
	}

	// Wait for the corresponding breakpoint hit break id
	breakId := <-es.BreakStopNotify
	es.Status = statusBreak

	// Probably not a good idea to pass out breakId for a breakpoint that is gone
	// But we're not using breakId currently
	if isEnabledPhpTemporaryBreakpoint(es, breakId) {
		delete(es.Breakpoints, breakId)
		return breakId, true
	}

	if isEnabledPhpBreakpoint(es, breakId) {
		return breakId, true
	}

	return breakId, false
}

func DebuggerIdeCmdLoop(es *DebugEngineState) {
	color.Yellow("dontbug: Trying to connect to debugger IDE")
	conn, err := net.Dial("tcp", ":9000")
	if err != nil {
		log.Fatal(err)
	}

	es.IdeConnection = conn

	// send the init packet
	payload := fmt.Sprintf(initXmlResponseFormat, es.EntryFilePHP, os.Getpid())
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
		for {
			var userResponse string
			var a [5]string // @TODO remove this kludge
			fmt.Scanln(&userResponse, &a[0], &a[1], &a[2], &a[3], &a[4])

			if strings.HasPrefix(userResponse, "t") {
				reverse = !reverse
				if reverse {
					color.Red("In reverse mode")
				} else {
					color.Green("In forward mode")
				}
			} else if strings.HasPrefix(userResponse, "-") {
				// @TODO remove this kludge
				command := strings.TrimSpace(fmt.Sprintf("%v %v %v %v %v %v", userResponse[1:], a[0], a[1], a[2], a[3], a[4]))
				result := sendGdbCommand(es.GdbSession, command);

				jsonResult, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					log.Fatal(err)
				}
				fmt.Println(string(jsonResult))
			} else if strings.HasPrefix(userResponse, "v") {
				Noisy = !Noisy
				if Noisy {
					color.Red("Noisy mode")
				} else {
					color.Green("Quiet mode")
				}
			} else if strings.HasPrefix(userResponse, "n") {
				GdbNotifications = !GdbNotifications
				if GdbNotifications {
					color.Red("Will show gdb notifications")
				} else {
					color.Green("Wont show gdb notifications")
				}
			} else if strings.HasPrefix(userResponse, "#") {
				// @TODO remove this kludge
				command := strings.TrimSpace(fmt.Sprintf("%v %v %v %v %v %v", userResponse[1:], a[0], a[1], a[2], a[3], a[4]))
				// @TODO blacklist commands that are handled in gdb or dontbug instead
				xmlResult := diversionSessionCmd(es, command);
				fmt.Println(xmlResult)
			} else if strings.HasPrefix(userResponse, "q") {
				color.Yellow("Exiting.")
				conn.Close()
				es.GdbSession.Exit()
				es.RRFile.Write([]byte{3}) // send rr Ctrl+C.
			} else {
				if reverse {
					color.Red("In reverse mode")
				} else {
					color.Green("In forward mode")
				}
			}
			fmt.Print("(dontbug) ")
		}
	}()

	go func() {
		for es.Status != statusStopped {
			buf := bufio.NewReader(conn)
			command, err := buf.ReadString(byte(0))
			command = strings.TrimRight(command, "\x00")
			if err == io.EOF {
				color.Yellow("Received EOF from IDE")
				break
			} else if err != nil {
				log.Fatal(err)
			}

			if Noisy {
				color.Cyan("\nide -> dontbug: %v", command)
			}

			payload = dispatchIdeRequest(es, command, reverse)
			conn.Write(constructDbgpPacket(payload))

			if Noisy {
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

func constructDbgpPacket(payload string) []byte {
	header_xml := "<?xml version=\"1.0\" encoding=\"iso-8859-1\"?>\n"
	var buf bytes.Buffer
	buf.WriteString(strconv.Itoa(len(payload) + len(header_xml)))
	buf.Write([]byte{0})
	buf.WriteString(header_xml)
	buf.WriteString(payload)
	buf.Write([]byte{0})
	return buf.Bytes()
}

func dispatchIdeRequest(es *DebugEngineState, command string, reverse bool) string {
	dbgpCmd := parseCommand(command)
	if es.LastSequenceNum > dbgpCmd.Sequence {
		log.Fatal("Sequence number", dbgpCmd.Sequence, "has already been seen")
	}

	es.LastSequenceNum = dbgpCmd.Sequence
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
		es.SourceMap = nil // Just to reduce size of map dump to stdout
		fmt.Println(es)
		log.Fatal("Unimplemented command:", command)
	}

	return ""
}

func makeNoisy(f func(*DebugEngineState, DbgpCmd) string, es *DebugEngineState, dCmd DbgpCmd) string {
	originalNoisy := Noisy
	Noisy = true
	result := f(es, dCmd)
	Noisy = originalNoisy
	return result
}

// Output a fatal error if there is anything wrong with dirPath
// Otherwise output the absolute path of the directory
func getDirAbsPath(dirPath string) string {
	// Create an absolute path for the dirPath directory
	dirAbsPath, err := filepath.Abs(dirPath)
	if err != nil {
		log.Fatal(err)
	}

	// Does the directory even exist?
	_, err = os.Stat(dirAbsPath)
	if err != nil {
		log.Fatal(err)
	}

	return dirAbsPath
}
