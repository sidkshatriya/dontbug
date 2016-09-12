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
	"errors"
	"fmt"
	"github.com/fatih/color"
	"log"
	"strconv"
	"strings"
)

const (
	// The following are all PHP breakpoint types
	// Each PHP breakpoint has an entry in the DebugEngineState.Breakpoints table
	// *and* within GDB internally, of course
	breakpointTypeLine        engineBreakpointType = "line"
	breakpointTypeCall        engineBreakpointType = "call"
	breakpointTypeReturn      engineBreakpointType = "return"
	breakpointTypeException   engineBreakpointType = "exception"
	breakpointTypeConditional engineBreakpointType = "conditional"
	breakpointTypeWatch       engineBreakpointType = "watch"
	// This is a non-PHP breakpoint, i.e. a pure GDB breakpoint
	// Usually internal breakpoints are not stored in the DebugEngineState.Breakpoints table
	// They are usually created and thrown away on demand
	breakpointTypeInternal engineBreakpointType = "internal"

	breakpointHitCondGtEq engineBreakpointCondition = ">="
	breakpointHitCondEq   engineBreakpointCondition = "=="
	breakpointHitCondMod  engineBreakpointCondition = "%"

	breakpointStateDisabled engineBreakpointState = "disabled"
	breakpointStateEnabled  engineBreakpointState = "enabled"

	// Error codes returned when a user (php) breakpoint cannot be set
	breakpointErrorCodeCouldNotSet      engineBreakpointErrorCode = 200
	breakpointErrorCodeTypeNotSupported engineBreakpointErrorCode = 201
)

type engineBreakpointError struct {
	code    engineBreakpointErrorCode
	message string
}

type engineBreakpointType string
type engineBreakpointState string
type engineBreakpointCondition string
type engineBreakpointErrorCode int

type engineBreakPoint struct {
	id           string
	bpType       engineBreakpointType
	filename     string
	lineno       int
	state        engineBreakpointState
	temporary    bool
	hitCount     int
	hitValue     int
	hitCondition engineBreakpointCondition
	exception    string
	expression   string
}

