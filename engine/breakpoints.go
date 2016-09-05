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
	"strings"
	"log"
	"io"
	"fmt"
	"os"
	"bufio"
	"strconv"
	"github.com/fatih/color"
	"errors"
)

type BreakpointError struct {
	Code    int
	Message string
}

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

func ConstructBreakpointLocMap(extensionDir string) (map[string]int, [maxLevels]int) {
	absExtDir := GetDirAbsPath(extensionDir)
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

	return fmt.Sprintf(BreakpointRemoveOrUpdateXmlResponseFormat, "breakpoint_update", dCmd.Sequence)
}

func handleBreakpointRemove(es *DebugEngineState, dCmd DbgpCmd) string {
	d, ok := dCmd.Options["d"]
	if !ok {
		log.Fatal("Please provide breakpoint id to remove")
	}

	removeGdbBreakpoint(es, d)

	return fmt.Sprintf(BreakpointRemoveOrUpdateXmlResponseFormat, "breakpoint_remove", dCmd.Sequence)
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
		return fmt.Sprintf(ErrorXmlResponseFormat, "breakpoint_set", dCmd.Sequence, 201, "Hit condition/value is currently not supported")
	}

	_, ok = dCmd.Options["o"]
	if ok {
		return fmt.Sprintf(ErrorXmlResponseFormat, "breakpoint_set", dCmd.Sequence, 201, "Hit condition/value is currently not supported")
	}

	phpLineno, err := strconv.Atoi(phpLinenoString)
	if err != nil {
		log.Fatal(err)
	}

	id, breakErr := setPhpBreakpointInGdb(es, phpFilename, phpLineno, disabled, temporary)
	if breakErr != nil {
		return fmt.Sprintf(ErrorXmlResponseFormat, "breakpoint_set", dCmd.Sequence, breakErr.Code, breakErr.Message)
	}

	return fmt.Sprintf(BreakpointSetLineXmlResponseFormat, dCmd.Sequence, status, id)
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
		return fmt.Sprintf(ErrorXmlResponseFormat, "breakpoint_set", dCmd.Sequence, 201, "Breakpoint type " + tt + " is not supported")
	}

	return ""
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
