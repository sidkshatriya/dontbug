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

package cmd

import (
	"github.com/spf13/cobra"
	"os"
	"log"
	"bufio"
	"io"
	"strings"
	"os/exec"
	"github.com/kr/pty"
	"github.com/fatih/color"
	"github.com/cyrus-and/gdb"
	"net"
	"fmt"
	"time"
	"strconv"
	"bytes"
	"errors"
	"encoding/json"
)

const (
	dontbugCstepLineNumTemp int = 91
	dontbugCstepLineNum int = 99
	dontbugCpathStartsAt int = 6
	dontbugMasterBp = "1"
)

var (
	gTraceDir string
	gNoisy bool
	gNoisyPtr *bool
	gGdbNotifications bool
	gGdbNotificationsPtr *bool
)

var gInitXMLformat string =
	`<init xmlns="urn:debugger_protocol_v1" language="PHP" protocol_version="1.0"
		fileuri="file://%v"
		appid="%v" idekey="dontbug">
		<engine version="0.0.1"><![CDATA[dontbug]]></engine>
	</init>`

var gFeatureSetResponseFormat =
	`<response xmlns="urn:debugger_protocol_v1" command="feature_set"
		transaction_id="%v" feature="%v" success="%v">
	</response>`

var gStatusResponseFormat =
	`<response xmlns="urn:debugger_protocol_v1" command="status"
		transaction_id="%v" status="%v" reason="%v">
	</response>`

var gBreakpointSetLineResponseFormat =
	`<response xmlns="urn:debugger_protocol_v1" command="breakpoint_set" transaction_id="%v" status="%v" id="%v">
	</response>`

var gErrorFormat =
	`<response xmlns="urn:debugger_protocol_v1" command="%v" transaction_id="%v">
	 	<error code="%v">
        		<message>%v</message>
    		</error>
	</response>`

var gBreakpointRemoveOrUpdateFormat =
	`<response xmlns="urn:debugger_protocol_v1" command="%v" transaction_id="%v">
	</response>`

var gStepIntoBreakResponseFormat =
	`<response xmlns="urn:debugger_protocol_v1" xmlns:xdebug="http://xdebug.org/dbgp/xdebug" command="step_into"
		transaction_id="%v" status="break" reason="ok">
		<xdebug:message filename="%v" lineno="%v"></xdebug:message>
	</response>`

var gRunOrStepBreakResponseFormat =
	`<response xmlns="urn:debugger_protocol_v1" xmlns:xdebug="http://xdebug.org/dbgp/xdebug" command="%v"
		transaction_id="%v" status="break" reason="ok">
		<xdebug:message filename="%v" lineno="%v"></xdebug:message>
	</response>`

// @TODO Always fail the stdout/stdout/stderr commands, until this is implemented
var gStdFdResponseFormat =
	`<response transaction_id="%v" command="%v" success="0"></response>`

// Replay under rr is read-only. The property set function is to fail, always.
var gPropertySetResponseFormat =
	`<response transaction_id="%v" command="property_set" success="0"></response>`

type DbgpCmd struct {
	Command     string // only the command name eg. stack_get
	FullCommand string // just the options after the command name
	Options     map[string]string
	Sequence    int
}

type BreakpointError struct {
	Code    int
	Message string
}

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
type DebugEngineBreakpointType string
type DebugEngineBreakpointState string
type DebugEngineBreakpointCondition string

const (
	// The following are all PHP breakpoint types
	// Each PHP breakpoint has an entry in the DebugEngineState.Breakpoints table
	// *and* within GDB internally, of course
	breakpointTypeLine DebugEngineBreakpointType = "line"
	breakpointTypeCall DebugEngineBreakpointType = "call"
	breakpointTypeReturn DebugEngineBreakpointType = "return"
	breakpointTypeException DebugEngineBreakpointType = "exception"
	breakpointTypeConditional DebugEngineBreakpointType = "conditional"
	breakpointTypeWatch DebugEngineBreakpointType = "watch"

	// This is a non-PHP breakpoint, i.e. a pure GDB breakpoint
	// Usually internal breakpoints are not stored in the DebugEngineState.Breakpoints table
	// They are usually created and thrown away on demand
	breakpointTypeInternal DebugEngineBreakpointType = "internal"
)

