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