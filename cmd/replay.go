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
)

var (
	gTraceDir string
)

var gInitXMLformat string =
	`<?xml version="1.0" encoding="iso-8859-1"?>
	<init xmlns="urn:debugger_protocol_v1"
		fileuri="file://%v"
		language="PHP"
		protocol_version="1.0"
		appid="%v" idekey="dontbug">
		<engine version="0.0.1"><![CDATA[dontbug]]></engine>
	</init>`

var gFeatureSetResponseFormat =
	`<?xml version="1.0" encoding="iso-8859-1"?>
	<response xmlns="urn:debugger_protocol_v1" command="feature_set" transaction_id="%v" feature="%v" success="%v"></response>`

type DbgpCmd struct {
	Command  string
	Options  map[string]string
	Sequence int
}

type DebugEngineState struct {
	GdbSession      *gdb.Gdb
	EntryFilePHP    string
	LastSequenceNum int
	FeatureMap      map[string]FeatureValue
}

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
		"language_version" : &FeatureString{"7.0", true},
		"encoding" : &FeatureString{"ISO-8859-1", true},
		"protocol_version" : &FeatureInt{1, true},
		"supports_async" : &FeatureBool{false, true},
		"breakpoint_types" : &FeatureString{"line call return exception conditional watch", true},
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
		if (len(gExtDir) <= 0) {
			color.Yellow("dontbug: No --ext-dir provided, assuming \"ext/dontbug\"")
			gExtDir = "ext/dontbug"
		}

		if len(args) < 1 {
			color.Yellow("dontbug: No trace directory provided, latest-trace trace directory assumed")
			gTraceDir = ""
		} else {
			gTraceDir = args[0]
		}

		bpMap := constructBreakpointLocMap(gExtDir)
		engineState := startReplayInRR(gTraceDir)
		debuggerIdeCmdLoop(engineState, bpMap)
	},
}

func init() {
	RootCmd.AddCommand(replayCmd)
}

func debuggerIdeCmdLoop(engineState *DebugEngineState, bpMap map[string]int) {
	color.Yellow("dontbug: Trying to connect to debugger IDE")
	conn, err := net.Dial("tcp", ":9000")
	if err != nil {
		log.Fatal(err)
	}
	color.Green("dontbug: Connected to debugger IDE (aka \"client\")")

	// send the init packet
	payload := fmt.Sprintf(gInitXMLformat, engineState.EntryFilePHP, os.Getpid())
	packet := constructDbgpPacket(payload)
	conn.Write(packet)
	color.Green("dontbug -> %v", payload)

	buf := bufio.NewReader(conn)
	for {
		command, err := buf.ReadString(byte(0))
		command = strings.TrimRight(command, "\x00")
		if err != nil {
			log.Fatal(err)
		}
		color.Cyan("dontbug <- %v", command)
		payload = handleIdeRequest(engineState, command)
		conn.Write(constructDbgpPacket(handleIdeRequest(engineState, command)))
		color.Green("dontbug -> %v", payload)
	}
}

func constructDbgpPacket(payload string) []byte {
	var buf bytes.Buffer
	buf.WriteString(strconv.Itoa(len(payload)))
	buf.Write([]byte{0})
	buf.WriteString(payload)
	buf.Write([]byte{0})
	return buf.Bytes()
}

func handleIdeRequest(es *DebugEngineState, command string) string {
	dbgpCmd := parseCommand(command)
	if es.LastSequenceNum > dbgpCmd.Sequence {
		log.Fatal("Sequence number", dbgpCmd.Sequence, "has already been seen")
	}

	switch(dbgpCmd.Command) {
	case "feature_set":
		return handleFeatureSet(es, dbgpCmd)
	default:
		fmt.Println(es.FeatureMap)
		log.Fatal("Unimplemented command:", command)
	}

	return ""
}

