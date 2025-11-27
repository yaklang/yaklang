package loop_http_differ

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
)

var fuzzMethodAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"fuzz_method",
		"Fuzz the HTTP request method. Use this to test different HTTP methods (GET, POST, PUT, DELETE, PATCH, OPTIONS, HEAD, etc.) and observe response differences.",
		[]aitool.ToolOption{
			aitool.WithStringArrayParam("methods", aitool.WithParam_Description("HTTP methods to test, e.g., ['GET', 'POST', 'PUT', 'DELETE', 'PATCH', 'OPTIONS', 'HEAD']"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("reason", aitool.WithParam_Description("Explain why you want to test these methods")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			methods := action.GetStringSlice("methods")
			if len(methods) == 0 {
				return fmt.Errorf("methods parameter is required and cannot be empty")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			methods := action.GetStringSlice("methods")
			reason := action.GetString("reason")

			log.Infof("fuzz_method action: testing methods %v, reason: %s", methods, reason)

			fuzzReq, err := getFuzzRequest(loop)
			if err != nil {
				operator.Fail(err)
				return
			}

			// Execute fuzz on method
			fuzzResult := fuzzReq.FuzzMethod(methods...)

			// Execute and compare
			diffResult, err := executeFuzzAndCompare(loop, fuzzResult, "fuzz_method")
			if err != nil {
				operator.Fail(err)
				return
			}

			r.AddToTimeline("fuzz_method", fmt.Sprintf("Tested methods: %v\n%s", methods, diffResult))
			operator.Feedback(diffResult)
		},
	)
}

