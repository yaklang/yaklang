package loop_http_flow_analyze

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var matchFlowsAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"match_flows",
		`Advanced matching with multiple matchers and complex logic (for 20% complex scenarios).

Use this when you need:
- Multiple matchers with AND/OR logic between them
- Complex nested conditions
- Fine-grained control over matching behavior

**Matcher Structure Definition:**

{
  "type": "word" | "regex" | "status" | "binary" | "dsl",
  "patterns": ["pattern1", "pattern2"],  // Required: list of patterns to match
  "scope": "request" | "response" | "all",  // Optional, default "all"
  "match_all": false,  // Optional: patterns logic (false=OR, true=AND), default false
  "negative": false,   // Optional: invert match, default false
  "encoding": "hex" | "base64",  // Optional: only for type=binary
  "expr_type": "nuclei-dsl"      // Optional: only for type=dsl
}

**Parameters:**

- matchers: Array of Matcher objects (see structure above)
- matcher_condition: "or" | "and" (default "or")
  - "or": ANY matcher matches → flow matches
  - "and": ALL matchers match → flow matches

**Logic Explanation:**

1. Within each Matcher: patterns are combined by "match_all"
   - match_all=false (OR): pattern1 OR pattern2 OR ...
   - match_all=true (AND): pattern1 AND pattern2 AND ...

2. Between Matchers: combined by "matcher_condition"
   - matcher_condition="or": matcher1 OR matcher2 OR ...
   - matcher_condition="and": matcher1 AND matcher2 AND ...

**Examples:**

1. Find flows with (admin OR root in request) AND (5xx status):
{
  "matchers": [
    {"type": "word", "patterns": ["admin", "root"], "scope": "request", "match_all": false},
    {"type": "status", "patterns": ["500", "502", "503"]}
  ],
  "matcher_condition": "and"
}

2. Find flows containing BOTH "error" AND "exception" in response:
{
  "matchers": [
    {"type": "word", "patterns": ["error", "exception"], "scope": "response", "match_all": true}
  ]
}

3. Exclude successful responses (NOT 2xx):
{
  "matchers": [
    {"type": "status", "patterns": ["200", "201", "204"], "negative": true}
  ]
}

For simple single-condition matching, use match_flows_simple instead.`,
		[]aitool.ToolOption{
			// 流量来源
			aitool.WithStringParam("flow_source", aitool.WithParam_Description("Query result name to match against (e.g. 'login_flows', 'last'). Leave empty to use flow_ids or last query.")),
			aitool.WithStringParam("flow_ids", aitool.WithParam_Description("Comma-separated flow IDs, e.g. '123,456,789'. Use this OR flow_source, not both.")),

			// 高级匹配参数
			aitool.WithStringParam("matchers", aitool.WithParam_Description("JSON array of Matcher objects. Each Matcher has: type (word/regex/status/binary/dsl), patterns (array), scope (request/response/all), match_all (bool), negative (bool). See action description for full structure definition.")),
			aitool.WithStringParam("matcher_condition", aitool.WithParam_Description("Logic between multiple matchers: 'or' (any matcher matches, default) or 'and' (all matchers must match)")),

			// 结果命名
			aitool.WithStringParam("match_name", aitool.WithParam_Description("Optional name for this match result. Auto-generated if not provided.")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			// 验证 matchers 参数
			matchersJSON := action.GetString("matchers")
			if matchersJSON == "" {
				return utils.Errorf("matchers parameter is required. For simple matching, use match_flows_simple instead")
			}

			// 验证 JSON 格式
			var matchers []SimplifiedMatcher
			if err := json.Unmarshal([]byte(matchersJSON), &matchers); err != nil {
				return utils.Errorf("failed to parse matchers JSON: %v", err)
			}

			if len(matchers) == 0 {
				return utils.Errorf("matchers array cannot be empty")
			}

			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			// === 1. 获取流量来源 (使用 fallback 逻辑) ===
			nodeId := "http-flow-match"
			reactloops.EmitStatus(loop, "准备匹配流量 / Preparing to Match Flows...")

			db := consts.GetGormProjectDatabase()
			if db == nil {
				operator.Fail("project database is not available")
				return
			}

			var sourceFlowIDs []int64
			var sourceQuery string

			flowSource := action.GetString("flow_source")
			flowIDsStr := action.GetString("flow_ids")

			// Fallback 逻辑: 参数输入 -> attached IDs -> 无条件全流量
			if flowSource != "" {
				// 尝试从查询结果获取
				queryResult := getQueryResult(loop, flowSource)
				if queryResult != nil && len(queryResult.FlowIDs) > 0 {
					sourceFlowIDs = queryResult.FlowIDs
					sourceQuery = queryResult.Name
				} else {
					log.Warnf("query result '%s' not found or empty, falling back to attached flows", flowSource)
				}
			} else if flowIDsStr != "" {
				// 直接提供的 flow IDs
				sourceFlowIDs = parseFlowIDs(flowIDsStr)
				if len(sourceFlowIDs) > 0 {
					sourceQuery = "direct_ids"
				}
			}

			// Fallback to attached flows
			if len(sourceFlowIDs) == 0 {
				if attachedIDsRaw := loop.GetVariable(attachedHTTPFlowIDsKey); attachedIDsRaw != nil {
					if attachedIDs, ok := attachedIDsRaw.([]int64); ok && len(attachedIDs) > 0 {
						sourceFlowIDs = attachedIDs
						sourceQuery = "attached"
						log.Infof("[match_flows] using attached flow IDs: %d flows", len(sourceFlowIDs))
					}
				}
			}

			// Fallback to all flows (empty IDs means no filter)
			if len(sourceFlowIDs) == 0 {
				sourceQuery = "all"
				log.Infof("[match_flows] no flow IDs specified, matching all flows in database")
			}

			log.Infof("[match_flows] source: '%s', flow count: %d", sourceQuery, len(sourceFlowIDs))

			// === 2. 解析 matchers ===
			matchersJSON := action.GetString("matchers")
			var matchers []SimplifiedMatcher
			if err := json.Unmarshal([]byte(matchersJSON), &matchers); err != nil {
				operator.Fail(fmt.Sprintf("failed to parse matchers JSON: %v", err))
				return
			}

			matcherCondition := strings.ToLower(action.GetString("matcher_condition", "or"))
			if matcherCondition != "or" && matcherCondition != "and" {
				matcherCondition = "or"
			}

			matcherDesc := describeSimplifiedMatchers(matchers)
			if len(matchers) > 1 {
				matcherDesc += fmt.Sprintf(" (condition=%s)", matcherCondition)
			}

			// === 3. 构建并发送第1行累积流（参数摘要）===
			line1 := fmt.Sprintf("匹配 source=%s, %s", sourceQuery, matcherDesc)
			reactloops.EmitActionLog(loop, nodeId, line1)

			// === 4. 执行匹配（流式处理）===
			reactloops.EmitStatus(loop, "匹配流量中 / Matching Flows...")

			var matchedFlows []*schema.HTTPFlow
			var totalCount int
			var discardCount int
			var builder strings.Builder

			log.Infof("[match_flows] matching flows from source '%s' with %d matcher(s): %s", sourceQuery, len(matchers), matcherDesc)

			builder.WriteString(fmt.Sprintf("Matching flows from source '%s' with %d matcher(s) (condition=%s)\n",
				sourceQuery, len(matchers), matcherCondition))
			builder.WriteString(fmt.Sprintf("Matchers: %s\n\n", matcherDesc))

			// 转换为 YakMatcher
			yakMatchers := make([]*httptpl.YakMatcher, len(matchers))
			for i, m := range matchers {
				yakMatchers[i] = convertSimplifiedToYakMatcher(&m)
			}

			// 使用流式处理代替循环加载
			ctx := context.Background()
			filter := &ypb.QueryHTTPFlowRequest{}
			if len(sourceFlowIDs) > 0 {
				filter.IncludeId = sourceFlowIDs
			}

			for flow := range yakit.YieldHTTPFlowsByFilter(db, ctx, filter) {
				totalCount++
				if totalCount > 10 && totalCount%100 == 0 {
					reactloops.EmitProgress(loop, totalCount, 0, "匹配进度", "Matching")
				}

				// 执行匹配逻辑
				respForMatch := &httptpl.RespForMatch{
					RawPacket:     []byte(flowResponse(flow)),
					RequestPacket: []byte(flowRequest(flow)),
				}

				var matchResults []bool
				var matchErr error

				for _, yakMatcher := range yakMatchers {
					matched, err := yakMatcher.Execute(respForMatch, nil)
					if err != nil {
						matchErr = err
						break
					}
					matchResults = append(matchResults, matched)
				}

				if matchErr != nil {
					builder.WriteString(fmt.Sprintf(" - #%d [error] %v\n", flow.ID, matchErr))
					discardCount++
					continue
				}

				// 根据 matcher_condition 判断最终结果
				finalMatched := false
				if matcherCondition == "and" {
					// AND: 所有 matcher 都必须匹配
					finalMatched = true
					for _, m := range matchResults {
						if !m {
							finalMatched = false
							break
						}
					}
				} else {
					// OR: 任一 matcher 匹配即可
					for _, m := range matchResults {
						if m {
							finalMatched = true
							break
						}
					}
				}

				if !finalMatched {
					discardCount++
					continue
				}

				matchedFlows = append(matchedFlows, flow)
				builder.WriteString(fmt.Sprintf("%d) #%d [%s] %d %s | tags=%s | src=%s\n",
					len(matchedFlows),
					flow.ID,
					flow.Method,
					flow.StatusCode,
					utils.ShrinkString(flow.Url, 160),
					shrinkTags(flow.Tags),
					flow.SourceType,
				))
			}

			builder.WriteString(fmt.Sprintf("\nMatched %d flow(s) from %d total; discarded %d.",
				len(matchedFlows), totalCount, discardCount))

			summary := builder.String()

			// === 4. 保存结果 ===
			matchName := action.GetString("match_name")
			if matchName == "" {
				matchName = fmt.Sprintf("match_%d", loop.GetCurrentIterationIndex())
			}

			loopDataDir := loop.GetLoopContentDir("data")
			filename := filepath.Join(loopDataDir,
				fmt.Sprintf("match_advanced_%s_%s.txt", matchName, utils.DatetimePretty2()))

			fullSummary := summary
			if len(fullSummary) > maxHTTPFlowSummaryBytes {
				preview := utils.ShrinkTextBlock(fullSummary, 2000)
				summary = fmt.Sprintf("Summary length %d exceeded %d; saved to file: %s\nUse file reading tool to load full content.\n\nPreview:\n%s",
					len(fullSummary), maxHTTPFlowSummaryBytes, filename, preview)
			}

			if err := reactloops.SaveAndPinFile(loop, filename, []byte(fullSummary)); err != nil {
				log.Warnf("failed to save file: %v", err)
			}

			// === 5. 保存状态 ===
			matchedFlowIDs := make([]int64, len(matchedFlows))
			for i, f := range matchedFlows {
				matchedFlowIDs[i] = int64(f.ID)
			}

			matchResult := &MatchResult{
				Name:         matchName,
				SourceQuery:  sourceQuery,
				FlowIDs:      matchedFlowIDs,
				MatchedCount: len(matchedFlows),
				MatcherDesc:  matcherDesc,
				SummaryFile:  filename,
				CreatedAt:    time.Now(),
			}
			saveMatchResult(loop, matchResult)

			// === 6. 发送完成状态 ===
			reactloops.EmitStatus(loop, fmt.Sprintf("匹配完成，找到 %d 条 / Match Complete, Found %d Flows", len(matchedFlows), len(matchedFlows)))

			// === 7. 构建并发送第2行累积流（结果摘要）===
			line2 := fmt.Sprintf("完成: 匹配 %d/%d 条流量",
				len(matchedFlows), totalCount)
			reactloops.EmitActionLog(loop, nodeId, line2, summary)

			// === 8. 记录历史 ===
			recordAction(loop,
				"match_flows",
				fmt.Sprintf("source=%s, %s", sourceQuery, matcherDesc),
				fmt.Sprintf("matched %d/%d flows, saved as '%s'", len(matchedFlows), totalCount, matchName),
				matcherDesc)

			// === 8. 返回结果 ===
			feedbackMsg := fmt.Sprintf("Match '%s' completed: matched %d flows from source '%s' (%d total)\n\nFile: %s\n\n%s",
				matchName, len(matchedFlows), sourceQuery, totalCount, filename, summary)

			invoker := loop.GetInvoker()
			if invoker != nil {
				invoker.AddToTimeline("match_flows", feedbackMsg)
			}

			operator.Feedback(feedbackMsg)
		},
	)
}