func handleFeatureSet(es *DebugEngineState, dCmd DbgpCmd) string {
	n, ok := dCmd.Options["n"]
	if !ok {
		log.Fatal("Not provided n option in feature_set")
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

func parseCommand(command string) DbgpCmd {
	components := strings.Fields(command)
	flags := make(map[string]string)
	command = components[0]
	for i, v := range components[1:] {
		if (i % 2 == 1) {
			continue
		}

		// Also remove the leading "-" in the flag via [1:]
		flags[strings.TrimSpace(v)[1:]] = strings.TrimSpace(components[i + 2])
	}

	seq, ok := flags["i"]
	if !ok {
		log.Fatal("Could not find sequence number in command")
	}

	seqInt, err := strconv.Atoi(seq)
	if err != nil {
		log.Fatal(err)
	}

	return DbgpCmd{command, flags, seqInt}
}

func constructBreakpointLocMap(extensionDir string) map[string]int {
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
	lineno := 0
	for {
		line, err := buf.ReadString('\n')
		lineno++
		if err == io.EOF {
			break
		} else if (err != nil) {
			log.Fatal(err)
		}

		index := strings.Index(line, "//###")
		if index == -1 {
			continue
		}

		filename := strings.TrimSpace(line[index + 5:])
		bpLocMap[filename] = lineno
	}

	fmt.Println("dontbug: Completed building association of filename and linenumbers for breakpoints")
	return bpLocMap
}

func startReplayInRR(traceDir string) *DebugEngineState {
	absTraceDir := ""
	if len(traceDir) > 0 {
		absTraceDir = getDirAbsPath(traceDir)
	}

	// Start an rr replay session
	replaySession := exec.Command("rr", "replay", "-s", "9999", absTraceDir)
	fmt.Println("dontbug: Using rr at:", replaySession.Path)
	f, err := pty.Start(replaySession)
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
			log.Fatal("could not find gdb connection string")
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
			return startGdb(hardlinkFile)
		}
	}

	// @TODO is this correct??
	go func() {
		err := replaySession.Wait()
		if err != nil {
			log.Fatal(err)
		}
	}()

	return nil
}

// Starts gdb and creates a new DebugEngineState object
func startGdb(hardlinkFile string) *DebugEngineState {
	gdbArgs := []string{"gdb", "-l", "-1", "-ex", "target extended-remote :9999", "--interpreter", "mi", hardlinkFile}
	fmt.Println("dontbug: Starting gdb with the following string:", strings.Join(gdbArgs, " "))
	gdbSession, err := gdb.NewCmd(gdbArgs, nil)
	if err != nil {
		log.Fatal(err)
	}

	go io.Copy(os.Stdout, gdbSession)

	// @TODO remove this hardcoded number
	sendGdbCommand(gdbSession, "break-insert -f --source dontbug.c --line 94")
	sendGdbCommand(gdbSession, "exec-continue")
	result := sendGdbCommand(gdbSession, "data-evaluate-expression filename")
	payload, _ := result["payload"].(map[string]interface{})
	filename, ok := payload["value"].(string)
	if (ok) {
		return &DebugEngineState{
			GdbSession: gdbSession,
			EntryFilePHP:parseGdbStringResponse(filename),
			FeatureMap:initFeatureMap(),
			LastSequenceNum:0}
	} else {
		log.Fatal("Could not get starting filename")
		return nil
	}
}

// a gdb string response looks like '0x7f261d8624e8 "some string here"'
func parseGdbStringResponse(gdbResponse string) string {
	first := strings.Index(gdbResponse, "\"")
	last := strings.LastIndex(gdbResponse, "\"")
	if (first == -1 || last == -1 || first == last) {
		log.Fatal("Improper gdb data-evaluate-expression string response")
	}

	return gdbResponse[first + 1:last]
}

func sendGdbCommand(gdbSession *gdb.Gdb, command string) map[string]interface{} {
	color.Green("dontbug -> %v", command)
	result, err := gdbSession.Send(command)
	if err != nil {
		log.Fatal(err)
	}
	color.Cyan("dontbug <- %v", result)
	return result
}
