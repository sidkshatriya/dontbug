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
	"log"
)

const (
	dontbugDefaultRecordPort             int    = 9001
	dontbugDefaultPhpBuiltInServerPort   int    = 8088
	dontbugDefaultPhpBuiltInServerListen string = "127.0.0.1"
)

var (
	gServerListen  string
	gPhpExecutable string
	gArgs          string
)

func init() {
	RootCmd.AddCommand(recordCmd)
	recordCmd.Flags().BoolP("php-cli-script", "c", false, "run PHP in cli mode instead of the PHP built in server (which is the default)")
	recordCmd.Flags().BoolP("with-snapshot", "s", false, "record after taking a snapshot of the PHP sources so the recording can be replayed anytime in the future -- even when there have been intervening code changes")
	recordCmd.Flags().Int("server-port", dontbugDefaultPhpBuiltInServerPort, "default port for the PHP built in server")
	recordCmd.Flags().StringVar(&gServerListen, "server-listen", dontbugDefaultPhpBuiltInServerListen, "default listen ip address for the PHP built in server")
	recordCmd.Flags().StringVar(&gPhpExecutable, "php-executable", "", "PHP executable to use (default is to use php found on $PATH)")
	recordCmd.Flags().Int("max-stack-depth", dontbugDefaultMaxStackDepth, "max depth of stack during execution")
	recordCmd.Flags().Int("record-port", dontbugDefaultRecordPort, "dbgp client/ide port for recording")
	recordCmd.Flags().StringVar(&gArgs, "args", "", "arguments (in quotes) to be passed to PHP script (requires use of --php-cli-script flag)")
}

// recordCmd represents the record command
var recordCmd = &cobra.Command{
	Use:   "record <php-source-root-directory> <docroot-dir>|<php-script>",
	Short: "Start a PHP script/webserver and record execution for later debugging in a PHP IDE",
	Long: `
The 'dontbug record' command records the execution of PHP scripts or the PHP built-in webserver to be used
for later forward/reverse debugging in a PHP IDE.

    dontbug record <php-source-root-directory> <docroot-dir>|<php-script> [flags]

Examples:

    dontbug record /var/www/fancy-site /var/www/fancy-site/docroot
    dontbug record ~/php-test/ ~/php-test/calculate-factorial-min-max.php --php-cli-script --args "10 20"

The first example will spawn the PHP built-in webserver for recording the execution of "fancy-site"
website (as the user navigates various URLs in a browser). dontbug will be able to handle any PHP
framework or CMS as long as you have installed dontbug properly and meet its minimum requirements.

The second example will record the execution of a PHP script with two arguments 10 and 20 passed to it.
Note the quotes to enclose the arguments.

The <php-source-root-directory> means the directory of all possible PHP scripts that might be executed
in this project. Note that this is _not_ the same as all the scripts in, say, docroot as scripts might
be placed outside the docroot in some PHP projects e.g. vendor scripts installed by composer. Please keep
this directory as minimal as possible. For example, you _could_ specify "/" (the root directory) as
<php-source-root-directory> as it contains all the possible PHP scripts on your system. But this would
impact performance hugely. Typically this directory would be the docroot in your PHP project or its
parent folder (if some vendor PHP scripts etc. are stored there).

PHP built-in webserver tips:

You may record as many http page loads for later debugging when running the PHP built in webserver
(unlike traditional PHP debugging which is usually one page load at a time). However be aware that
recording too many page loads may degrade performance when setting breakpoints. Additionally, you
may _not_ pass arguments to scripts that will be run in the PHP built in server i.e. the --args
flag is ignored if not used in conjunction with --php-cli-script.
`,

	Run: func(cmd *cobra.Command, args []string) {
		engine.VerboseFlag = viper.GetBool("verbose")

		recordPort := viper.GetInt("record-port")
		serverPort := viper.GetInt("server-port")
		serverListen := viper.GetString("server-listen")
		maxStackDepth := viper.GetInt("max-stack-depth")
		installLocation := viper.GetString("install-location")
		rrExecutable := viper.GetString("rr-executable")
		phpExecutable := viper.GetString("php-executable")
		isCli := viper.GetBool("php-cli-script")
		arguments := viper.GetString("args")
		withSnapshot := viper.GetBool("with-snapshot")

		if arguments != "" && !isCli {
			color.Yellow("--args flag used but --php-cli-script flag not used. Ignoring --args flag")
		}

		// @TODO check if this a valid install location?
		color.Yellow("dontbug: Using --install-location \"%v\"", installLocation)
		extDir := installLocation + "/ext/dontbug"

		docrootOrScript := ""
		if len(args) < 1 {
			log.Fatal("Please provide the PHP source root path. Note: No PHP sources should lie outside the source root path that will be used by this project")
		} else if len(args) < 2 {
			log.Fatal("Please provide the docroot dir/script name")
		} else {
			docrootOrScript = args[1]
		}

		rootDir := args[0]
		if withSnapshot {
			color.Yellow("--with-snapshot option used. %v needs to be a git repository or program will exit with fatal error", rootDir)
		}

		engine.DoChecksAndRecord(
			phpExecutable,
			rrExecutable,
			rootDir,
			extDir,
			docrootOrScript,
			maxStackDepth,
			isCli,
			arguments,
			recordPort,
			serverListen,
			serverPort,
			withSnapshot,
		)
	},
}
