package loop_http_differ

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
)

var fuzzPathAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"fuzz_path",
		"Fuzz the HTTP request path. Use this to test path traversal, different endpoints, or path-based attacks.",
		[]aitool.ToolOption{
			aitool.WithStringArrayParam("paths", aitool.WithParam_Description("Paths to test, e.g., ['/admin', '/api/v2', '../etc/passwd', '/backup']"), aitool.WithParam_Required(true)),
			aitool.WithBoolParam("append_mode", aitool.WithParam_Description("If true, append paths to existing path instead of replacing. Default is false (replace mode)")),
			aitool.WithStringParam("reason", aitool.WithParam_Description("Explain why you want to test these paths")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			paths := action.GetStringSlice("paths")
			if len(paths) == 0 {
				return fmt.Errorf("paths parameter is required and cannot be empty")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			paths := action.GetStringSlice("paths")
			appendMode := action.GetBool("append_mode")
			reason := action.GetString("reason")

			log.Infof("fuzz_path action: testing paths %v, append_mode: %v, reason: %s", paths, appendMode, reason)

			fuzzReq, err := getFuzzRequest(loop)
			if err != nil {
				operator.Fail(err)
				return
			}

			// Execute fuzz on path
			var fuzzResult mutate.FuzzHTTPRequestIf

			if appendMode {
				fuzzResult = fuzzReq.FuzzPathAppend(paths...)
			} else {
				fuzzResult = fuzzReq.FuzzPath(paths...)
			}

			// Execute and compare
			diffResult, err := executeFuzzAndCompare(loop, fuzzResult, "fuzz_path")
			if err != nil {
				operator.Fail(err)
				return
			}

			mode := "replace"
			if appendMode {
				mode = "append"
			}
			r.AddToTimeline("fuzz_path", fmt.Sprintf("Tested paths (%s mode): %v\n%s", mode, paths, diffResult))
			operator.Feedback(diffResult)
		},
	)
}