func stringToBreakpointType(t string) (engineBreakpointType, error) {
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

func handleBreakpointUpdate(es *engineState, dCmd dbgpCmd) string {
	d, ok := dCmd.options["d"]
	if !ok {
		panicWith("Please provide breakpoint number for breakpoint_update")
	}

	_, ok = dCmd.options["n"]
	if ok {
		panicWith("Line number updates are currently unsupported in breakpoint_update")
	}

	_, ok = dCmd.options["h"]
	if ok {
		panicWith("Hit condition/value update is currently not supported in breakpoint_update")
	}

	_, ok = dCmd.options["o"]
	if ok {
		panicWith("Hit condition/value is currently not supported in breakpoint_update")
	}

	s, ok := dCmd.options["s"]
	if !ok {
		panicWith("Please provide new breakpoint status in breakpoint_update")
	}

	if s == "disabled" {
		disableGdbBreakpoint(es, d)
	} else if s == "enabled" {
		enableGdbBreakpoint(es, d)
	} else {
		panicWith(fmt.Sprintf("Unknown breakpoint status %v for breakpoint_update", s))
	}

	return fmt.Sprintf(gBreakpointRemoveOrUpdateXmlResponseFormat, "breakpoint_update", dCmd.seqNum)
}

func handleBreakpointRemove(es *engineState, dCmd dbgpCmd) string {
	d, ok := dCmd.options["d"]
	if !ok {
		panicWith("Please provide breakpoint id to remove")
	}

	removeGdbBreakpoint(es, d)

	return fmt.Sprintf(gBreakpointRemoveOrUpdateXmlResponseFormat, "breakpoint_remove", dCmd.seqNum)
}

func handleBreakpointSetLineBreakpoint(es *engineState, dCmd dbgpCmd) string {
	phpFilename, ok := dCmd.options["f"]
	if !ok {
		panicWith("Please provide filename option -f in breakpoint_set")
	}

	status, ok := dCmd.options["s"]
	disabled := false
	if ok {
		if status == "disabled" {
			disabled = true
		} else if status != "enabled" {
			panicWith("Unknown breakpoint status: " + status)
		}
	} else {
		status = "enabled"
	}

	phpLinenoString, ok := dCmd.options["n"]
	if !ok {
		panicWith("Please provide line number option -n in breakpoint_set")
	}

	r, ok := dCmd.options["r"]
	temporary := false
	if ok && r == "1" {
		temporary = true
	}

	_, ok = dCmd.options["h"]
	if ok {
		return fmt.Sprintf(gErrorXmlResponseFormat, "breakpoint_set", dCmd.seqNum, breakpointErrorCodeTypeNotSupported, "Hit condition/value is currently not supported")
	}

	_, ok = dCmd.options["o"]
	if ok {
		return fmt.Sprintf(gErrorXmlResponseFormat, "breakpoint_set", dCmd.seqNum, breakpointErrorCodeTypeNotSupported, "Hit condition/value is currently not supported")
	}

	phpLineno, err := strconv.Atoi(phpLinenoString)
	panicIf(err)

	id, breakErr := setPhpBreakpointInGdb(es, phpFilename, phpLineno, disabled, temporary)
	if breakErr != nil {
		return fmt.Sprintf(gErrorXmlResponseFormat, "breakpoint_set", dCmd.seqNum, breakErr.code, breakErr.message)
	}

	return fmt.Sprintf(gBreakpointSetLineXmlResponseFormat, dCmd.seqNum, status, id)
}

func handleBreakpointSet(es *engineState, dCmd dbgpCmd) string {
	t, ok := dCmd.options["t"]
	if !ok {
		panicWith("Please provide breakpoint type option -t in breakpoint_set")
	}

	tt, err := stringToBreakpointType(t)
	panicIf(err)

	switch tt {
	case breakpointTypeLine:
		return handleBreakpointSetLineBreakpoint(es, dCmd)
	default:
		return fmt.Sprintf(gErrorXmlResponseFormat, "breakpoint_set", dCmd.seqNum, breakpointErrorCodeTypeNotSupported, "Breakpoint type "+tt+" is not supported")
	}

	return ""
}

func getEnabledPhpBreakpoints(es *engineState) []string {
	var enabledPhpBreakpoints []string
	for name, bp := range es.breakpoints {
		if bp.state == breakpointStateEnabled && bp.bpType != breakpointTypeInternal {
			enabledPhpBreakpoints = append(enabledPhpBreakpoints, name)
		}
	}

	return enabledPhpBreakpoints
}

func isEnabledPhpBreakpoint(es *engineState, id string) bool {
	for name, bp := range es.breakpoints {
		if name == id && bp.state == breakpointStateEnabled && bp.bpType != breakpointTypeInternal {
			return true
		}
	}

	return false
}

func isEnabledPhpTemporaryBreakpoint(es *engineState, id string) bool {
	for name, bp := range es.breakpoints {
		if name == id &&
			bp.state == breakpointStateEnabled &&
			bp.bpType != breakpointTypeInternal &&
			bp.temporary {
			return true
		}
	}

	return false
}

func disableGdbBreakpoints(es *engineState, bpList []string) {
	if len(bpList) > 0 {
		commandArgs := fmt.Sprintf("%v", strings.Join(bpList, " "))
		sendGdbCommand(es.gdbSession, "break-disable", commandArgs)
		for _, el := range bpList {
			bp, ok := es.breakpoints[el]
			if ok {
				bp.state = breakpointStateDisabled
			}
		}
	}
}

// convenience function
func disableGdbBreakpoint(es *engineState, bp string) {
	disableGdbBreakpoints(es, []string{bp})
}

// Note that not all "internal" breakpoints are stored in the breakpoints table
func disableAllGdbBreakpoints(es *engineState) {
	sendGdbCommand(es.gdbSession, "break-disable")
	for _, bp := range es.breakpoints {
		bp.state = breakpointStateDisabled
	}
}

func enableAllGdbBreakpoints(es *engineState) {
	sendGdbCommand(es.gdbSession, "break-enable")
	for _, bp := range es.breakpoints {
		bp.state = breakpointStateEnabled
	}
}

func enableGdbBreakpoints(es *engineState, bpList []string) {
	if len(bpList) > 0 {
		commandArgs := fmt.Sprintf("%v", strings.Join(bpList, " "))
		sendGdbCommand(es.gdbSession, "break-enable", commandArgs)
		for _, el := range bpList {
			bp, ok := es.breakpoints[el]
			if ok {
				bp.state = breakpointStateEnabled
			}
		}
	}
}

func getAssocEnabledPhpBreakpoint(es *engineState, filename string, lineno int) (string, bool) {
	for name, bp := range es.breakpoints {
		if bp.filename == filename &&
			bp.lineno == lineno &&
			bp.state == breakpointStateEnabled &&
			bp.bpType != breakpointTypeInternal {
			return name, true
		}
	}

	return "", false
}

// convenience function
func enableGdbBreakpoint(es *engineState, bp string) {
	enableGdbBreakpoints(es, []string{bp})
}

// Sets an equivalent breakpoint in gdb for PHP
// Also inserts the breakpoint into es.Breakpoints table
func setPhpBreakpointInGdb(es *engineState, phpFilename string, phpLineno int, disabled bool, temporary bool) (string, *engineBreakpointError) {
	internalLineno, ok := es.sourceMap[phpFilename]
	if !ok {
		warning := fmt.Sprintf("dontbug: Not able to find %v to add a breakpoint. Either the IDE is trying to set a breakpoint for a file from a different project (which is OK) or you need to run 'dontbug generate' specific to this project", phpFilename)
		color.Yellow(warning)
		return "", &engineBreakpointError{breakpointErrorCodeCouldNotSet, warning}
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
	result := sendGdbCommand(es.gdbSession,
		fmt.Sprintf("break-insert %v%v-f -c \"lineno == %v\" --source dontbug_break.c --line %v", temporaryFlag, disabledFlag, phpLineno, internalLineno))

	if result["class"] != "done" {
		warning := "Could not set breakpoint in gdb. Something is probably wrong with breakpoint parameters"
		color.Red(warning)
		return "", &engineBreakpointError{breakpointErrorCodeCouldNotSet, warning}
	}

	payload := result["payload"].(map[string]interface{})
	bkpt := payload["bkpt"].(map[string]interface{})
	id := bkpt["number"].(string)

	_, ok = es.breakpoints[id]
	if ok {
		log.Fatal("breakpoint number returned by gdb not unique: ", id)
	}

	es.breakpoints[id] = &engineBreakPoint{
		id:        id,
		filename:  phpFilename,
		lineno:    phpLineno,
		state:     breakpointState,
		temporary: temporary,
		bpType:    breakpointTypeLine,
	}

	return id, nil
}

// Does not make an entry in breakpoints table
func setPhpStackDepthLevelBreakpointInGdb(es *engineState, level int) string {
	if level > es.maxStackDepth {
		log.Fatalf("Max stack depth is %v but asked to set breakpoint at depth %v\n", es.maxStackDepth, level+1)
	}
	line := es.levelAr[level]

	result := sendGdbCommand(es.gdbSession, "break-insert",
		fmt.Sprintf("-f --source dontbug_break.c --line %v", line))

	if result["class"] != "done" {
		log.Fatal("Breakpoint was not set successfully")
	}

	payload := result["payload"].(map[string]interface{})
	bkpt := payload["bkpt"].(map[string]interface{})
	id := bkpt["number"].(string)

	return id
}

func removeGdbBreakpoint(es *engineState, id string) {
	sendGdbCommand(es.gdbSession, "break-delete", id)
	_, ok := es.breakpoints[id]
	if ok {
		delete(es.breakpoints, id)
	}
}

func gotoMasterBpLocation(es *engineState, reverse bool) (string, bool) {
	enableGdbBreakpoint(es, dontbugMasterBp)
	id, ok := continueExecution(es, reverse)
	disableGdbBreakpoint(es, dontbugMasterBp)
	return id, ok
}