func stringToBreakpointType(t string) (DebugEngineBreakpointType, error) {
	switch t {
	case "line":
		return breakpointTypeLine, nil
	case "call":
		return breakpointTypeCall, nil
	case "return":
		return breakpointTypeReturn, nil
	case "exception":
		return breakpointTypeException, nil
	case "conditional":
		return breakpointTypeConditional, nil
	case "watch":
		return breakpointTypeWatch, nil
	// Deliberately omit the internal breakpoint type
	default:
		return "", errors.New("Unknown breakpoint type")
	}
}

const (
	breakpointHitCondGtEq DebugEngineBreakpointCondition = ">="
	breakpointHitCondEq DebugEngineBreakpointCondition = "=="
	breakpointHitCondMod DebugEngineBreakpointCondition = "%"
)

const (
	breakpointStateDisabled DebugEngineBreakpointState = "disabled"
	breakpointStateEnabled DebugEngineBreakpointState = "enabled"
)

type DebugEngineBreakPoint struct {
	Id           string
	Type         DebugEngineBreakpointType
	Filename     string
	Lineno       int
	State        DebugEngineBreakpointState
	Temporary    bool
	HitCount     int
	HitValue     int
	HitCondition DebugEngineBreakpointCondition
	Exception    string
	Expression   string
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

type FeatureBool struct{ Value bool; ReadOnly bool }
type FeatureInt struct{ Value int; ReadOnly bool }
type FeatureString struct{ Value string; ReadOnly bool }

type FeatureValue interface {
	Set(value string)
	String() string
}

func (this *FeatureBool) Set(value string) {
	if this.ReadOnly {
		log.Fatal("Trying assign to a read only value")
	}

	if value == "0" {
		this.Value = false
	} else if value == "1" {
		this.Value = true
	} else {
		log.Fatal("Trying to assign a non-boolean value to a boolean.")
	}
}

func (this FeatureBool) String() string {
	if this.Value {
		return "1"
	} else {
		return "0"
	}
}

func (this *FeatureString) Set(value string) {
	if this.ReadOnly {
		log.Fatal("Trying assign to a read only value")
	}
	this.Value = value
}

func (this FeatureInt) String() string {
	return strconv.Itoa(this.Value)
}

func (this *FeatureInt) Set(value string) {
	if this.ReadOnly {
		log.Fatal("Trying assign to a read only value")
	}
	var err error
	this.Value, err = strconv.Atoi(value)
	if err != nil {
		log.Fatal(err)
	}

}

func (this FeatureString) String() string {
	return this.Value
}

func initFeatureMap() map[string]FeatureValue {
	var featureMap = map[string]FeatureValue{
		"language_supports_threads" : &FeatureBool{false, true},
		"language_name" : &FeatureString{"PHP", true},
		// @TODO should the exact version be ascertained?
		"language_version" : &FeatureString{"7.0", true},
		"encoding" : &FeatureString{"ISO-8859-1", true},
		"protocol_version" : &FeatureInt{1, true},
		"supports_async" : &FeatureBool{false, true},
		// @TODO full list
		// "breakpoint_types" : &FeatureString{"line call return exception conditional watch", true},
		"breakpoint_types" : &FeatureString{"line", true},
		"multiple_sessions" : &FeatureBool{false, false},
		"max_children": &FeatureInt{64, false},
		"max_data": &FeatureInt{2048, false},
		"max_depth" : &FeatureInt{1, false},
		"extended_properties": &FeatureBool{false, false},
		"show_hidden": &FeatureBool{false, false},
	}

	return featureMap
}

// replayCmd represents the replay command
var replayCmd = &cobra.Command{
	Use:   "replay [optional-trace-dir]",
	Short: "Replay and debug a previous execution",
	Run: func(cmd *cobra.Command, args []string) {
		gNoisy = *gNoisyPtr
		gGdbNotifications = *gGdbNotificationsPtr

		if (len(gExtDir) <= 0) {
			color.Yellow("dontbug: No --ext-dir provided, assuming \"./ext/dontbug\"")
			gExtDir = "ext/dontbug"
		}

		if len(args) < 1 {
			color.Yellow("dontbug: No trace directory provided, latest-trace trace directory assumed")
			gTraceDir = ""
		} else {
			gTraceDir = args[0]
		}

		bpMap, levelAr := constructBreakpointLocMap(gExtDir)
		engineState := startReplayInRR(gTraceDir, bpMap, levelAr)
		debuggerIdeCmdLoop(engineState)
		engineState.RRCmd.Wait()
	},
}

func init() {
	RootCmd.AddCommand(replayCmd)
	gNoisyPtr = replayCmd.Flags().BoolP("verbose", "v", false, "show messages between dontbug, gdb and the ide")
	gGdbNotificationsPtr = replayCmd.Flags().BoolP("gdb-notify", "g", false, "show notification messages from gdb")
}

func debuggerIdeCmdLoop(es *DebugEngineState) {
	color.Yellow("dontbug: Trying to connect to debugger IDE")
	conn, err := net.Dial("tcp", ":9000")
	if err != nil {
		log.Fatal(err)
	}

	es.IdeConnection = conn

	// send the init packet
	payload := fmt.Sprintf(gInitXMLformat, es.EntryFilePHP, os.Getpid())
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
				gNoisy = !gNoisy
				if gNoisy {
					color.Red("Noisy mode")
				} else {
					color.Green("Quiet mode")
				}
			} else if strings.HasPrefix(userResponse, "n") {
				gGdbNotifications = !gGdbNotifications
				if gGdbNotifications {
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

			if gNoisy {
				color.Cyan("\nide -> dontbug: %v", command)
			}

			payload = handleIdeRequest(es, command, reverse)
			conn.Write(constructDbgpPacket(payload))

			if gNoisy {
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

func handleIdeRequest(es *DebugEngineState, command string, reverse bool) string {
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
	case "context_get":
		return handleInDiversionSessionWithNoGdbBpts(es, dbgpCmd)
	case "run":
		return handleRun(es, dbgpCmd, reverse)
	case "stop":
		color.Yellow("IDE sent 'stop' command")
		return handleStop(es, dbgpCmd)
	// All these are dealt with in handleInDiversionSessionStandard()
	case "stack_get":
		fallthrough
	case "stack_depth":
		fallthrough
	case "context_names":
		fallthrough
	case "typemap_get":
		fallthrough
	case "source":
		fallthrough
	case "property_get":
		fallthrough
	case "property_value":
		return handleInDiversionSessionStandard(es, dbgpCmd)
	default:
		es.SourceMap = nil // Just to reduce size of map dump to stdout
		fmt.Println(es)
		log.Fatal("Unimplemented command:", command)
	}

	return ""
}

// rr replay sessions are read-only so property_set will always fail
func handlePropertySet(es *DebugEngineState, dCmd DbgpCmd) string {
	return fmt.Sprintf(gPropertySetResponseFormat, dCmd.Sequence)
}

// @TODO The stdout/stdin/stderr commands always returns attribute success = "0" until this is implemented
func handleStdFd(es *DebugEngineState, dCmd DbgpCmd, fdName string) string {
	return fmt.Sprintf(gStdFdResponseFormat, dCmd.Sequence, fdName)
}

func makeNoisy(f func(*DebugEngineState, DbgpCmd) string, es *DebugEngineState, dCmd DbgpCmd) string {
	originalNoisy := gNoisy
	gNoisy = true
	result := f(es, dCmd)
	gNoisy = originalNoisy
	return result
}

func handleStop(es *DebugEngineState, dCmd DbgpCmd) string {
	es.Status = statusStopped
	return fmt.Sprintf(gStatusResponseFormat, dCmd.Sequence, es.Status, es.Reason)
}

func handleInDiversionSessionStandard(es *DebugEngineState, dCmd DbgpCmd) string {
	return diversionSessionCmd(es, dCmd.FullCommand)
}

func diversionSessionCmd(es *DebugEngineState, command string) string {
	result := xSlashSgdb(es.GdbSession, fmt.Sprintf("dontbug_xdebug_cmd(\"%v\")", command))
	return result
}

// @TODO do we need to do the save/restore of breakpoints?
func handleInDiversionSessionWithNoGdbBpts(es *DebugEngineState, dCmd DbgpCmd) string {
	bpList := getEnabledPhpBreakpoints(es)
	disableAllGdbBreakpoints(es)
	result := diversionSessionCmd(es, dCmd.FullCommand)
	enableGdbBreakpoints(es, bpList)
	return result
}

func getEnabledPhpBreakpoints(es *DebugEngineState) []string {
	var enabledPhpBreakpoints []string
	for name, bp := range es.Breakpoints {
		if bp.State == breakpointStateEnabled && bp.Type != breakpointTypeInternal {
			enabledPhpBreakpoints = append(enabledPhpBreakpoints, name)
		}
	}

	return enabledPhpBreakpoints
}

func isEnabledPhpBreakpoint(es *DebugEngineState, id string) bool {
	for name, bp := range es.Breakpoints {
		if name == id && bp.State == breakpointStateEnabled && bp.Type != breakpointTypeInternal {
			return true
		}
	}

	return false
}

func isEnabledPhpTemporaryBreakpoint(es *DebugEngineState, id string) bool {
	for name, bp := range es.Breakpoints {
		if name == id &&
			bp.State == breakpointStateEnabled &&
			bp.Type != breakpointTypeInternal &&
			bp.Temporary {
			return true
		}
	}

	return false
}

func disableGdbBreakpoints(es *DebugEngineState, bpList []string) {
	if len(bpList) > 0 {
		commandArgs := fmt.Sprintf("%v", strings.Join(bpList, " "))
		sendGdbCommand(es.GdbSession, "break-disable", commandArgs)
		for _, el := range bpList {
			bp, ok := es.Breakpoints[el]
			if ok {
				bp.State = breakpointStateDisabled
			}
		}
	}
}

// convenience function
func disableGdbBreakpoint(es *DebugEngineState, bp string) {
	disableGdbBreakpoints(es, []string{bp})
}

// Note that not all "internal" breakpoints are stored in the breakpoints table
func disableAllGdbBreakpoints(es *DebugEngineState) {
	sendGdbCommand(es.GdbSession, "break-disable")
	for _, bp := range es.Breakpoints {
		bp.State = breakpointStateDisabled
	}
}

func enableAllGdbBreakpoints(es *DebugEngineState) {
	sendGdbCommand(es.GdbSession, "break-enable")
	for _, bp := range es.Breakpoints {
		bp.State = breakpointStateEnabled
	}
}

func enableGdbBreakpoints(es *DebugEngineState, bpList []string) {
	if len(bpList) > 0 {
		commandArgs := fmt.Sprintf("%v", strings.Join(bpList, " "))
		sendGdbCommand(es.GdbSession, "break-enable", commandArgs)
		for _, el := range bpList {
			bp, ok := es.Breakpoints[el]
			if ok {
				bp.State = breakpointStateEnabled
			}
		}
	}
}

func getAssocEnabledPhpBreakpoint(es *DebugEngineState, filename string, lineno int) (string, bool) {
	for name, bp := range es.Breakpoints {
		if bp.Filename == filename &&
			bp.Lineno == lineno &&
			bp.State == breakpointStateEnabled &&
			bp.Type != breakpointTypeInternal {
			return name, true
		}
	}

	return "", false
}

// convenience function
func enableGdbBreakpoint(es *DebugEngineState, bp string) {
	enableGdbBreakpoints(es, []string{bp})
}

// Sets an equivalent breakpoint in gdb for PHP
// Also inserts the breakpoint into es.Breakpoints table
func setPhpBreakpointInGdb(es *DebugEngineState, phpFilename string, phpLineno int, disabled bool, temporary bool) (string, *BreakpointError) {
	internalLineno, ok := es.SourceMap[phpFilename]
	if !ok {
		warning := fmt.Sprintf("dontbug: Not able to find %v to add a breakpoint. Either the IDE is trying to set a breakpoint for a file from a different project (which is OK) or you need to run 'dontbug generate' specific to this project", phpFilename)
		color.Yellow(warning)
		return "", &BreakpointError{200, warning}
	}

	breakpointState := breakpointStateEnabled
	disabledFlag := ""
	if disabled {
		disabledFlag = "-d " // Note the space after -d
		breakpointState = breakpointStateDisabled
	}

	temporaryFlag := ""
	if temporary {
		temporaryFlag = "-t " // Note the space after -t
	}

	// @TODO for some reason this break-insert command stops working if we break sendGdbCommand call into operation, argument params
	result := sendGdbCommand(es.GdbSession,
		fmt.Sprintf("break-insert %v%v-f -c \"lineno == %v\" --source dontbug_break.c --line %v", temporaryFlag, disabledFlag, phpLineno, internalLineno))

	if result["class"] != "done" {
		warning := "Could not set breakpoint in gdb. Something is probably wrong with breakpoint parameters"
		color.Red(warning)
		return "", &BreakpointError{200, warning}
	}

	payload := result["payload"].(map[string]interface{})
	bkpt := payload["bkpt"].(map[string]interface{})
	id := bkpt["number"].(string)

	_, ok = es.Breakpoints[id]
	if ok {
		log.Fatal("breakpoint number returned by gdb not unique:", id)
	}

	es.Breakpoints[id] = &DebugEngineBreakPoint{
		Id:id,
		Filename:phpFilename,
		Lineno:phpLineno,
		State:breakpointState,
		Temporary:temporary,
		Type:breakpointTypeLine,
	}

	return id, nil
}

// Does not make an entry in breakpoints table
func setPhpStackLevelBreakpointInGdb(es *DebugEngineState, level int) string {
	line := es.LevelAr[level]

	result := sendGdbCommand(es.GdbSession, "break-insert",
		fmt.Sprintf("-f --source dontbug_break.c --line %v", line))

	if result["class"] != "done" {
		log.Fatal("Breakpoint was not set successfully")
	}

	payload := result["payload"].(map[string]interface{})
	bkpt := payload["bkpt"].(map[string]interface{})
	id := bkpt["number"].(string)

	return id
}

func removeGdbBreakpoint(es *DebugEngineState, id string) {
	sendGdbCommand(es.GdbSession, "break-delete", id)
	_, ok := es.Breakpoints[id]
	if ok {
		delete(es.Breakpoints, id)
	}
}

func handleStepInto(es *DebugEngineState, dCmd DbgpCmd, reverse bool) string {
	gotoMasterBpLocation(es, reverse)

	filename := xSlashSgdb(es.GdbSession, "filename")
	lineno := xSlashDgdb(es.GdbSession, "lineno")
	return fmt.Sprintf(gStepIntoBreakResponseFormat, dCmd.Sequence, filename, lineno)
}

func gotoMasterBpLocation(es *DebugEngineState, reverse bool) (string, bool) {
	enableGdbBreakpoint(es, dontbugMasterBp)
	id, ok := continueExecution(es, reverse)
	disableGdbBreakpoint(es, dontbugMasterBp)
	return id, ok
}

func handleRun(es *DebugEngineState, dCmd DbgpCmd, reverse bool) string {
	// Don't hit a breakpoint on your (own) line
	if reverse {
		bpList := getEnabledPhpBreakpoints(es)
		disableGdbBreakpoints(es, bpList)
		// Kind of a step_into backwards
		gotoMasterBpLocation(es, true)
		enableGdbBreakpoints(es, bpList)
	}

	// Resume execution, either forwards or backwards
	_, userBreakPointHit := continueExecution(es, reverse)

	if userBreakPointHit {
		bpList := getEnabledPhpBreakpoints(es)
		disableGdbBreakpoints(es, bpList)
		if !reverse {
			gotoMasterBpLocation(es, false)
		} else {
			// After you hit the php breakpoint, step over backwards.
			currentPhpStackLevel := xSlashDgdb(es.GdbSession, "level")
			id := setPhpStackLevelBreakpointInGdb(es, currentPhpStackLevel)
			continueExecution(es, true)
			removeGdbBreakpoint(es, id)

			// Note that we move in the forward direction even though we are in the reverse case
			gotoMasterBpLocation(es, false)
		}

		filename := xSlashSgdb(es.GdbSession, "filename")
		phpLineno := xSlashDgdb(es.GdbSession, "lineno")

		enableGdbBreakpoints(es, bpList)

		return fmt.Sprintf(gRunOrStepBreakResponseFormat, "run", dCmd.Sequence, filename, phpLineno)
	}

	log.Fatal("Unimplemented program end handling")
	return ""
}

func handleStepOverOrOut(es *DebugEngineState, dCmd DbgpCmd, reverse bool, stepOut bool) string {
	command := "step_over"
	if (stepOut) {
		command = "step_out"
	}

	currentPhpStackLevel := xSlashDgdb(es.GdbSession, "level")
	levelLimit := currentPhpStackLevel
	if stepOut && currentPhpStackLevel > 0 {
		levelLimit = currentPhpStackLevel - 1
	}

	// We're interested in maintaining or decreasing the stack level for step over
	// We're interested in strictly decreasing the stack level for step out
	id := setPhpStackLevelBreakpointInGdb(es, levelLimit)
	_, ok := continueExecution(es, reverse)

	if !reverse {
		// Cleanup
		removeGdbBreakpoint(es, id)

		gotoMasterBpLocation(es, false)
	} else {
		// A user (php) breakpoint was hit
		if ok {
			// Cleanup
			removeGdbBreakpoint(es, id)

			// What stack level are we on currently?
			levelLimit := xSlashDgdb(es.GdbSession, "level")

			// Disable all currently active breaks
			bpList := getEnabledPhpBreakpoints(es)
			disableGdbBreakpoints(es, bpList)

			// Step over/out in reverse to the previous statement with all other breaks disabled
			id2 := setPhpStackLevelBreakpointInGdb(es, levelLimit)
			continueExecution(es, true)

			// Remove this one too
			removeGdbBreakpoint(es, id2)

			enableGdbBreakpoints(es, bpList)

		} else {
			// Disable all currently active breaks
			bpList := getEnabledPhpBreakpoints(es)
			disableGdbBreakpoints(es, bpList)

			// Do this again with the php stack level breakpoint enabled
			continueExecution(es, true)
			enableGdbBreakpoints(es, bpList)

			// Cleanup
			removeGdbBreakpoint(es, id)
		}

		// Note that we run in forward direction, even though we're in reverse mode
		gotoMasterBpLocation(es, false)
	}

	filename := xSlashSgdb(es.GdbSession, "filename")
	phpLineno := xSlashDgdb(es.GdbSession, "lineno")

	return fmt.Sprintf(gRunOrStepBreakResponseFormat, command, dCmd.Sequence, filename, phpLineno)
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

func handleFeatureSet(es *DebugEngineState, dCmd DbgpCmd) string {
	n, ok := dCmd.Options["n"]
	if !ok {
		log.Fatal("Please provide -n option in feature_set")
	}

	v, ok := dCmd.Options["v"]
	if !ok {
		log.Fatal("Not provided v option in feature_set")
	}

	var featureVal FeatureValue
	featureVal, ok = es.FeatureMap[n]
	if !ok {
		log.Fatal("Unknown option:", n)
	}

	featureVal.Set(v)
	return fmt.Sprintf(gFeatureSetResponseFormat, dCmd.Sequence, n, 1)
}

func handleStatus(es *DebugEngineState, dCmd DbgpCmd) string {
	return fmt.Sprintf(gStatusResponseFormat, dCmd.Sequence, es.Status, es.Reason)
}

func handleBreakpointSet(es *DebugEngineState, dCmd DbgpCmd) string {
	t, ok := dCmd.Options["t"]
	if !ok {
		log.Fatal("Please provide breakpoint type option -t in breakpoint_set")
	}

	tt, err := stringToBreakpointType(t)
	if err != nil {
		log.Fatal(err)
	}

	switch tt {
	case breakpointTypeLine:
		return handleBreakpointSetLineBreakpoint(es, dCmd)
	default:
		return fmt.Sprintf(gErrorFormat, "breakpoint_set", dCmd.Sequence, 201, "Breakpoint type " + tt + " is not supported")
	}

	return ""
}

func handleBreakpointUpdate(es *DebugEngineState, dCmd DbgpCmd) string {
	d, ok := dCmd.Options["d"]
	if !ok {
		log.Fatal("Please provide breakpoint number for breakpoint_update")
	}

	_, ok = dCmd.Options["n"]
	if ok {
		log.Fatal("Line number updates are currently unsupported in breakpoint_update")
	}

	_, ok = dCmd.Options["h"]
	if ok {
		log.Fatal("Hit condition/value update is currently not supported in breakpoint_update")
	}

	_, ok = dCmd.Options["o"]
	if ok {
		log.Fatal("Hit condition/value is currently not supported in breakpoint_update")
	}

	s, ok := dCmd.Options["s"]
	if !ok {
		log.Fatal("Please provide new breakpoint status in breakpoint_update")
	}

	if s == "disabled" {
		disableGdbBreakpoint(es, d)
	} else if s == "enabled" {
		enableGdbBreakpoint(es, d)
	} else {
		log.Fatalf("Unknown breakpoint status %v for breakpoint_update", s)
	}

	return fmt.Sprintf(gBreakpointRemoveOrUpdateFormat, "breakpoint_update", dCmd.Sequence)
}

func handleBreakpointRemove(es *DebugEngineState, dCmd DbgpCmd) string {
	d, ok := dCmd.Options["d"]
	if !ok {
		log.Fatal("Please provide breakpoint id to remove")
	}

	removeGdbBreakpoint(es, d)

	return fmt.Sprintf(gBreakpointRemoveOrUpdateFormat, "breakpoint_remove", dCmd.Sequence)
}

func handleBreakpointSetLineBreakpoint(es *DebugEngineState, dCmd DbgpCmd) string {
	phpFilename, ok := dCmd.Options["f"]
	if !ok {
		log.Fatal("Please provide filename option -f in breakpoint_set")
	}

	status, ok := dCmd.Options["s"]
	disabled := false
	if ok {
		if status == "disabled" {
			disabled = true
		} else if status != "enabled" {
			log.Fatalf("Unknown breakpoint status %v", status)
		}
	} else {
		status = "enabled"
	}

	phpLinenoString, ok := dCmd.Options["n"]
	if !ok {
		log.Fatal("Please provide line number option -n in breakpoint_set")
	}

	r, ok := dCmd.Options["r"]
	temporary := false
	if ok && r == "1" {
		temporary = true
	}

	_, ok = dCmd.Options["h"]
	if ok {
		return fmt.Sprintf(gErrorFormat, "breakpoint_set", dCmd.Sequence, 201, "Hit condition/value is currently not supported")
	}

	_, ok = dCmd.Options["o"]
	if ok {
		return fmt.Sprintf(gErrorFormat, "breakpoint_set", dCmd.Sequence, 201, "Hit condition/value is currently not supported")
	}

	phpLineno, err := strconv.Atoi(phpLinenoString)
	if err != nil {
		log.Fatal(err)
	}

	id, breakErr := setPhpBreakpointInGdb(es, phpFilename, phpLineno, disabled, temporary)
	if breakErr != nil {
		return fmt.Sprintf(gErrorFormat, "breakpoint_set", dCmd.Sequence, breakErr.Code, breakErr.Message)
	}

	return fmt.Sprintf(gBreakpointSetLineResponseFormat, dCmd.Sequence, status, id)
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

func constructBreakpointLocMap(extensionDir string) (map[string]int, [maxLevels]int) {
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
	var levelLocAr [maxLevels]int

	level := 0
	lineno := 0
	line, err := buf.ReadString('\n')
	lineno++
	if err != nil {
		log.Fatal(err)
	}

	sentinel := "//&&& Number of Files:"
	indexNumFiles := strings.Index(line, sentinel)
	if indexNumFiles == -1 {
		log.Fatal("Could not find the marker: ", sentinel)
	}

	numFiles, err := strconv.Atoi(strings.TrimSpace(line[indexNumFiles + len(sentinel):]))
	if err != nil {
		log.Fatal(err)
	}

	for {
		line, err := buf.ReadString('\n')
		lineno++
		if err == io.EOF {
			break
		} else if (err != nil) {
			log.Fatal(err)
		}

		indexB := strings.Index(line, "//###")
		indexL := strings.Index(line, "//$$$")
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
	return bpLocMap, levelLocAr
}

func startReplayInRR(traceDir string, bpMap map[string]int, levelAr [maxLevels]int) *DebugEngineState {
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

// @TODO what about multiple breakpoints on the same c source code line?
func breakpointStopGetId(notification map[string]interface{}) (string, bool) {
	class, ok := notification["class"].(string)
	if !ok || class != "stopped" {
		return "", false
	}

	payload, ok := notification["payload"].(map[string]interface{})
	if !ok {
		return "", false
	}

	breakPointNumString, ok := payload["bkptno"].(string)
	if !ok {
		return "", false
	}

	reason, ok := payload["reason"].(string)
	if !ok || reason != "breakpoint-hit" {
		return "", false
	}

	return breakPointNumString, true
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
			if gGdbNotifications {
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

func sendGdbCommand(gdbSession *gdb.Gdb, command string, arguments ...string) map[string]interface{} {
	if (gNoisy) {
		color.Green("dontbug -> gdb: %v %v", command, strings.Join(arguments, " "))
	}
	result, err := gdbSession.Send(command, arguments...)
	if err != nil {
		log.Fatal(err)
	}

	if (gNoisy) {
		continued := ""
		if (len(result) > 300) {
			continued = "..."
		}
		color.Cyan("gdb -> dontbug: %.300v%v", result, continued)
	}
	return result
}

func sendGdbCommandNoisy(gdbSession *gdb.Gdb, command string, arguments ...string) map[string]interface{} {
	originalNoisy := gNoisy
	gNoisy = true
	result := sendGdbCommand(gdbSession, command, arguments...)
	gNoisy = originalNoisy
	return result
}
