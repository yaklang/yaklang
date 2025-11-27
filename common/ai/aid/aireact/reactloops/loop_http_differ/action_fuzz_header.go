package loop_http_differ

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
)

var fuzzHeaderAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"fuzz_header",
		"Fuzz HTTP request headers. Use this to test header injection, authentication bypass, or header-based attacks.",
		[]aitool.ToolOption{
			aitool.WithStringParam("header_name", aitool.WithParam_Description("The header name to fuzz, e.g., 'X-Forwarded-For', 'Authorization', 'User-Agent'"), aitool.WithParam_Required(true)),
			aitool.WithStringArrayParam("header_values", aitool.WithParam_Description("Values to test for the header"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("reason", aitool.WithParam_Description("Explain why you want to test this header")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			headerName := action.GetString("header_name")
			if headerName == "" {
				return fmt.Errorf("header_name parameter is required")
			}
			headerValues := action.GetStringSlice("header_values")
			if len(headerValues) == 0 {
				return fmt.Errorf("header_values parameter is required and cannot be empty")
			}
			return nil
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
			diffResult, err := executeFuzzAndCompare(loop, fuzzResult, "fuzz_header")
			if err != nil {
				operator.Fail(err)
				return
			}

			r.AddToTimeline("fuzz_header", fmt.Sprintf("Tested header %s with values: %v\n%s", headerName, headerValues, diffResult))
			operator.Feedback(diffResult)
		},
	)
}

