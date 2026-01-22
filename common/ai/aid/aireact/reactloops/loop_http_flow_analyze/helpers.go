package loop_http_flow_analyze

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func buildQueryRequestFromAction(action *aicommon.Action, defaultLimit int) *ypb.QueryHTTPFlowRequest {
	limit := action.GetInt("limit", defaultLimit)
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > 500 {
		limit = 500
	}

	tags := splitMulti(action.GetString("tags"))
	excludeKeywords := splitMulti(action.GetString("exclude_keywords"))
	includeURL := splitMulti(action.GetString("url_contains"))

	req := &ypb.QueryHTTPFlowRequest{
		Keyword:         action.GetString("keyword"),
		KeywordType:     action.GetString("keyword_type"),
		Methods:         action.GetString("methods"),
		StatusCode:      action.GetString("status_code"),
		Tags:            tags,
		ExcludeKeywords: excludeKeywords,
		IncludeInUrl:    includeURL,
		RuntimeId:       action.GetString("runtime_id"),
		SourceType:      action.GetString("source_type"),
		SearchURL:       action.GetString("search_url"),
		Full:            true,
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   int64(limit),
			OrderBy: "updated_at",
			Order:   "desc",
		},
	}

	if req.SearchURL == "" {
		req.SearchURL = action.GetString("url_contains")
	}

	return req
}

func flowRequest(flow *schema.HTTPFlow) string {
	if flow == nil {
		return ""
	}
	if req := flow.GetRequest(); req != "" {
		return req
	}
	return flow.Request
}

func flowResponse(flow *schema.HTTPFlow) string {
	if flow == nil {
		return ""
	}
	if rsp := flow.GetResponse(); rsp != "" {
		return rsp
	}
	return flow.Response
}

func shrinkTags(tags string) string {
	parts := utils.PrettifyListFromStringSplited(tags, "|")
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}

func splitMulti(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	normalized := strings.NewReplacer("|", ",", "\n", ",", "\t", ",", ";", ",").Replace(raw)
	return utils.PrettifyListFromStringSplited(normalized, ",")
}
