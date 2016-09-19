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
	"path"
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
	recordCmd.Flags().BoolP("php-cli-script", "p", false, "run PHP in cli mode instead of the PHP built in server")
	recordCmd.Flags().BoolP("take-snapshot", "s", false,
		`(Advanced/Experimental) Record after taking a snapshot of the PHP sources.
	                       Essentially, save execution trace *and* PHP sources. Recording can be replayed
	                       anytime in future; even when there have been intervening code changes. As
	                       most debugging sessions are after 'dontbug record', you may not need this
	                       feature in most cases.`)
	recordCmd.Flags().Int("server-port", dontbugDefaultPhpBuiltInServerPort, "default port for the PHP built in server")
	recordCmd.Flags().StringVar(&gServerListen, "server-listen", dontbugDefaultPhpBuiltInServerListen, "default listen ip address for the PHP built in server")
	recordCmd.Flags().StringVar(&gPhpExecutable, "with-php", "", "PHP (>= 7.0) executable to use (default is to use php found on $PATH)")
	recordCmd.Flags().Int("max-stack-depth", dontbugDefaultMaxStackDepth, "max depth of stack during execution")
	recordCmd.Flags().Int("record-port", dontbugDefaultRecordPort, "dbgp client/ide port for recording")
	recordCmd.Flags().StringVarP(&gArgs, "args", "a", "", "arguments (in quotes) to be passed to PHP script (requires --php-cli-script)")
}

// recordCmd represents the record command
var recordCmd = &cobra.Command{
	Use: `record <php-source-root-dir> [<docroot-dir>] [flags]
  dontbug record <php-source-root-dir> <php-script> --php-cli-script [args-in-quotes] [flags]

  Note: <docroot-dir> or <php-script> must be specified as a *relative* paths
  w.r.t to the <php-source-root-dir>. See Examples above.`,
	Short: "Start a PHP script/webserver and record execution for later debugging in a PHP IDE",
	Long: `
Dontbug Debugger version 0.1
Dontbug is a reversible debugger for PHP
Copyright (c) Sidharth Kshatriya 2016

dontbug record
~~~~~~~~~~~~~~

The 'dontbug record' command records the execution of PHP scripts in the PHP built-in webserver [1] or
in the PHP command line interpreter. This is used for later forward/reverse debugging in a PHP IDE.
A typical workflow is to 'dontbug record' followed by 'dontbug replay'.

Syntax
------
    dontbug record <php-source-root-dir> [<docroot-dir>] [flags]
    dontbug record <php-source-root-dir> <php-script> --php-cli-script [args-in-quotes] [flags]

Examples
--------
    dontbug record /var/www/fancy-site docroot
    dontbug record /var/www/another-site

    dontbug record ~/php-test/ list-supported-functions.php --php-cli-script
    dontbug record ~/php-test/ math/calculate-factorial-min-max.php --php-cli-script --args "10 20"

The first example will spawn the PHP built-in webserver for recording the execution of "fancy-site"
website (as the user navigates various URLs in a browser). The docroot of the fancy site will be
/var/www/fancy-site/docroot and the <php-source-dir> will be /var/www/fancy-site

In general, dontbug will be able to handle any PHP framework/CMS as long you meet its minimum requirements
and the framework/CMS runs in PHP's built in webserver (most of them should). Here the PHP built in webserver
is substituting for Apache, Nginx etc.

The second example is like the first. Here the <docroot-dir> is assumed to be the same as the
<php-source-root-dir>.

The third example will record the execution of ~/php-test/list-supported-functions.php

The fourth example will record the execution of a PHP script with two arguments 10 and 20 passed to it.
Note the quotes to enclose the arguments. The script's full path is ~/php-test/math/calculate-factorial-min-max.php

As you have seen _if_ you specify <docroot-dir> or <php-script> then it should specified as a *relative* path
w.r.t to the <php-source-root-dir>.

The <php-source-root-dir> means the outermost directory of all possible PHP scripts that might be executed
in this project by PHP sources in this project.

Note:
(1) Typically <php-source-root-dir> would be the docroot in your PHP project or, sometimes its parent folder.

<php-source-root-dir> is not the same as <docroot-dir>, sometimes, as scripts might be placed outside the docroot
in some PHP projects e.g. vendor scripts installed by composer. Please keep this directory as specific as possible.
For example, you _could_ specify "/" (the root directory) as <php-source-root-dir> as it
contains all the possible PHP scripts on your system. But this would impact performance hugely.

(2) If you have sources symlinked from inside the <php-source-root-dir> to outside that dir, dontbug should be
able to handle that (without you having to increase the scope of the <php-source-root-dir>)

PHP built-in webserver tips
---------------------------
You may record as many http page loads for later debugging when running the PHP built in webserver
(unlike traditional PHP debugging which is usually one page load at a time). However be aware that
recording too many page loads may degrade performance during debugging. Additionally, you
may _not_ pass arguments to the PHP built-in webserver i.e. the --args flag is ignored if not used in
conjunction with --php-cli-script.

Config file
-----------
If you find that you are frequently passing the same flags to dontbug, you may provide custom config for
various flags in a $HOME/.dontbug.yaml file. Sample file:

server-port: 8003
install-location: /some-path/src/github.com/sidkshatriya/dontbug

Most of the parameters defaults should suffice and you will typically need a very minimal .dontbug.yaml config file.

Flags passed via command line will always override any configuration in a .yaml file. If the .yaml
file and user flags don't specify a particular parameter, the defaults mentioned in
'dontbug record --help' will apply.

[1] https://secure.php.net/manual/en/features.commandline.webserver.php

                                    *-*-*
`,

	Run: func(cmd *cobra.Command, args []string) {
		engine.VerboseFlag = viper.GetBool("verbose")

		recordPort := viper.GetInt("record-port")
		serverPort := viper.GetInt("server-port")
		serverListen := viper.GetString("server-listen")
		maxStackDepth := viper.GetInt("max-stack-depth")
		installLocation := viper.GetString("install-location")
		rrExecutable := viper.GetString("with-rr")
		phpExecutable := viper.GetString("with-php")
		isCli := viper.GetBool("php-cli-script")
		arguments := viper.GetString("args")
		takeSnapshot := viper.GetBool("take-snapshot")

		if arguments != "" && !isCli {
			color.Yellow("dontbug: --args flag used but --php-cli-script flag not used. Ignoring --args flag")
		}

		docrootOrScriptRelPath := ""
		if len(args) < 1 {
			log.Fatal("Please provide the <php-source-root-dir> argument. See dontbug record --help for more details")
		} else if len(args) < 2 {
			if isCli {
				log.Fatal(`Please provide the script name as a path relative to the <php-source-root-dir> e.g. 'math/factorial.php'
See dontbug record --help for more details`)
			} else {
				color.Yellow("dontbug: No <docroot-dir> argument provided. Assuming its the same as <php-source-root-dir>")
				docrootOrScriptRelPath = "."
			}

		} else {
			docrootOrScriptRelPath = args[1]
			if path.IsAbs(docrootOrScriptRelPath) {
				log.Fatal(`Please provide a *relative* path for the docroot or php script argument e.g. '.', 'docroot', 'scriptDir/testing.php', 'hello.php'
See dontbug record --help for more details`)
			}
		}

		rootDir := args[0]
		engine.DoChecksAndRecord(
			phpExecutable,
			rrExecutable,
			rootDir,
			installLocation,
			docrootOrScriptRelPath,
			maxStackDepth,
			isCli,
			arguments,
			recordPort,
			serverListen,
			serverPort,
			takeSnapshot,
		)
	},
}
