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
	"log"
	"fmt"
	"strconv"
)

type featureBool struct{ Value bool; ReadOnly bool }
type featureInt struct{ Value int; ReadOnly bool }
type featureString struct{ Value string; ReadOnly bool }

type featureValue interface {
	Set(value string)
	String() string
}

func (this *featureBool) Set(value string) {
	if this.ReadOnly {
		log.Fatal("Trying assign to a read only value")
	}

	if value == "0" {
		this.Value = false
	} else if value == "1" {
		this.Value = true
	} else {
		log.Fatal("Trying to assign a non-boolean value to a boolean.")
	}
}

func (this featureBool) String() string {
	if this.Value {
		return "1"
	} else {
		return "0"
	}
}

func (this *featureString) Set(value string) {
	if this.ReadOnly {
		log.Fatal("Trying assign to a read only value")
	}
	this.Value = value
}

func (this featureInt) String() string {
	return strconv.Itoa(this.Value)
}

func (this *featureInt) Set(value string) {
	if this.ReadOnly {
		log.Fatal("Trying assign to a read only value")
	}
	var err error
	this.Value, err = strconv.Atoi(value)
	if err != nil {
		log.Fatal(err)
	}

}

func (this featureString) String() string {
	return this.Value
}

func initFeatureMap() map[string]featureValue {
	var featureMap = map[string]featureValue{
		"language_supports_threads" : &featureBool{false, true},
		"language_name" : &featureString{"PHP", true},
		// @TODO should the exact version be ascertained?
		"language_version" : &featureString{"7.0", true},
		"encoding" : &featureString{"ISO-8859-1", true},
		"protocol_version" : &featureInt{1, true},
		"supports_async" : &featureBool{false, true},
		// @TODO full list
		// "breakpoint_types" : &FeatureString{"line call return exception conditional watch", true},
		"breakpoint_types" : &featureString{"line", true},
		"multiple_sessions" : &featureBool{false, false},
		"max_children": &featureInt{64, false},
		"max_data": &featureInt{2048, false},
		"max_depth" : &featureInt{1, false},
		"extended_properties": &featureBool{false, false},
		"show_hidden": &featureBool{false, false},
	}

	return featureMap
}

func handleFeatureSet(es *engineState, dCmd dbgpCmd) string {
	n, ok := dCmd.Options["n"]
	if !ok {
		log.Fatal("Please provide -n option in feature_set")
	}

	v, ok := dCmd.Options["v"]
	if !ok {
		log.Fatal("Not provided v option in feature_set")
	}

	var featureVal featureValue
	featureVal, ok = es.featureMap[n]
	if !ok {
		log.Fatal("Unknown option:", n)
	}

	featureVal.Set(v)
	return fmt.Sprintf(gFeatureSetXmlResponseFormat, dCmd.Sequence, n, 1)
}
