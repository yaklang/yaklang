package loop_http_differ

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
)

var setHTTPRequestAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"set_http_request",
		"Set the HTTP request to be tested. This action MUST be called first before any fuzz actions. All subsequent fuzz actions will be based on this HTTP request.",
		[]aitool.ToolOption{
			aitool.WithStringParam("http_request", aitool.WithParam_Description("The raw HTTP request packet to test. Must be a valid HTTP request format."), aitool.WithParam_Required(true)),
			aitool.WithBoolParam("is_https", aitool.WithParam_Description("Whether the request should use HTTPS. Default is false.")),
			aitool.WithStringParam("reason", aitool.WithParam_Description("Explain why you want to test this HTTP request")),
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

			// Create FuzzHTTPRequest object
			fuzzReq, err := mutate.NewFuzzHTTPRequest([]byte(httpRequest), mutate.OptHTTPS(isHttps))
			if err != nil {
				operator.Fail(fmt.Errorf("failed to create FuzzHTTPRequest: %v", err))
				return
			}

			// Store the fuzz request in loop context
			loop.Set("fuzz_request", fuzzReq)
			loop.Set("original_request", httpRequest)
			loop.Set("is_https", utils.InterfaceToString(isHttps))

			// Clear previous fuzz results
			loop.Set("last_request", "")
			loop.Set("last_response", "")
			loop.Set("diff_result", "")

			r.AddToTimeline("set_http_request", fmt.Sprintf("HTTP request set successfully, is_https: %v", isHttps))

			// Build feedback message
			var feedback strings.Builder
			feedback.WriteString("HTTP request set successfully.\n\n")
			feedback.WriteString("=== Request Summary ===\n")
			feedback.WriteString(utils.ShrinkTextBlock(httpRequest, 500))
			feedback.WriteString("\n\n")
			feedback.WriteString("You can now use fuzz actions (fuzz_method, fuzz_path, fuzz_header, fuzz_get_params, fuzz_body, fuzz_cookie) to test this request.")

			operator.Feedback(feedback.String())
			log.Infof("set_http_request done: request set successfully")
		},
	)
}
