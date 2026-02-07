package loop_http_flow_analyze

import (
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

var matchHTTPFlowsWithSimpleMatcherAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"match_http_flows_with_matcher",
		"Query HTTP flows with filters and apply a single HTTPResponseMatcher. Use this for simple matching scenarios where you need one matcher condition. For complex multi-matcher logic, use filter_and_match_http_flows instead.",
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

			// HTTPResponseMatcher 字段
			aitool.WithStringParam("matcher_type", aitool.WithParam_Description("Matcher type: word/regex/status_code/binary/dsl/nuclei-dsl")),
			aitool.WithStringParam("scope", aitool.WithParam_Description("Match scope: raw(default)/header/body/all/request/response/all_headers/all_bodies")),
			aitool.WithStringParam("condition", aitool.WithParam_Description("Logical condition: and/or (when Group has multiple items)")),
			aitool.WithStringParam("group", aitool.WithParam_Description("Match patterns/values, comma separated")),
			aitool.WithStringParam("group_encoding", aitool.WithParam_Description("Group value encoding: (empty)/hex/base64")),
			aitool.WithBoolParam("negative", aitool.WithParam_Description("Negative match: true to invert the match result")),
			aitool.WithStringParam("expr_type", aitool.WithParam_Description("Expression type: (empty)/nuclei-dsl")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			limit := action.GetInt("limit")
			if limit < 0 {
				return utils.Errorf("limit must be non-negative")
			}

			// 检查是否提供了 matcher 参数
			matcherType := strings.TrimSpace(action.GetString("matcher_type"))
			if matcherType == "" {
				return utils.Errorf("matcher_type is required")
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
				log.Errorf("match_http_flows_with_matcher query failed: %v", err)
				operator.Fail(fmt.Sprintf("query http flows failed: %v", err))
				return
			}

			total := 0
			if paging != nil {
				total = paging.TotalRecord
			} else {
				total = len(flows)
			}

			// 构建 HTTPResponseMatcher
			matcher := &ypb.HTTPResponseMatcher{
				MatcherType:   action.GetString("matcher_type"),
				Scope:         action.GetString("scope"),
				Condition:     action.GetString("condition"),
				GroupEncoding: action.GetString("group_encoding"),
				Negative:      action.GetBool("negative"),
				ExprType:      action.GetString("expr_type"),
			}

			// 解析 group 参数（逗号分隔）
			groupStr := strings.TrimSpace(action.GetString("group"))
			if groupStr != "" {
				matcher.Group = splitMulti(groupStr)
			}

			// 如果 scope 为空，设置默认值
			if matcher.Scope == "" {
				matcher.Scope = "raw"
			}

			var (
				matchedCount int
				discardCount int
				builder      strings.Builder
			)

			localMatcher := newSimpleMatcherFromGRPC(matcher)
			localMatchers := []*simpleMatcher{localMatcher}

			builder.WriteString(fmt.Sprintf("HTTP flow query returned %d items (showing %d); applying matcher (type=%s, scope=%s)\n",
				total, len(flows), matcher.MatcherType, matcher.Scope))

			pr, pw := utils.NewPipe()
			defer pw.Close()

			emitter := loop.GetEmitter()
			var streamId string
			if event, _ := emitter.EmitDefaultStreamEvent("thought", pr, loop.GetCurrentTask().GetId()); event != nil {
				streamId = event.GetStreamEventWriterId()
			}

			pw.WriteString(fmt.Sprintf("Found [%v] HTTP flows...", len(flows)))

			if len(flows) <= 0 {
				pw.WriteString(fmt.Sprintf("[DONE]"))
			}

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

			invoker := loop.GetInvoker()
			fullSummary := builder.String()
			summary := fullSummary

			if streamId != "" {
				emitter.EmitTextReferenceMaterial(streamId, fullSummary)
			}

			// 总是保存到文件
			var filename string
			if invoker != nil {
				loopDir := loop.Get("loop_directory")
				filename = filepath.Join(loopDir, fmt.Sprintf("http_flow_simple_match_summary_%d_%s.txt", loop.GetCurrentIterationIndex(), utils.DatetimePretty2()))
				loop.Set("last_match_summary_file", filename)
				loop.GetEmitter().EmitPinFilename(filename)
			}

			// 只有超过限制时才修改 summary
			if len(fullSummary) > maxHTTPFlowSummaryBytes && filename != "" {
				preview := utils.ShrinkTextBlock(fullSummary, 2000)
				summary = fmt.Sprintf("Summary length %d exceeded %d; saved to file: %s\nUse `read_reference_file` (or other file-reading tool) to load the full content.\n\nPreview:\n%s",
					len(fullSummary), maxHTTPFlowSummaryBytes, filename, preview)
			}

			invoker.AddToTimeline("match_http_flows_with_matcher", summary)
			loop.Set("last_match_summary", summary)

			operator.Feedback(summary)
		},
	)
}
