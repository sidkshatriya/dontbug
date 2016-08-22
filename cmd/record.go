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
	"github.com/spf13/cobra"
	"os/exec"
	"log"
	"github.com/kr/pty"
	"io"
	"os"
	"net"
	"bytes"
	"strconv"
	"github.com/fatih/color"
	"os/signal"
	"strings"
)

// recordCmd represents the record command
var recordCmd = &cobra.Command{
	Use:   "record [server-docroot-path]",
	Short: "start the built in PHP server and record execution",
	Run: func(cmd *cobra.Command, args []string) {
		if (len(gExtDir) <= 0) {
			color.Yellow("dontbug: No --ext-dir provided, assuming \"ext/dontbug\"")
			gExtDir = "ext/dontbug"
		}

		dlPath := checkDontbugWasCompiled(gExtDir)
		startBasicDebuggerClient()
		if len(args) < 1 {
			color.Yellow("dontbug: no PHP built-in cli server docroot path provided. Assuming \".\" ")
			doRecordSession(".", dlPath)
		} else {
			doRecordSession(args[0], dlPath)
		}
	},
}

func init() {
	RootCmd.AddCommand(recordCmd)
}

func doRecordSession(docroot, dlPath string) {
	docrootAbsPath := getDirAbsPath(docroot)
	rr_cmd := []string{"record", "php", "-S", "127.0.0.1:8088", "-d", "extension=" + dlPath, "-t", docrootAbsPath}
	fmt.Println("dontbug: Issuing command: rr", strings.Join(rr_cmd, " "))
	recordSession := exec.Command("rr", rr_cmd...)
	fmt.Println("dontbug: Using the following rr:", recordSession.Path)

	f, err := pty.Start(recordSession)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("dontbug: Successfully started recording session... Press Ctrl-C to terminate recording")
	go io.Copy(os.Stdout, f)

	// Handle a Ctrl+C
	// If we don't do this rr will terminate abruptly and not save the execution traces properly
	c := make(chan os.Signal)
	defer close(c)

	signal.Notify(c, os.Interrupt) // Ctrl+C
	go func() {
		<-c
		color.Yellow("dontbug: Sending a Ctrl+C to recording")
		f.Write([]byte{3}) // Ctrl+C is ASCII code 3
		signal.Stop(c)
	}()

	err = recordSession.Wait()
	if err != nil {
		log.Fatal(err)
	}

	color.Green("dontbug: Closed cleanly after terminating PHP built-cli server. Replay should work properly")
}

func startBasicDebuggerClient() {
	listener, err := net.Listen("tcp", "127.0.0.1:9000")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("dontbug: Dontbug DBGp debugger client is listening on 127.0.0.1:9000 for connections from PHP")
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Fatal(err)
			}

			go func(conn net.Conn) {
				buf := make([]byte, 2048)
				seq := 0
				for {
					bytesRead, _ := conn.Read(buf)
					if (bytesRead <= 0) {
						return
					}

					nullAt := bytes.IndexByte(buf, byte(0))
					if nullAt == -1 {
						log.Fatal("Could not find length in debugger engine response")
					}

					dataLen, err := strconv.Atoi(string(buf[0:nullAt]))
					if err != nil {
						log.Fatal(err)
					}

					bytesLeft := dataLen - (bytesRead - nullAt - 2)
					// fmt.Println("bytes_left:", bytes_left, "data_len:", data_len, "bytes_read:", bytes_read, "null_at:", null_at)
					if bytesLeft != 0 {
						log.Fatal("There are still some bytes left to receive -- strange")
					}

					//color.Green("dontbug <-%v", string(buf[nullAt + 1:bytesRead - 1]))
					color.Green("dontbug <-%v", string(buf[:bytesRead]))
					seq++

					// Keep running until we are able to record the execution
					runCommand := fmt.Sprintf("run -i %d\x00", seq)
					color.Yellow("dontbug ->%v", runCommand)
					conn.Write([]byte(runCommand))
				}
			}(conn)
		}
	}()
}

func checkDontbugWasCompiled(extDir string) string {
	extDirAbsPath := getDirAbsPath(gExtDir)
	dlPath := extDirAbsPath + "/.libs/dontbug.so"

	// Does the zend extension exist?
	_, err := os.Stat(dlPath)
	if err != nil {
		color.Yellow("Not able to find dontbug.so please run 'dontbug generate' to generate it")
		log.Fatal(err)
	}

	return dlPath
}
