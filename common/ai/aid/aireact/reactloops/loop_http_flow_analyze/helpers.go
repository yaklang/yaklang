package loop_http_flow_analyze

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const maxHTTPFlowSummaryBytes = 1024 * 5

func buildSearchParamSummary(action *aicommon.Action) string {
	var parts []string
	if v := action.GetString("keyword"); v != "" {
		parts = append(parts, fmt.Sprintf("keyword=%q", v))
	}
	if v := action.GetString("keyword_type"); v != "" {
		parts = append(parts, fmt.Sprintf("keyword_type=%s", v))
	}
	if v := action.GetString("methods"); v != "" {
		parts = append(parts, fmt.Sprintf("methods=%s", v))
	}
	if v := action.GetString("status_code"); v != "" {
		parts = append(parts, fmt.Sprintf("status=%s", v))
	}
	if v := action.GetString("url_contains"); v != "" {
		parts = append(parts, fmt.Sprintf("url=%q", v))
	}
	if v := action.GetString("tags"); v != "" {
		parts = append(parts, fmt.Sprintf("tags=%s", v))
	}
	if v := action.GetString("exclude_keywords"); v != "" {
		parts = append(parts, fmt.Sprintf("exclude=%q", v))
	}
	if v := action.GetString("source_type"); v != "" {
		parts = append(parts, fmt.Sprintf("source=%s", v))
	}
	if v := action.GetString("runtime_id"); v != "" {
		parts = append(parts, fmt.Sprintf("runtime=%s", v))
	}
	if v := action.GetInt("limit"); v > 0 {
		parts = append(parts, fmt.Sprintf("limit=%d", v))
	}
	if len(parts) == 0 {
		return "(no filters)"
	}
	return strings.Join(parts, ", ")
}

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
