package loop_http_differ

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
)

var fuzzGetParamsAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"fuzz_get_params",
		"Fuzz GET query parameters. Use this to test SQL injection, XSS, or other parameter-based attacks on URL query string.",
		[]aitool.ToolOption{
			aitool.WithStringParam("param_name", aitool.WithParam_Description("The GET parameter name to fuzz. If empty, will add new parameters"), aitool.WithParam_Required(true)),
			aitool.WithStringArrayParam("param_values", aitool.WithParam_Description("Values to test for the parameter, e.g., [\"' OR '1'='1\", '<script>alert(1)</script>', '{{7*7}}']"), aitool.WithParam_Required(true)),
			aitool.WithBoolParam("raw_mode", aitool.WithParam_Description("If true, replace the entire query string with the provided values")),
			aitool.WithStringParam("reason", aitool.WithParam_Description("Explain why you want to test these values")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			paramName := action.GetString("param_name")
			rawMode := action.GetBool("raw_mode")
			if !rawMode && paramName == "" {
				return fmt.Errorf("param_name is required when not in raw_mode")
			}
			paramValues := action.GetStringSlice("param_values")
			if len(paramValues) == 0 {
				return fmt.Errorf("param_values parameter is required and cannot be empty")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			paramName := action.GetString("param_name")
			paramValues := action.GetStringSlice("param_values")
			rawMode := action.GetBool("raw_mode")
			reason := action.GetString("reason")

			log.Infof("fuzz_get_params action: testing param %s with values %v, raw_mode: %v, reason: %s", paramName, paramValues, rawMode, reason)

			fuzzReq, err := getFuzzRequest(loop)
			if err != nil {
				operator.Fail(err)
				return
			}

			// Execute fuzz on GET params
			var fuzzResult mutate.FuzzHTTPRequestIf

			if rawMode {
				fuzzResult = fuzzReq.FuzzGetParamsRaw(paramValues...)
			} else {
				fuzzResult = fuzzReq.FuzzGetParams(paramName, paramValues)
			}

			// Execute and compare
			diffResult, err := executeFuzzAndCompare(loop, fuzzResult, "fuzz_get_params")
			if err != nil {
				operator.Fail(err)
				return
			}

			mode := "param"
			if rawMode {
				mode = "raw"
			}
			r.AddToTimeline("fuzz_get_params", fmt.Sprintf("Tested GET param %s (%s mode) with values: %v\n%s", paramName, mode, paramValues, diffResult))
			operator.Feedback(diffResult)
		},
	)
}

