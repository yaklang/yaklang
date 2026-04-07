package loop_http_fuzztest

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
)

var fuzzCookieAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"fuzz_cookie",
		"Fuzz HTTP cookies. Use this to test session manipulation, cookie injection, or authentication bypass attacks.",
		[]aitool.ToolOption{
			aitool.WithStringParam("cookie_name", aitool.WithParam_Description("The cookie name to fuzz. If empty and raw_mode is true, will replace entire Cookie header")),
			aitool.WithStringArrayParam("cookie_values", aitool.WithParam_Description("Values to test for the cookie"), aitool.WithParam_Required(true)),
			aitool.WithBoolParam("raw_mode", aitool.WithParam_Description("If true, replace entire Cookie header with the provided values")),
			aitool.WithStringParam("reason", aitool.WithParam_Description("请用中文说明为什么要测试这些 Cookie 值、怀疑的漏洞类型以及安全测试边界。")),
		},
		[]*reactloops.LoopStreamField{
			{FieldName: "fuzz_cookie", AINodeId: "thought"},
			{FieldName: "cookie_name", AINodeId: "thought"},
			{FieldName: "reason", AINodeId: "thought"},
			{FieldName: "cookie_values", AINodeId: "thought"},
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			cookieValues := action.GetStringSlice("cookie_values")
			if len(cookieValues) == 0 {
				return fmt.Errorf("cookie_values parameter is required and cannot be empty")
			}
			rawMode := action.GetBool("raw_mode")
			if !rawMode {
				cookieName := action.GetString("cookie_name")
				if cookieName == "" {
					return fmt.Errorf("cookie_name is required when not in raw_mode")
				}
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			cookieName := action.GetString("cookie_name")
			cookieValues := action.GetStringSlice("cookie_values")
			rawMode := action.GetBool("raw_mode")
			reason := action.GetString("reason")

			log.Infof("fuzz_cookie action: testing cookie %s with values %v, raw_mode: %v, reason: %s", cookieName, cookieValues, rawMode, reason)

			fuzzReq, err := getFuzzRequest(loop)
			if err != nil {
				operator.Fail(err)
				return
			}

			// Execute fuzz on cookie
			var fuzzResult mutate.FuzzHTTPRequestIf

			if rawMode {
				fuzzResult = fuzzReq.FuzzCookieRaw(cookieValues)
			} else {
				fuzzResult = fuzzReq.FuzzCookie(cookieName, cookieValues)
			}

			// Execute and compare
			diffResult, verifyResult, err := executeFuzzAndCompare(loop, fuzzResult, "fuzz_cookie")
			if err != nil {
				operator.Fail(err)
				return
			}

			mode := "param"
			if rawMode {
				mode = "raw"
			}
			r.AddToTimeline("fuzz_cookie", fmt.Sprintf("Tested cookie %s (%s mode) with values: %v\n%s", cookieName, mode, cookieValues, buildFuzzTimelineSummary(diffResult)))
			applyFuzzVerificationOutcome(loop, operator, diffResult, verifyResult)
		},
	)
}
