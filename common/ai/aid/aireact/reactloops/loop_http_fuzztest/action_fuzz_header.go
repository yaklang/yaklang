package loop_http_fuzztest

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
)

func pinBoundedSaaSHeaderAction(action *aicommon.Action) {
	if action == nil {
		return
	}
	action.ForceSet("header_name", boundedSaaSHeaderName)
	action.ForceSet("header_values", append([]string(nil), boundedSaaSHeaderValues...))
}

var fuzzHeaderAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"fuzz_header",
		"Fuzz HTTP request headers. Use this to test header injection, authentication bypass, or header-based attacks.",
		[]aitool.ToolOption{
			aitool.WithStringParam("header_name", aitool.WithParam_Description("The header name to fuzz, e.g., 'X-Forwarded-For', 'Authorization', 'User-Agent'"), aitool.WithParam_Required(true)),
			aitool.WithStringArrayParam("header_values", aitool.WithParam_Description("Values to test for the header. Supports arbitrary fuzztag; see the FUZZTAG_REFERENCE and AVAILABLE_PAYLOAD_GROUPS context blocks for the current full tag manual and payload dictionary groups. When batch generation is needed, prefer concise fuzztag rules over long handwritten lists."), aitool.WithParam_Required(true)),
		},
		[]*reactloops.LoopStreamField{
			{FieldName: "reason", AINodeId: "thought"},
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			if isBoundedSaaSHTTPFuzz(l.GetConfig()) {
				pinBoundedSaaSHeaderAction(action)
			}
			headerName := action.GetString("header_name")
			if headerName == "" {
				return fmt.Errorf("header_name parameter is required")
			}
			headerValues := action.GetStringSlice("header_values")
			if len(headerValues) == 0 {
				return fmt.Errorf("header_values parameter is required and cannot be empty")
			}
			return validateBoundedSaaSHeaderAction(l, headerName, headerValues)
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			headerName := action.GetString("header_name")
			headerValues := action.GetStringSlice("header_values")
			reason := action.GetString("reason")

			log.Infof("fuzz_header action: testing header %s with values %v, reason: %s", headerName, headerValues, reason)

			fuzzReq, err := getFuzzRequest(loop)
			if err != nil {
				operator.Fail(err)
				return
			}

			// Execute fuzz on header
			fuzzResult := fuzzReq.FuzzHTTPHeader(headerName, headerValues)

			// Execute and compare
			paramSummary := fmt.Sprintf("header_name=%s; header_values=%v; reason=%s", headerName, headerValues, reason)
			diffResult, verifyResult, err := executeFuzzAndCompare(loop, fuzzResult, "fuzz_header", paramSummary, action)
			if err != nil {
				operator.Fail(err)
				return
			}

			r.AddToTimeline("fuzz_header", fmt.Sprintf("Tested header %s with values: %v\n%s", headerName, headerValues, buildFuzzTimelineSummary(diffResult)))
			if isBoundedSaaSHTTPFuzz(loop.GetConfig()) {
				operator.Feedback(diffResult)
				operator.Exit()
				return
			}
			applyFuzzVerificationOutcome(loop, operator, diffResult, verifyResult)
		},
	)
}
