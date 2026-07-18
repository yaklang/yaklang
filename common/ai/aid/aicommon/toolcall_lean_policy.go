package aicommon

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

var lowRiskReadOnlyTools = map[string]struct{}{
	"read_file": {}, "find_file": {}, "find_files": {}, "grep": {}, "grep_text": {},
	"simple_crawler": {}, "web_search": {}, "url_content_summary": {}, "search_knowledge": {},
	"dig": {}, "banner_grab": {},
}

func shouldSkipLowRiskToolReview(tool *aitool.Tool, params aitool.InvokeParams, config KeyValueConfigIf) bool {
	if tool == nil || config == nil || !config.GetConfigBool(ConfigEnableLowRiskToolAutoApprove, true) {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(tool.Name))
	if _, ok := lowRiskReadOnlyTools[name]; ok {
		return true
	}
	if name != "do_http_request" && name != "batch_do_http_request" {
		return false
	}
	method := strings.ToUpper(strings.TrimSpace(params.GetString("method")))
	if method == "" {
		method = "GET"
	}
	if method != "GET" && method != "HEAD" && method != "OPTIONS" {
		return false
	}
	return strings.TrimSpace(params.GetString("body")) == "" && strings.TrimSpace(params.GetString("packet")) == ""
}
