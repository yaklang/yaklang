package loop_http_differ

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
)

var fuzzBodyAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"fuzz_body",
		"Fuzz HTTP request body. Use this to test POST parameters, JSON body, or raw body content for various attacks.",
		[]aitool.ToolOption{
			aitool.WithStringParam("body_type", aitool.WithParam_Description("Type of body fuzzing: 'raw' (replace entire body), 'post_params' (fuzz form parameters), 'json_params' (fuzz JSON fields)"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("param_name", aitool.WithParam_Description("Parameter name to fuzz (required for post_params and json_params types)")),
			aitool.WithStringArrayParam("param_values", aitool.WithParam_Description("Values to test"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("reason", aitool.WithParam_Description("Explain why you want to test these values")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			bodyType := action.GetString("body_type")
			if bodyType == "" {
				return fmt.Errorf("body_type parameter is required")
			}
			if bodyType != "raw" && bodyType != "post_params" && bodyType != "json_params" {
				return fmt.Errorf("body_type must be one of: raw, post_params, json_params")
			}
			paramValues := action.GetStringSlice("param_values")
			if len(paramValues) == 0 {
				return fmt.Errorf("param_values parameter is required and cannot be empty")
			}
			if bodyType != "raw" {
				paramName := action.GetString("param_name")
				if paramName == "" {
					return fmt.Errorf("param_name is required for %s body_type", bodyType)
				}
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			bodyType := action.GetString("body_type")
			paramName := action.GetString("param_name")
			paramValues := action.GetStringSlice("param_values")
			reason := action.GetString("reason")

			log.Infof("fuzz_body action: type=%s, param=%s, values=%v, reason: %s", bodyType, paramName, paramValues, reason)

			fuzzReq, err := getFuzzRequest(loop)
			if err != nil {
				operator.Fail(err)
				return
			}

			// Execute fuzz on body based on type
			var fuzzResult mutate.FuzzHTTPRequestIf

			switch bodyType {
			case "raw":
				fuzzResult = fuzzReq.FuzzPostRaw(paramValues...)
			case "post_params":
				fuzzResult = fuzzReq.FuzzPostParams(paramName, paramValues)
			case "json_params":
				fuzzResult = fuzzReq.FuzzPostJsonParams(paramName, paramValues)
			default:
				operator.Fail(fmt.Errorf("unknown body_type: %s", bodyType))
				return
			}

			// Execute and compare
			diffResult, err := executeFuzzAndCompare(loop, fuzzResult, "fuzz_body")
			if err != nil {
				operator.Fail(err)
				return
			}

			r.AddToTimeline("fuzz_body", fmt.Sprintf("Tested body (%s) param %s with values: %v\n%s", bodyType, paramName, paramValues, diffResult))
			operator.Feedback(diffResult)
		},
	)
}

