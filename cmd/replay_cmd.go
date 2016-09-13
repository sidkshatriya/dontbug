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
	"github.com/fatih/color"
	"github.com/sidkshatriya/dontbug/engine"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	dontbugDefaultReplayPort            int = 9000
	dontbugDefaultGdbExtendedRemotePort int = 9999
)

var (
	gGdbExecutableFlag string
)

// replayCmd represents the replay command
var replayCmd = &cobra.Command{
	Use:   "replay [<full-snapshot-tagname-or-any-portion-of-tagname>|latest|list]",
	Short: "Replay and debug a previous execution",
	Run: func(cmd *cobra.Command, args []string) {
		engine.VerboseFlag = viper.GetBool("verbose")
		engine.ShowGdbNotifications = viper.GetBool("gdb-notify")

		replayPort := viper.GetInt("replay-port")
		installLocation := viper.GetString("install-location")
		targedExtendedRemotePort := viper.GetInt("gdb-remote-port")
		rrExecutable := viper.GetString("rr-executable")
		gdbExecutable := viper.GetString("gdb-executable")

		// @TODO check if this a valid install location?
		color.Yellow("dontbug: Using --install-location \"%v\"", installLocation)
		extDir := installLocation + "/ext/dontbug"

		snapshotTagnamePortion := ""
		if len(args) >= 1 {
			snapshotTagnamePortion = args[0]
		}

		rrPath := engine.CheckRRExecutable(rrExecutable)
		gdbPath := engine.CheckGdbExecutable(gdbExecutable)

		engine.DoReplay(
			extDir,
			snapshotTagnamePortion,
			rrPath,
			gdbPath,
			replayPort,
			targedExtendedRemotePort,
		)
	},
}

func init() {
	RootCmd.AddCommand(replayCmd)
	replayCmd.Flags().BoolP("gdb-notify", "g", false, "show notification messages from gdb")
	replayCmd.Flags().Int("replay-port", dontbugDefaultReplayPort, "dbgp client/ide port for replaying")
	replayCmd.Flags().Int("gdb-remote-port", dontbugDefaultGdbExtendedRemotePort, "port at which rr backend should be made available to gdb")
	replayCmd.Flags().StringVar(&gGdbExecutableFlag, "gdb-executable", "", "the gdb (>= 7.11.1) executable (with the full path) (default is to assume gdb exists in $PATH)")
}
