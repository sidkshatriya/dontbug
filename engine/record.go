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
	"bufio"
	"bytes"
	"fmt"
	"github.com/fatih/color"
	"github.com/kr/pty"
	"github.com/libgit2/git2go"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	// These strings are not to be changed as these strings are sentinels from the dontbug zend extension
	dontbugZendExtensionLoadedSentinel          = "dontbug zend extension: dontbug.so successfully loaded by PHP"
	dontbugZendXdebugNotLoadedSentinel          = "dontbug zend extension: Xdebug has not been loaded"
	dontbugZendXdebugEntryPointNotFoundSentinel = "dontbug zend extension: Xdebug entrypoint not found"
	// End do not change

	dontbugRRTraceDirSentinel = "rr: Saving execution to trace directory `"

	dontbugNotPatchedXdebugMsg = `Unpatched Xdebug zend extension (xdebug.so) found. See below for more information:
dontbug zend extension currently relies on a patched version of Xdebug to function correctly.
This is a very minor patch and simply makes a single function extern (instead of static) linkage.
It seems you are using the plain vanilla version of Xdebug. Consult documentation on patching Xdebug.
`
)

func getOrCreateDontbugSharePath() string {
	currentUser, err := user.Current()
	fatalIf(err)

	dontbugShareDir := currentUser.HomeDir + "/.local/share/dontbug/"
	err = os.MkdirAll(dontbugShareDir, 0700)
	if err != nil {
		log.Fatalf("Was trying to do `mkdir -p %v' essentially. Encountered error: %v\n", dontbugShareDir, err)
	}

	return dontbugShareDir
}

func copyAndMakeUniqueDontbugSo(sharedObjectPath, dontbugShareDir string) string {
	uniqueDontbugSoFilename := path.Clean(fmt.Sprintf("%v/dontbug-%v.so", dontbugShareDir, time.Now().UnixNano()))
	output, err := exec.Command("cp", sharedObjectPath, uniqueDontbugSoFilename).CombinedOutput()
	if err != nil {
		log.Fatal(output)
	}
	return uniqueDontbugSoFilename
}

