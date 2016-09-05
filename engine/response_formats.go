package engine

var InitXmlResponseFormat string =
	`<init xmlns="urn:debugger_protocol_v1" language="PHP" protocol_version="1.0"
		fileuri="file://%v"
		appid="%v" idekey="dontbug">
		<engine version="0.0.1"><![CDATA[dontbug]]></engine>
	</init>`

var FeatureSetXmlResponseFormat =
	`<response xmlns="urn:debugger_protocol_v1" command="feature_set"
		transaction_id="%v" feature="%v" success="%v">
	</response>`

var StatusXmlResponseFormat =
	`<response xmlns="urn:debugger_protocol_v1" command="status"
		transaction_id="%v" status="%v" reason="%v">
	</response>`

var BreakpointSetLineXmlResponseFormat =
	`<response xmlns="urn:debugger_protocol_v1" command="breakpoint_set" transaction_id="%v" status="%v" id="%v">
	</response>`

var ErrorXmlResponseFormat =
	`<response xmlns="urn:debugger_protocol_v1" command="%v" transaction_id="%v">
	 	<error code="%v">
        		<message>%v</message>
    		</error>
	</response>`

var BreakpointRemoveOrUpdateXmlResponseFormat =
	`<response xmlns="urn:debugger_protocol_v1" command="%v" transaction_id="%v">
	</response>`

var StepIntoBreakXmlResponseFormat =
	`<response xmlns="urn:debugger_protocol_v1" xmlns:xdebug="http://xdebug.org/dbgp/xdebug" command="step_into"
		transaction_id="%v" status="break" reason="ok">
		<xdebug:message filename="%v" lineno="%v"></xdebug:message>
	</response>`

var RunOrStepBreakXmlResponseFormat =
	`<response xmlns="urn:debugger_protocol_v1" xmlns:xdebug="http://xdebug.org/dbgp/xdebug" command="%v"
		transaction_id="%v" status="break" reason="ok">
		<xdebug:message filename="%v" lineno="%v"></xdebug:message>
	</response>`

// @TODO Always fail the stdout/stdout/stderr commands, until this is implemented
var StdFdXmlResponseFormat =
	`<response transaction_id="%v" command="%v" success="0"></response>`

// Replay under rr is read-only. The property set function is to fail, always.
var PropertySetXmlResponseFormat =
	`<response transaction_id="%v" command="property_set" success="0"></response>`

