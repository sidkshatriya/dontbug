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

package main

import (
	"github.com/sidkshatriya/dontbug/cmd"
	"log"
)

func main() {
	log.SetFlags(log.Lshortfile)
	// Light red background
	log.SetPrefix("dontbug: \x1b[101mfatal error:\x1b[0m ")
	cmd.Execute()
}