// Assumptions:
// - rrPath represents an rr executable that meets dontbug's requirements
// - phpPath represents an php executable that meets dontbug's requirements
// - sharedObject path is the path to xdebug.so that meets dontbug's requirements
// - docrootDirOrScript is a valid docroot directory or a php script
func doRecordSession(docrootDirOrScript, sharedObjectPath, rrPath, phpPath string, isCli bool, arguments, serverListen string, serverPort, recordPort, maxStackDepth int, takeSnapshot bool, rootDir, commitID, tagname string) {
	// @TODO remove this check and move to separate function
	docrootOrScriptAbsPath := getAbsPathOrFatal(docrootDirOrScript)

	newSharedObjectPath := sharedObjectPath
	if takeSnapshot {
		dontbugShareDir := getOrCreateDontbugSharePath()
		newSharedObjectPath = copyAndMakeUniqueDontbugSo(sharedObjectPath, dontbugShareDir)
	}

	// Many of these options are not really necessary to be specified.
	// However, we still do that to override any settings that
	// might be present in user php.ini files and change them
	// to sensible defaults for 'dontbug record'
	rrCmd := []string{
		"record",
		phpPath,
		"-d", "zend_extension=xdebug.so",
		"-d", "zend_extension=" + newSharedObjectPath,
		"-d", fmt.Sprintf("xdebug.remote_port=%v", recordPort),
		"-d", "xdebug.remote_autostart=1",
		"-d", "xdebug.remote_connect_back=0",
		"-d", "xdebug.remote_enable=1",
		"-d", "xdebug.remote_mode=req",
		"-d", "xdebug.auto_trace=0",
		"-d", "xdebug.trace_enable_trigger=\"\"",
		"-d", "xdebug.coverage_enable=0",
		"-d", "xdebug.extended_info=1",
		"-d", fmt.Sprintf("xdebug.max_nesting_level=%v", maxStackDepth),
		"-d", "xdebug.profiler_enable=0",
		"-d", "xdebug.profiler_enable_trigger=0",
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

	Verboseln("dontbug: Issuing command: rr", strings.Join(rrCmd, " "))
	recordSession := exec.Command(rrPath, rrCmd...)

	f, err := pty.Start(recordSession)
	fatalIf(err)

	color.Yellow("dontbug: -- Recording. Ctrl-C to terminate recording if running on the PHP built-in webserver")
	color.Yellow("dontbug: -- Recording. Ctrl-C if running a script or simply wait for it to end")

	rrTraceDir := ""
	go func() {
		wrappedF := bufio.NewReader(f)
		fatalIf(err)

		for {
			line, err := wrappedF.ReadString('\n')
			fmt.Print(line)
			if err == io.EOF {
				return
			} else if err != nil {
				log.Fatal(err)
			}

			if strings.Contains(line, dontbugRRTraceDirSentinel) {
				start := strings.LastIndex(line, "`")
				end := strings.LastIndex(line, "'")
				if start == -1 || end == -1 || start+1 == len(line) {
					log.Fatal("Could not understand rr trace directory message")
				}

				rrTraceDir = line[start+1 : end]
			}

			if strings.Contains(line, dontbugZendXdebugNotLoadedSentinel) {
				log.Fatal("Xdebug zend extension was not loaded. dontbug needs Xdebug to work correctly")
			}

			if strings.Contains(line, dontbugZendXdebugEntryPointNotFoundSentinel) {
				log.Fatal(dontbugNotPatchedXdebugMsg)
			}

			if strings.Contains(line, "Failed loading") && strings.Contains(line, "dontbug.so") {
				log.Fatal("Could not load dontbug.so")
			}

			if strings.Contains(line, dontbugZendExtensionLoadedSentinel) {
				break
			}
		}

		io.Copy(os.Stdout, f)
	}()

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
	fatalIf(err)

	if takeSnapshot {
		if rrTraceDir == "" {
			log.Fatal("Could not detect rr trace dir location")
		}
		createSnapshotMetadata(rrTraceDir, rootDir, commitID, tagname)
	}
	color.Green("\ndontbug: Closed cleanly. Replay should work properly")
}

func createSnapshotMetadata(rrTraceDir, rootDir, commitID, tagname string) {
	// @TODO? rootDir represents _both_ the git workdir in addition to the root sourcedir at the moment
	fileData := []byte(tagname + ":" + rootDir + ":" + commitID + ":\n")
	metaDataFilename := path.Clean(rrTraceDir + "/" + tagname)
	err := ioutil.WriteFile(metaDataFilename, fileData, 0700)
	if err != nil {
		log.Fatalf("Could not write to %v\n", metaDataFilename)
	}
}

// Here we're basically serving the role of an PHP debugger in an IDE
func startBasicDebuggerClient(recordPort int) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%v", recordPort))
	fatalIf(err)

	Verbosef("Started debug client for recording at 127.0.0.1:%v\n", recordPort)
	go func() {
		for {
			conn, err := listener.Accept()
			fatalIf(err)

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
					fatalIf(err)

					bytesLeft := dataLen - (bytesRead - nullAt - 2)
					if bytesLeft != 0 {
						log.Fatal("There are still some bytes left to receive -- strange")
					}

					seq++

					// Keep running until we are able to record the execution
					runCommand := fmt.Sprintf("run -i %d\x00", seq)
					conn.Write([]byte(runCommand))
				}
			}(conn)
		}
	}()
}

func checkDontbugWasCompiled(extDir string) string {
	extDirAbsPath := getAbsPathOrFatal(extDir)
	dlPath := extDirAbsPath + "/modules/dontbug.so"

	// Does the zend extension exist?
	_, err := os.Stat(dlPath)
	if err != nil {
		log.Fatal("Not able to find dontbug.so")
	}

	return dlPath
}

