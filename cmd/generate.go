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
	"log"
	"github.com/sidkshatriya/dontbug/engine"
	"github.com/fatih/color"
)
// generateCmd represents the generate command
var generateCmd = &cobra.Command{
	Use:   "generate [root-directory]",
	Short: "Generate debug_break.c",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			log.Fatal("Please provide root directory of PHP source files on the command line")
		}

		if (len(gExtDir) <= 0) {
			color.Yellow("dontbug: No --ext-dir provided, assuming \"./ext/dontbug\"")
			gExtDir = "ext/dontbug"
		}

		engine.DoGeneration(args[0], gExtDir)
	},
}

func init() {
	RootCmd.AddCommand(generateCmd)
}