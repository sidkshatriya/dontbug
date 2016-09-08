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

package engine

import (
	"bytes"
	"fmt"
	"github.com/fatih/color"
	"github.com/kr/pty"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
)

// Assumptions:
// - rrPath represents an rr executable that meets dontbug's requirements
// - phpPath represents an php executable that meets dontbug's requirements
// - sharedObject path is the path to xdebug.so that meets dontbug's requirements
// - docrootDirOrScript is a valid docroot directory or a php script
func DoRecordSession(docrootDirOrScript, sharedObjectPath, rrPath, phpPath string, isCli bool, arguments, serverListen string, serverPort, recordPort int) {
	// @TODO remove this check and move to separate function
	docrootOrScriptAbsPath := getAbsPathOrFatal(docrootDirOrScript)

	rrCmd := []string{"record", phpPath,
		"-d", "zend_extension=xdebug.so",
		"-d", "zend_extension=" + sharedObjectPath,
		"-d", fmt.Sprintf("xdebug.remote_port=%v", recordPort),
		"-d", "xdebug.remote_autostart=1",
		"-d", "xdebug.remote_enable=1",
	}

	if isCli {
		arguments = strings.TrimSpace(arguments)
		rrCmd = append(rrCmd, docrootOrScriptAbsPath)
		if arguments != "" {
			argumentsAr := strings.Split(arguments, " ")
			rrCmd = append(rrCmd, argumentsAr...)
		}
	} else {
		rrCmd = append(
			rrCmd,
			"-S", fmt.Sprintf("%v:%v", serverListen, serverPort),
			"-t", docrootOrScriptAbsPath)
	}

	fmt.Println("dontbug: Issuing command: rr", strings.Join(rrCmd, " "))
	recordSession := exec.Command(rrPath, rrCmd...)

	f, err := pty.Start(recordSession)
	if err != nil {
		log.Fatal(err)
	}

	color.Yellow("dontbug: -- Recording. Ctrl-C to terminate recording if running on the PHP built-in webserver")
	color.Yellow("dontbug: --            Ctrl-C if running a script or simply wait for it to end")
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

	color.Green("\ndontbug: Closed cleanly. Replay should work properly")
}

func StartBasicDebuggerClient(recordPort int) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%v", recordPort))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Started debug client for recording at 127.0.0.1:%v\n", recordPort)
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
					if bytesRead <= 0 {
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

					// color.Green("dontbug <-%v", string(buf[nullAt + 1:bytesRead - 1]))
					// color.Green("dontbug <-%v", string(buf[:bytesRead]))
					seq++

					// Keep running until we are able to record the execution
					runCommand := fmt.Sprintf("run -i %d\x00", seq)
					// color.Cyan("dontbug ->%v", runCommand)
					conn.Write([]byte(runCommand))
				}
			}(conn)
		}
	}()
}

func CheckDontbugWasCompiled(extDir string) string {
	extDirAbsPath := getAbsPathOrFatal(extDir)
	dlPath := extDirAbsPath + "/modules/dontbug.so"

	// Does the zend extension exist?
	_, err := os.Stat(dlPath)
	if err != nil {
		log.Fatal("Not able to find dontbug.so")
	}

	return dlPath
}
