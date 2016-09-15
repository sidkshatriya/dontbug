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

var gInitXMLResponseFormat = `<init xmlns="urn:debugger_protocol_v1" language="PHP" protocol_version="1.0"
		fileuri="file://%v"
		appid="%v" idekey="dontbug">
		<engine version="0.0.1"><![CDATA[dontbug]]></engine>
	</init>`

var gFeatureSetXMLResponseFormat = `<response xmlns="urn:debugger_protocol_v1" command="feature_set"
		transaction_id="%v" feature="%v" success="%v">
	</response>`

var gFeatureGetXMLResponseFormat = `<response xmlns="urn:debugger_protocol_v1" command="feature_get"
		transaction_id="%v" feature_name="%v" supported="%v">
		%v
	</response>`

var gStatusXMLResponseFormat = `<response xmlns="urn:debugger_protocol_v1" command="status"
		transaction_id="%v" status="%v" reason="%v">
	</response>`

var gBreakpointSetLineXMLResponseFormat = `<response xmlns="urn:debugger_protocol_v1" command="breakpoint_set" transaction_id="%v" status="%v" id="%v">
	</response>`

var gErrorXMLResponseFormat = `<response xmlns="urn:debugger_protocol_v1" command="%v" transaction_id="%v">
	 	<error code="%v">
        		<message>%v</message>
    		</error>
	</response>`

var gBreakpointRemoveOrUpdateXMLResponseFormat = `<response xmlns="urn:debugger_protocol_v1" command="%v" transaction_id="%v">
	</response>`

var gStepIntoBreakXMLResponseFormat = `<response xmlns="urn:debugger_protocol_v1" xmlns:xdebug="http://xdebug.org/dbgp/xdebug" command="step_into"
		transaction_id="%v" status="break" reason="ok">
		<xdebug:message filename="%v" lineno="%v"></xdebug:message>
	</response>`

var gRunOrStepBreakXMLResponseFormat = `<response xmlns="urn:debugger_protocol_v1" xmlns:xdebug="http://xdebug.org/dbgp/xdebug" command="%v"
		transaction_id="%v" status="break" reason="ok">
		<xdebug:message filename="%v" lineno="%v"></xdebug:message>
	</response>`

// @TODO Always fail the stdout/stdout/stderr commands, until this is implemented
var gStdFdXMLResponseFormat = `<response transaction_id="%v" command="%v" success="0"></response>`

// Replay under rr is read-only. The property set function is to fail, always.
var gPropertySetXMLResponseFormat = `<response transaction_id="%v" command="property_set" success="0"></response>`
