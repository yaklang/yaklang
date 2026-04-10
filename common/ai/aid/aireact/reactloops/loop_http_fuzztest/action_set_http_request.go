package loop_http_fuzztest

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var setHTTPRequestAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"set_http_request",
		"Set the HTTP request to be tested. This action MUST be called first before any fuzz actions. All subsequent fuzz actions will be based on this HTTP request.",
		[]aitool.ToolOption{
			aitool.WithStringParam("http_request", aitool.WithParam_Description("The raw HTTP request packet to test. Must be a valid HTTP request format."), aitool.WithParam_Required(true)),
			aitool.WithBoolParam("is_https", aitool.WithParam_Description("Whether the request should use HTTPS. Default is false.")),
			aitool.WithStringParam("reason", aitool.WithParam_Description("请用中文说明为什么要测试这个 HTTP 数据包、怀疑的漏洞点，以及必须遵守的安全边界。")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			httpRequest := action.GetString("http_request")
			if httpRequest == "" {
				return fmt.Errorf("http_request parameter is required and cannot be empty")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			httpRequest := action.GetString("http_request")
			isHttps := action.GetBool("is_https")
			reason := action.GetString("reason")

			log.Infof("set_http_request action: setting HTTP request, is_https: %v, reason: %s", isHttps, reason)

			result, err := applyLoopHTTPFuzzRequestChange(loop, r, &loopHTTPFuzzRequestChange{
				RawRequest:          httpRequest,
				IsHTTPS:             isHttps,
				SourceAction:        "set_http_request",
				ChangeReason:        reason,
				EventOp:             loopHTTPFuzzRequestEventOpReplace,
				ResetBaseline:       true,
				ClearActionTracking: true,
				EmitEvent:           true,
				EmitEditablePacket:  true,
				PersistSession:      true,
				Task:                operator.GetTask(),
			})
			if err != nil {
				operator.Fail(fmt.Errorf("failed to apply HTTP request: %v", err))
				return
			}

			record := recordLoopHTTPFuzzMetaAction(loop, "set_http_request", fmt.Sprintf("is_https=%v; reason=%s", isHttps, reason), utils.ShrinkTextBlock(httpRequest, 240))

			r.AddToTimeline("set_http_request", fmt.Sprintf("HTTP request set successfully, is_https: %v", isHttps))

			// Build feedback message
			var feedback strings.Builder
			feedback.WriteString("HTTP request set successfully.\n\n")
			feedback.WriteString("=== Request Summary ===\n")
			feedback.WriteString(utils.ShrinkTextBlock(result.CurrentState.RawRequest, 500))
			feedback.WriteString("\n\n")
			feedback.WriteString("The request will be executed with HTTP flow persistence enabled, so each fuzz result can be traced in the system by runtime and task context.\n\n")
			feedback.WriteString("You can now use fuzz actions (fuzz_method, fuzz_path, fuzz_header, fuzz_get_params, fuzz_body, fuzz_cookie) to test this request.")

			operator.Feedback(buildLoopHTTPFuzzActionFeedback(record) + "\n\n" + feedback.String())
			log.Infof("set_http_request done: request set successfully")
		},
	)
}
