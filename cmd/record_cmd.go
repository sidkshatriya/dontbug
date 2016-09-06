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
	"github.com/sidkshatriya/dontbug/engine"
	"github.com/fatih/color"
	"log"
	"github.com/spf13/viper"
)

const (
	dontbugDefaultRecordPort int = 9001
	dontbugDefaultPhpBuiltInServerPort int = 8088
	dontbugDefaultPhpBuiltInServerListen string = "127.0.0.1"
)

var (
	gServerListen string
)

func init() {
	RootCmd.AddCommand(recordCmd)
	recordCmd.Flags().Int("record-port", dontbugDefaultRecordPort, "dbgp client/ide port for recording")
	recordCmd.Flags().Int("server-port", dontbugDefaultPhpBuiltInServerPort, "default port for the PHP built in server")
	recordCmd.Flags().StringVar(&gServerListen, "server-listen", dontbugDefaultPhpBuiltInServerListen, "default listen ip for the PHP built in server")
	recordCmd.Flags().Int("max-stack-depth", dontbugDefaultMaxStackDepth, "max depth of stack during execution")

}

// recordCmd represents the record command
var recordCmd = &cobra.Command{
	Use:   "record <php-source-root-path> [<docroot>]",
	Short: "start the built in PHP server and record execution",
	Run: func(cmd *cobra.Command, args []string) {
		recordPort := viper.GetInt("record-port")
		serverPort := viper.GetInt("server-port")
		serverListen := viper.GetString("server-listen")
		maxStackDepth := viper.GetInt("max-stack-depth")
		installLocation := viper.GetString("install-location")
		rr_executable := viper.GetString("rr-executable")

		// @TODO check if this a valid install location?
		color.Yellow("dontbug: Using --install-location \"%v\"", installLocation)
		extDir := installLocation + "/ext/dontbug"

		docroot := ""
		if len(args) < 1 {
			log.Fatal("Please provide the PHP source root path (this is usually the docroot or its parent directory). Note: No PHP sources should lie outside the source root path for this particular site")
		} else if len(args) < 2 {
			docroot = args[0] + "/docroot"
			color.Yellow("dontbug: docroot not provided. Assuming %v", docroot)
		} else {
			docroot = args[1]
		}

		engine.DoGeneration(args[0], extDir, maxStackDepth)
		dlPath := engine.CheckDontbugWasCompiled(extDir)
		engine.StartBasicDebuggerClient(recordPort)
		engine.DoRecordSession(docroot, dlPath, rr_executable, serverListen, serverPort, recordPort)
	},
}
