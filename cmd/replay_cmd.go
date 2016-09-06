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
	"github.com/spf13/viper"
)

const (
	dontbugDefaultReplayPort int = 9000
	dontbugDefaultGdbExtendedRemotePort int = 9999
)

var (
	gGdbExecutableFlag string
)

// replayCmd represents the replay command
var replayCmd = &cobra.Command{
	Use:   "replay [<trace-dir>]",
	Short: "Replay and debug a previous execution",
	Run: func(cmd *cobra.Command, args []string) {
		engine.Verbose = viper.GetBool("verbose")
		engine.ShowGdbNotifications = viper.GetBool("gdb-notify")

		replayPort := viper.GetInt("replay-port")
		installLocation := viper.GetString("install-location")
		targedExtendedRemotePort := viper.GetInt("gdb-remote-port")
		rr_executable := viper.GetString("rr-executable")
		gdb_executable := viper.GetString("gdb-executable")

		// @TODO check if this a valid install location?
		color.Yellow("dontbug: Using --install-location \"%v\"", installLocation)
		extDir := installLocation + "/ext/dontbug"

		traceDir := ""
		if len(args) < 1 {
			color.Yellow("dontbug: No trace directory provided, latest-trace trace directory assumed")
		} else {
			traceDir = args[0]
		}

		engine.DoReplay(extDir, traceDir, rr_executable, gdb_executable, replayPort, targedExtendedRemotePort)
	},
}

func init() {
	RootCmd.AddCommand(replayCmd)
	replayCmd.Flags().BoolP("verbose", "v", false, "show messages between dontbug, gdb and the ide")
	replayCmd.Flags().BoolP("gdb-notify", "g", false, "show notification messages from gdb")
	replayCmd.Flags().Int("replay-port", dontbugDefaultReplayPort, "dbgp client/ide port for replaying")
	replayCmd.Flags().Int("gdb-remote-port", dontbugDefaultGdbExtendedRemotePort, "port at which rr backend should be made available to gdb")
	replayCmd.Flags().StringVar(&gGdbExecutableFlag, "gdb-executable", "", "the gdb (>= 7.11.1) executable (with the full path) (default is to assume gdb exists in $PATH)")
}
