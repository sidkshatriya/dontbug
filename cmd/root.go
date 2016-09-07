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
	"fmt"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
)

const (
	dontbugDefaultMaxStackDepth = 128
)

var (
	cfgFile              string
	gInstallLocationFlag string
	gRRExecutableFlag    string
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "dontbug",
	Short: "Dontbug is a reversible debugger for PHP.\nCopyright (c) Sidharth Kshatriya 2016",
}

// Execute adds all child commands to the root command sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	RootCmd.PersistentFlags().BoolP("verbose", "v", false, "print more messages to know what dontbug is doing")
	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.dontbug.yaml)")
	RootCmd.PersistentFlags().StringVar(&gInstallLocationFlag, "install-location", ".", "location of dontbug folder")
	RootCmd.PersistentFlags().StringVar(&gRRExecutableFlag, "rr-executable", "", "the rr executable (with the full path) (default is assume rr exists in $PATH)")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// enable ability to specify config file via flag
		viper.SetConfigFile(cfgFile)
	}

	viper.SetConfigName(".dontbug") // name of config file (without extension)
	viper.AddConfigPath("$HOME")    // adding home directory as first search path
	viper.AutomaticEnv()            // read in environment variables that match
	viper.SetConfigType("yaml")

	viper.BindPFlag("record-port", recordCmd.Flags().Lookup("record-port"))
	viper.BindPFlag("server-port", recordCmd.Flags().Lookup("server-port"))
	viper.BindPFlag("server-listen", recordCmd.Flags().Lookup("server-listen"))
	viper.BindPFlag("max-stack-depth", recordCmd.Flags().Lookup("max-stack-depth"))
	viper.BindPFlag("php-executable", recordCmd.Flags().Lookup("php-executable"))
	viper.BindPFlag("php-cli-script", recordCmd.Flags().Lookup("php-cli-script"))
	viper.BindPFlag("args", recordCmd.Flags().Lookup("args"))

	viper.BindPFlag("replay-port", replayCmd.Flags().Lookup("replay-port"))
	viper.BindPFlag("gdb-notify", replayCmd.Flags().Lookup("gdb-notify"))
	viper.BindPFlag("gdb-remote-port", replayCmd.Flags().Lookup("gdb-remote-port"))
	viper.BindPFlag("gdb-executable", replayCmd.Flags().Lookup("gdb-executable"))

	viper.BindPFlag("install-location", RootCmd.Flags().Lookup("install-location"))
	viper.BindPFlag("rr-executable", RootCmd.Flags().Lookup("rr-executable"))
	viper.BindPFlag("verbose", RootCmd.Flags().Lookup("verbose"))

	viper.SetDefault("rr-executable", "rr")
	viper.SetDefault("gdb-executable", "gdb")
	viper.SetDefault("php-executable", "php")
	viper.SetDefault("php-cli-script", false)
	viper.SetDefault("args", "")

	viper.RegisterAlias("record_port", "record-port")
	viper.RegisterAlias("server_port", "server-port")
	viper.RegisterAlias("server_listen", "server-listen")
	viper.RegisterAlias("gdb_notify", "gdb-notify")
	viper.RegisterAlias("replay_port", "replay-port")
	viper.RegisterAlias("max_stack_depth", "max-stack-depth")
	viper.RegisterAlias("install_location", "install-location")
	viper.RegisterAlias("gdb_remote_port", "gdb-remote-port")
	viper.RegisterAlias("gdb_executable", "gdb-executable")
	viper.RegisterAlias("rr_executable", "rr-executable")
	viper.RegisterAlias("php_executable", "php-executable")
	viper.RegisterAlias("php_cli_script", "php-cli-script")
	viper.RegisterAlias("arguments", "args")
	viper.RegisterAlias("argument", "args")
	viper.RegisterAlias("arg", "args")

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		color.Yellow("dontbug: Using config file:%v", viper.ConfigFileUsed())
	}
}