func DoChecksAndRecord(phpExecutable, rrExecutable, rootDir, extDir, docrootOrScript string, maxStackDepth int, isCli bool, arguments string, recordPort int, serverListen string, serverPort int, takeSnapshot bool) {
	commitID := ""
	tagname := ""
	if takeSnapshot {
		// @TODO rootDir is also the git repo location for now
		commitID, tagname = checkInAllChanges(rootDir)
	}

	phpPath := checkPhpExecutable(phpExecutable)
	rrPath := CheckRRExecutable(rrExecutable)

	doGeneration(rootDir, extDir, maxStackDepth, phpPath)
	dontbugSharedObjectPath := checkDontbugWasCompiled(extDir)
	startBasicDebuggerClient(recordPort)
	doRecordSession(
		docrootOrScript,
		dontbugSharedObjectPath,
		rrPath,
		phpPath,
		isCli,
		arguments,
		serverListen,
		serverPort,
		recordPort,
		maxStackDepth,
		takeSnapshot,
		rootDir,
		commitID,
		tagname,
	)
}

//
// Implements basic snapshotting of PHP sources using git
// returns the commit id and the tag name
//
func checkInAllChanges(gitDir string) (string, string) {
	// @TODO this needs to be consolidated with zend extension generation. Also incomplete
	phpFileTypes := []string{"**/*.php", "**/.module", "**/.install"}
	sig := &git.Signature{
		Name:  "dontbug",
		Email: "dontbug@dontbug.nowhere",
		When:  time.Now(),
	}

	repo, err := git.OpenRepository(gitDir)
	fatalIf(err)

	head, err := repo.Head()
	fatalIf(err)

	index, err := repo.Index()
	fatalIf(err)

	color.Green("dontbug: -- Creating a PHP source snapshot for use during a future replay by the dontbug engine")
	color.Green("dontbug: -- This snapshot will be available as a git commit with tag")
	color.Green("dontbug:")
	color.Green("dontbug: -- This snapshot represents the current state of the PHP sources in this repo")
	color.Green("dontbug: -- * It will include any untracked PHP files (as these could be accessed by PHP during a run)")
	color.Green("dontbug: -- * It will include the latest modifications (on disk) of tracked PHP sources")
	color.Green("dontbug: -- * Any non-PHP related source already on the stage will also be included in the snapshot")
	color.Green("dontbug: -- This snapshot will *not* disturb the stage (i.e. git index) or your current branch")
	color.Yellow("dontbug: -- You can continue working as if nothing happened (except a new commit and tag will exist in your repo)")

	// This is a subtle operation. Essentially we want to (git) snapshot the state of PHP sources when the recording
	// takes place. This snapshot should be available during a future replay (we would just need to do a git checkout).
	// In AddAll() we're basically doing a git add -A <pathspec>. Here pathspec is the "phpFileTypes"
	// This will "git add" all php files whether they are tracked or untracked
	// We need to add untracked files because PHP could still access them during a run and they need to be present
	// during a future replay.
	// After the AddAll() command we save the index tree, create a git commit and then tag it
	// @TODO what about submodules?
	index.AddAll(phpFileTypes, git.IndexAddForce, func(path, pathPattern string) int {
		return 0
	})
	// Note that we DON'T do index.Write() after index.AddAll(). We don't want to disturb the state of the index
	// as seen by the end user using the git repo

	oid, err := index.WriteTreeTo(repo)
	fatalIf(err)

	tree, err := repo.LookupTree(oid)
	fatalIf(err)

	headCommitObj, err := head.Peel(git.ObjectCommit)
	fatalIf(err)

	headCommit, err := headCommitObj.AsCommit()
	fatalIf(err)

	t := time.Now()
	commitMsg := fmt.Sprintf("dontbug snapshot taken on %v", t.Format(time.Stamp))
	oid, err = repo.CreateCommit("", sig, sig, commitMsg, tree, headCommit)
	fatalIf(err)

	commit, err := repo.LookupCommit(oid)
	fatalIf(err)

	commitID := fmt.Sprintf("%v", oid)
	timeNowStr := t.Format("20060102-1504")

	tagname := fmt.Sprintf("dontbug-snapshot-%v-%.8v", timeNowStr, commitID)
	_, err = repo.Tags.Create(tagname, commit, sig, tagname)
	fatalIf(err)

	color.Green("dontbug:\ndontbug: -- Snapshot created at %v", t.Format(time.Stamp))
	color.Yellow("dontbug: -- See git commit of snapshot: %v", commitID)
	color.Green("dontbug: -- Also created tag: %v", tagname)

	return commitID, tagname
}
