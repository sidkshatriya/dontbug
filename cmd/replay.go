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
)

var (
	gTraceDir string
)

// replayCmd represents the replay command
var replayCmd = &cobra.Command{
	Use:   "replay",
	Short: "Replay and debug a previous execution",
	Run: func(cmd *cobra.Command, args []string) {
		if (len(gExtDir) <= 0) {
			color.Yellow("dontbug: No --ext-dir provided, assuming \"ext/dontbug\"")
			gExtDir = "ext/dontbug"
		}
		createBpLocMap(gExtDir)
		startReplay()
		// connectToDebuggerIDE()
	},
}

func init() {
	RootCmd.AddCommand(replayCmd)
	//	replayCmd.Flags().StringVar(&gTraceDir, "trace-dir", "", "")
}

func connectToDebuggerIDE() {
	_, err := net.Dial("tcp", "127.0.0.1:9000")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Connected to debugger IDE (aka \"client\")")
}

func createBpLocMap(extensionDir string) map[string]int {
	absExtDir := dirAbsPathOrFatalError(extensionDir)
	dontbugBreakFilename := absExtDir + "/dontbug_break.c"
	fmt.Println("dontbug: Looking for dontbug_break.c in", absExtDir)

	file, err := os.Open(dontbugBreakFilename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	fmt.Println("dontbug: found", dontbugBreakFilename)
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

func startReplay() {
	replaySession := exec.Command("rr", "replay", "-s", "9999")
	fmt.Println("using rr at:", replaySession.Path)
	f, err := pty.Start(replaySession)
	if err != nil {
		log.Fatal(err)
	}

	color.Green("dontbug: Successfully started replay session")

	buf := bufio.NewReader(f)

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
	for {
		line, _ := buf.ReadString('\n')
		fmt.Println(line)
		if strings.Contains(line, "target extended-remote") {
			cancel <- true
			slashAt := strings.Index(line, "/")

			hardlinkFile := strings.TrimSpace(line[slashAt:])
			go startGdb(hardlinkFile)
			break
		}
	}

	go io.Copy(os.Stdout, f)

	err = replaySession.Wait()
	if err != nil {
		log.Fatal(err)
	}
}

func startGdb(hardlinkFile string) (*gdb.Gdb, string) {
	gdbArgs := []string{"gdb", "-l", "-1", "-ex", "target extended-remote :9999", "--interpreter", "mi", hardlinkFile}
	fmt.Println("Starting gdb with the following string:", strings.Join(gdbArgs, " "))
	gdbSession, err := gdb.NewCmd(gdbArgs, nil)
	if err != nil {
		log.Fatal(err)
	}

	go io.Copy(os.Stdout, gdbSession)

	gdbCommand(gdbSession, "break-insert -f --source dontbug.c --line 94")
	gdbCommand(gdbSession, "exec-continue")
	result := gdbCommand(gdbSession, "data-evaluate-expression filename")
	payload, _ := result["payload"].(map[string]interface{})
	filename, ok := payload["value"].(string)
	if (ok) {
		return gdbSession, filename
	} else {
		log.Fatal("Could not get starting filename")
		return nil, ""
	}
}

func gdbCommand(gdbSession *gdb.Gdb, command string) map[string]interface{} {
	color.Green("dontbug ->%v", command)
	result, err := gdbSession.Send(command)
	if err != nil {
		log.Fatal(err)
	}
	color.Yellow("dontbug <-%v", result)
	return result
}