package loop_http_flow_analyze

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var queryHTTPFlowsAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"query_http_flows",
		"Query HTTP flows from database with filters. Returns a list of flows that can be referenced by subsequent match_flows calls using flow_source parameter. Results are automatically saved with a name for later reference.",
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
			aitool.WithStringParam("query_name", aitool.WithParam_Description("Optional name for this query result, used for later reference. Auto-generated if not provided.")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			limit := action.GetInt("limit")
			if limit < 0 {
				return utils.Errorf("limit must be non-negative")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			// === 1. 提取 thought 和构建参数摘要 ===
			thought := action.GetString("human_readable_thought")
			paramSummary := buildSearchParamSummary(action)

			// 构建第一行累积流（操作摘要 + 可选思考）
			var line1 string
			if thought != "" {
				line1 = fmt.Sprintf("查询 %s | %s", paramSummary, thought)
			} else {
				line1 = fmt.Sprintf("查询 %s", paramSummary)
			}
			reactloops.EmitActionLog(loop, "http-flow-query", line1)

			// === 2. 发送瞬时状态 ===
			reactloops.EmitStatus(loop, "查询流量中 / Querying Flows...")

			// === 3. 执行查询 ===
			db := consts.GetGormProjectDatabase()
			if db == nil {
				operator.Fail("project database is not available")
				return
			}

			log.Infof("[query_http_flows] search params: %s", paramSummary)

			req := buildQueryRequestFromAction(action, 30)
			paging, flows, err := yakit.QueryHTTPFlow(db, req)
			if err != nil {
				log.Errorf("[query_http_flows] query failed: %v", err)
				operator.Fail(fmt.Sprintf("query http flows failed: %v", err))
				return
			}

			total := 0
			if paging != nil {
				total = paging.TotalRecord
			} else {
				total = len(flows)
			}

			// === 4. 生成结果摘要 ===
			var builder strings.Builder
			builder.WriteString(fmt.Sprintf("HTTP flow query returned %d items (showing %d)\n\n", total, len(flows)))

			for idx, f := range flows {
				builder.WriteString(fmt.Sprintf("%d) ID: %d, URL: %s, Method: %s, Status: %d, Tags: %s, Source: %s\n",
					idx+1,
					f.ID,
					utils.ShrinkString(f.Url, 160),
					f.Method,
					f.StatusCode,
					shrinkTags(f.Tags),
					f.SourceType,
				))
			}

			summary := builder.String()

			// === 5. 生成查询名称 ===
			queryName := action.GetString("query_name")
			if queryName == "" {
				queryName = fmt.Sprintf("query_%d", loop.GetCurrentIterationIndex())
			}

			// === 6. 保存文件 ===
			loopDataDir := loop.GetLoopContentDir("data")
			filename := filepath.Join(loopDataDir,
				fmt.Sprintf("query_%s_%d_%s.txt", queryName, loop.GetCurrentIterationIndex(), utils.DatetimePretty2()))

			fullSummary := summary
			if len(fullSummary) > maxHTTPFlowSummaryBytes && filename != "" {
				preview := utils.ShrinkTextBlock(fullSummary, 300)
				summary = fmt.Sprintf("结果过长 (共 %d 字节)，已保存到文件。使用文件读取工具查看完整内容。\n\n预览:\n%s\n\n文件: %s",
					len(fullSummary), preview, filename)
			}

			if err := reactloops.SaveAndPinFile(loop, filename, []byte(fullSummary)); err != nil {
				log.Warnf("[query_http_flows] failed to save file: %v", err)
			}

			// === 7. 保存到 loop 状态 ===
			flowIDs := make([]int64, len(flows))
			for i, f := range flows {
				flowIDs[i] = int64(f.ID)
			}

			queryResult := &QueryResult{
				Name:        queryName,
				FlowIDs:     flowIDs,
				TotalCount:  total,
				QueryParams: paramSummary,
				SummaryFile: filename,
				CreatedAt:   time.Now(),
			}
			saveQueryResult(loop, queryResult)

			// === 8. 发送完成状态 ===
			reactloops.EmitStatus(loop, fmt.Sprintf("查询完成，找到 %d 条流量 / Query Complete, Found %d Flows", total, total))

			// === 9. 构建第二行累积流（结果摘要）===
			line2 := fmt.Sprintf("完成: 找到 %d 条流量",
				total)
			reactloops.EmitActionLog(loop, "http-flow-query", line2, summary)

			// === 10. 技术日志 ===
			log.Infof("[query_http_flows] query completed: total=%d, showing=%d, saved_as='%s', file=%s",
				total, len(flows), queryName, filename)

			// === 11. 返回结果 ===
			feedbackMsg := fmt.Sprintf("Query '%s' completed: found %d flows (showing %d)\n\nFile: %s\n\n%s",
				queryName, total, len(flows), filename, summary)

			invoker := loop.GetInvoker()
			if invoker != nil {
				invoker.AddToTimeline("query_http_flows", feedbackMsg)
			}

			operator.Feedback(feedbackMsg)
		},
	)
}
