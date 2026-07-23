package yakit

import "strings"

const (
	HTTPFlowTagDiscarded          = "[被丢弃]"
	HTTPFlowTagMockResponse       = "[MOCK响应]"
	HTTPFlowTagManualEdit         = "[手动修改]"
	HTTPFlowTagRuleEdit           = "[规则修改]"
	HTTPFlowTagManualHijack       = "[手动劫持]"
	HTTPFlowTagResponseDiscarded  = "[响应被丢弃]"
	HTTPFlowTagAutoFixResponse    = "[自动修复]" // DB Response is fixed; wire is in KV (GetHTTPFlowBare, same as MITM bare).
	HTTPFlowTagResend             = "[重发]"
	HTTPFlowTagWebFuzzer          = "[WebFuzzer]" // 流量由 WebFuzzer（含序列）发出，便于从数据库按 tag 筛选
	HTTPFlowTagHAR                = "[HAR]"        // 流量由导入 HAR 文件产生，便于从数据库按 tag 筛选区分
)

// HTTPFlowBuiltinTags 后端内置 tag；命中则 HTTPFlowsFieldGroup 返回 Builtin=true。
var HTTPFlowBuiltinTags = map[string]struct{}{
	HTTPFlowTagDiscarded:         {},
	HTTPFlowTagMockResponse:      {},
	HTTPFlowTagManualEdit:        {},
	HTTPFlowTagRuleEdit:          {},
	HTTPFlowTagManualHijack:      {},
	HTTPFlowTagResponseDiscarded: {},
	HTTPFlowTagAutoFixResponse:   {},
	HTTPFlowTagResend:            {},
	HTTPFlowTagWebFuzzer:         {},
	HTTPFlowTagHAR:               {},
}

func IsHTTPFlowBuiltinTag(tag string) bool {
	if _, ok := HTTPFlowBuiltinTags[tag]; ok {
		return true
	}
	return strings.HasPrefix(tag, HTTPFlowTagResend)
}
