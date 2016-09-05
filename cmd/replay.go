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
	"github.com/fatih/color"
	"github.com/sidkshatriya/dontbug/engine"
)

var (
	gNoisyPtr *bool
	gGdbNotificationsPtr *bool
	gTraceDir string
)

// replayCmd represents the replay command
var replayCmd = &cobra.Command{
	Use:   "replay [optional-trace-dir]",
	Short: "Replay and debug a previous execution",
	Run: func(cmd *cobra.Command, args []string) {
		engine.Noisy = *gNoisyPtr
		engine.ShowGdbNotifications = *gGdbNotificationsPtr

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

		bpMap, levelAr := engine.ConstructBreakpointLocMap(gExtDir)
		engineState := engine.StartReplayInRR(gTraceDir, bpMap, levelAr)
		engine.DebuggerIdeCmdLoop(engineState)
		engineState.RRCmd.Wait()
	},
}

func init() {
	RootCmd.AddCommand(replayCmd)
	gNoisyPtr = replayCmd.Flags().BoolP("verbose", "v", false, "show messages between dontbug, gdb and the ide")
	gGdbNotificationsPtr = replayCmd.Flags().BoolP("gdb-notify", "g", false, "show notification messages from gdb")
}
