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
)

func init() {
	RootCmd.AddCommand(recordCmd)
}

// recordCmd represents the record command
var recordCmd = &cobra.Command{
	Use:   "record [php-source-root-path] [optional-docroot]",
	Short: "start the built in PHP server and record execution",
	Run: func(cmd *cobra.Command, args []string) {
		if len(gExtDir) == 0 {
			color.Yellow("dontbug: No --ext-dir provided, assuming \"./ext/dontbug\"")
			gExtDir = "ext/dontbug"
		}

		docroot := ""
		if len(args) < 1 {
			log.Fatal("Please provide the PHP source root path (this is usually the docroot or its parent directory). Note: No PHP sources should lie outside the source root path for this particular site")
		} else if len(args) < 2 {
			docroot = args[0] + "/docroot"
			color.Yellow("dontbug: docroot not provided. Assuming %v", docroot)
		} else {
			docroot = args[1]
		}

		engine.DoGeneration(args[0], gExtDir)
		dlPath := engine.CheckDontbugWasCompiled(gExtDir)
		engine.StartBasicDebuggerClient()
		engine.DoRecordSession(docroot, dlPath)
	},
}
