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
			log.Println("dontbug: No --ext-dir provided, assuming \"ext/dontbug\"")
			gExtDir = "ext/dontbug"
		}
		createBpLocMap(gExtDir)
		startReplay()
	},
}

func init() {
	RootCmd.AddCommand(replayCmd)
	replayCmd.Flags().StringVar(&gExtDir, "ext-dir", "", "")
	//	replayCmd.Flags().StringVar(&gTraceDir, "trace-dir", "", "")
}

func createBpLocMap(extensionDir string) map[string]int {
	absExtDir := dirAbsPathOrFatalError(extensionDir)
	dontbugBreakFilename := absExtDir + "/dontbug_break.c"
	log.Println("dontbug: Looking for dontbug_break.c in", absExtDir)

	file, err := os.Open(dontbugBreakFilename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	log.Println("dontbug: found", dontbugBreakFilename)
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

	log.Println("dontbug: Completed building association of filename and linenumbers for breakpoints")
	return bpLocMap
}

func startReplay() {
	replaySession := exec.Command("rr", "replay", "-s", "9999")

	f, err := pty.Start(replaySession)
	if err != nil {
		log.Fatal(err)
	}

	color.Set(color.FgGreen)
	log.Println("dontbug: Successfully started replay session")
	color.Unset()

	buf := bufio.NewReader(f)
	_, err = buf.ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}

	// Second line has gdb command
	line, err := buf.ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}

	// We are not interested in the contents of buf anymore
	// let it go to stdout
	go io.Copy(os.Stdout, f)

	if !strings.Contains(line, "target extended-remote") {
		log.Fatal("dontbug: could not ascertain remote debugging command from rr")
	}

	slashAt := strings.Index(line, "/")

	hardlinkFile := strings.TrimSpace(line[slashAt:])

	go startGdb(hardlinkFile)

	err = replaySession.Wait()
	if err != nil {
		log.Fatal(err)
	}
}

func startGdb(hardlinkFile string) {
	gdbArgs := []string{"gdb", "-l", "-1", "-ex", "target extended-remote :9999", "--interpreter", "mi", hardlinkFile}
	log.Println(strings.Join(gdbArgs, " "))
	gdbSession, err := gdb.NewCmd(gdbArgs, nil)
	if err != nil {
		log.Fatal(err)
	}

	go io.Copy(os.Stdout, gdbSession)

//	gdbCommand(gdbSession, "file-symbol-file " + hardlinkFile)
	gdbCommand(gdbSession, "break-insert -f --source dontbug.c --line 94")
	gdbCommand(gdbSession, "exec-continue")
	gdbCommand(gdbSession, "data-evaluate-expression filename")
	gdbSession.Exit()
}

func gdbCommand(gdbSession *gdb.Gdb, command string) {
	color.Set(color.FgGreen)
	log.Println("->", command)
	color.Unset()
	result, err := gdbSession.Send(command)
	if err != nil {
		log.Fatal(err)
	}
	color.Set(color.FgGreen)
	log.Println("<-", result)
	color.Unset()
}