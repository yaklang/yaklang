package loop_http_flow_analyze

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const maxHTTPFlowSummaryBytes = 1024 * 5

type matcherPayload struct {
	Matchers []*ypb.HTTPResponseMatcher `json:"matchers"`
}

func parseMatchers(raw string) ([]*ypb.HTTPResponseMatcher, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, utils.Error("matchers_json is empty")
	}

	var wrapper matcherPayload
	if err := json.Unmarshal([]byte(raw), &wrapper); err == nil && len(wrapper.Matchers) > 0 {
		return wrapper.Matchers, nil
	}

	var arr []*ypb.HTTPResponseMatcher
	if err := json.Unmarshal([]byte(raw), &arr); err == nil && len(arr) > 0 {
		return arr, nil
	}

	var single ypb.HTTPResponseMatcher
	if err := json.Unmarshal([]byte(raw), &single); err == nil && single.MatcherType != "" {
		return []*ypb.HTTPResponseMatcher{&single}, nil
	}

	return nil, utils.Errorf("failed to parse matchers_json, expected HTTPResponseMatcher array or object")
}

var filterAndMatchHTTPFlowsAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"filter_and_match_http_flows",
		"Filter HTTP flows by keyword/keyword_type/methods/status_code/tags/exclude_keywords/url_contains/runtime_id/source_type/limit first; only add matchers_json if filters cannot narrow the set. If the summary exceeds 20k chars it will be saved to a file—use the file reading tool to load the full content.",
		[]aitool.ToolOption{
			aitool.WithStringParam("keyword", aitool.WithParam_Description("Fuzzy search keyword across request/response/url")),
			aitool.WithStringParam("keyword_type", aitool.WithParam_Description("Limit keyword scope: request/response or leave empty for all")),
			aitool.WithStringParam("methods", aitool.WithParam_Description("Comma separated HTTP methods to include, e.g. GET,POST")),
			aitool.WithStringParam("status_code", aitool.WithParam_Description("Status codes or ranges, e.g. 200,404,5xx")),
			aitool.WithStringParam("tags", aitool.WithParam_Description("Comma or pipe separated tags to match")),
			aitool.WithStringParam("exclude_keywords", aitool.WithParam_Description("Keywords to exclude from request/response/url")),
			aitool.WithStringParam("url_contains", aitool.WithParam_Description("URL substring filter; multiple values separated by comma")),
			aitool.WithStringParam("runtime_id", aitool.WithParam_Description("Filter flows by runtime/session id")),
			aitool.WithStringParam("source_type", aitool.WithParam_Description("Filter by source type, e.g. mitm/crawler/scan")),
			aitool.WithIntegerParam("limit", aitool.WithParam_Description("Max result count (default 30, max 500)")),
			aitool.WithStringParam("matchers_json", aitool.WithParam_Description("Optional HTTPResponseMatcher definitions in JSON (single object, array, or {\"matchers\":[...]}). Prefer filters first; only use this when filters cannot isolate results. Enums must use allowed values only.")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			limit := action.GetInt("limit")
			if limit < 0 {
				return utils.Errorf("limit must be non-negative")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			db := consts.GetGormProjectDatabase()
			if db == nil {
				operator.Fail("project database is not available for matching HTTP flows")
				return
			}

			req := buildQueryRequestFromAction(action, 30)
			paging, flows, err := yakit.QueryHTTPFlow(db, req)
			if err != nil {
				log.Errorf("filter_and_match_http_flows query failed: %v", err)
				operator.Fail(fmt.Sprintf("query http flows failed: %v", err))
				return
			}

			total := 0
			if paging != nil {
				total = paging.TotalRecord
			} else {
				total = len(flows)
			}

			var (
				matchedCount  int
				discardCount  int
				localMatchers []*simpleMatcher
				builder       strings.Builder
			)

			rawMatchers := strings.TrimSpace(action.GetString("matchers_json"))
			if rawMatchers != "" {
				parsed, err := parseMatchers(rawMatchers)
				if err != nil {
					operator.Fail(err)
					return
				}
				for _, m := range parsed {
					localMatchers = append(localMatchers, newSimpleMatcherFromGRPC(m))
				}
			}

			builder.WriteString(fmt.Sprintf("HTTP flow query returned %d items (showing %d)", total, len(flows)))
			if len(localMatchers) > 0 {
				builder.WriteString(fmt.Sprintf("; applying %d matcher(s)\n", len(localMatchers)))
			} else {
				builder.WriteString("\n")
			}

			if len(localMatchers) == 0 {
				for idx, f := range flows {
					builder.WriteString(fmt.Sprintf("%d) ID: %d, URL: %s, Method: %s, Status: %d, Tags: %s, Source: %s\n",
						idx+1,
						f.ID,
						f.Url,
						f.Method,
						f.StatusCode,
						shrinkTags(f.Tags),
						f.SourceType,
					))
				}
			} else {
				for _, flow := range flows {
					matched, err := executeMatchers(
						localMatchers,
						&httptpl.RespForMatch{
							RawPacket:     []byte(flowResponse(flow)),
							RequestPacket: []byte(flowRequest(flow)),
						},
					)

					if err != nil {
						builder.WriteString(fmt.Sprintf(" - #%d [error] %v\n", flow.ID, err))
					}
					if !matched {
						discardCount++
						continue
					}

					matchedCount++
					builder.WriteString(fmt.Sprintf("%d) #%d [%s] %d %s | tags=%s | src=%s\n",
						matchedCount,
						flow.ID,
						flow.Method,
						flow.StatusCode,
						utils.ShrinkString(flow.Url, 160),
						shrinkTags(flow.Tags),
						flow.SourceType,
					))
				}

				builder.WriteString(fmt.Sprintf("\nMatched %d flow(s); discarded %d after matcher filter.", matchedCount, discardCount))
			}

			invoker := loop.GetInvoker()
			fullSummary := builder.String()
			summary := fullSummary

			// 总是保存到文件
			var filename string
			if invoker != nil {
				loopDataDir := loop.GetLoopContentDir("data")
				filename = filepath.Join(loopDataDir, fmt.Sprintf("http_flow_match_summary_%d_%s.txt", loop.GetCurrentIterationIndex(), utils.DatetimePretty2()))
				loop.Set("last_query_summary_file", filename)
				if len(localMatchers) > 0 {
					loop.Set("last_match_summary_file", filename)
				}
				loop.GetEmitter().EmitPinFilename(filename)
			}

			// 只有超过限制时才修改 summary
			if len(fullSummary) > maxHTTPFlowSummaryBytes && filename != "" {
				preview := utils.ShrinkTextBlock(fullSummary, 2000)
				summary = fmt.Sprintf("Summary length %d exceeded %d; saved to file: %s\nUse `read_reference_file` (or other file-reading tool) to load the full content.\n\nPreview:\n%s",
					len(fullSummary), maxHTTPFlowSummaryBytes, filename, preview)
			}

			invoker.AddToTimeline("filter_and_match_http_flows", summary)
			loop.Set("last_query_summary", summary)
			if len(localMatchers) > 0 {
				loop.Set("last_match_summary", summary)
			}

			operator.Feedback(summary)
		},
	)
}
