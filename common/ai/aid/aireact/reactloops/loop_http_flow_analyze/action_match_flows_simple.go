package loop_http_flow_analyze

import (
	"context"
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

var matchFlowsSimpleAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"match_flows_simple",
		`Simple and fast matching for common scenarios (covers 80% of use cases).

Use this when you need ONE simple matching condition:
- Built-in security patterns (recommended for security checks)
- Keywords matching (comma-separated words)
- Regex pattern matching
- Status code matching

Built-in security patterns:
- "sql_injection" - SQL injection detection
- "xss" - XSS detection
- "sensitive_data" - sensitive info exposure (in responses)
- "error_response" - error responses and stack traces
- "command_injection" - command injection attempts
- "path_traversal" - path traversal attempts
- "ssrf" - SSRF attempts
- "file_upload" - dangerous file upload attempts
- "xxe" - XML External Entity injection
- "ldap_injection" - LDAP injection attempts
- "nosql_injection" - NoSQL injection attempts
- "template_injection" - Server-Side Template Injection (SSTI)
- "open_redirect" - Open redirect attempts
- "crlf_injection" - CRLF injection attempts
- "debug_info" - Debug information disclosure
- "backup_files" - Backup file access attempts
- "jwt_token" - JWT token detection in responses
- "api_keys" - API keys and tokens exposure (AWS, Stripe, Google, GitHub, Slack)
- "database_error" - Database error messages
- "cors_misconfiguration" - CORS misconfiguration detection

Examples:
1. Security check: {"flow_source": "last", "security_pattern": "sql_injection"}
2. Keywords: {"flow_source": "login_flows", "keywords": "error,failed", "scope": "response"}
3. Regex: {"flow_source": "last", "regex": "\\d{15,16}", "scope": "response"}
4. Status: {"flow_source": "last", "status_codes": "500,502,503"}`,
		[]aitool.ToolOption{
			// 流量来源
			aitool.WithStringParam("flow_source", aitool.WithParam_Description("Query result name to match against (e.g. 'login_flows', 'last', 'last_query'). Leave empty to use flow_ids or last query.")),
			aitool.WithStringParam("flow_ids", aitool.WithParam_Description("Comma-separated flow IDs, e.g. '123,456,789'. Use this OR flow_source, not both.")),

			// 匹配模式（四选一）
			aitool.WithStringParam("security_pattern",
				aitool.WithParam_Description("Built-in security pattern for common vulnerability detection"),
				aitool.WithParam_Enum(
					"sql_injection",
					"xss",
					"sensitive_data",
					"error_response",
					"command_injection",
					"path_traversal",
					"ssrf",
					"file_upload",
					"xxe",
					"ldap_injection",
					"nosql_injection",
					"template_injection",
					"open_redirect",
					"crlf_injection",
					"debug_info",
					"backup_files",
					"jwt_token",
					"api_keys",
					"database_error",
					"cors_misconfiguration",
				),
			),
			aitool.WithStringParam("keywords", aitool.WithParam_Description("Comma-separated keywords for word matching")),
			aitool.WithStringParam("regex", aitool.WithParam_Description("Regular expression pattern")),
			aitool.WithStringParam("status_codes", aitool.WithParam_Description("Comma-separated status codes, e.g. '200,404,5xx'")),

			// 通用参数
			aitool.WithStringParam("scope", aitool.WithParam_Description("Matching scope: request/response/all (default: all)"), aitool.WithParam_Enum("request", "response", "all")),
			aitool.WithBoolParam("negative", aitool.WithParam_Description("Invert match result to exclude matched flows (default: false)")),

			// 结果命名
			aitool.WithStringParam("match_name", aitool.WithParam_Description("Optional name for this match result. Auto-generated if not provided.")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			// 验证必须有且只有一个匹配模式
			patterns := 0
			if action.GetString("security_pattern") != "" {
				patterns++
			}
			if action.GetString("keywords") != "" {
				patterns++
			}
			if action.GetString("regex") != "" {
				patterns++
			}
			if action.GetString("status_codes") != "" {
				patterns++
			}

			if patterns == 0 {
				return utils.Errorf("must provide exactly ONE matching pattern: security_pattern, keywords, regex, or status_codes")
			}
			if patterns > 1 {
				return utils.Errorf("can only use ONE matching pattern at a time. Use match_flows for complex multi-matcher scenarios")
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
						log.Infof("[match_flows_simple] using attached flow IDs: %d flows", len(sourceFlowIDs))
					}
				}
			}

			// Fallback to all flows (empty IDs means no filter)
			if len(sourceFlowIDs) == 0 {
				sourceQuery = "all"
				log.Infof("[match_flows_simple] no flow IDs specified, matching all flows in database")
			}

			log.Infof("[match_flows_simple] source: '%s', flow count: %d", sourceQuery, len(sourceFlowIDs))

			// === 2. 构建 matcher ===
			var matcher SimplifiedMatcher
			var matcherDesc string

			if secPattern := action.GetString("security_pattern"); secPattern != "" {
				pattern := getSecurityPattern(secPattern)
				if pattern == nil {
					operator.Fail(fmt.Sprintf("unknown security pattern: %s", secPattern))
					return
				}
				// 使用内置模式的第一个 matcher（简化）
				if len(pattern.Matchers) == 0 {
					operator.Fail(fmt.Sprintf("security pattern '%s' has no matchers", secPattern))
					return
				}
				matcher = pattern.Matchers[0]
				matcherDesc = fmt.Sprintf("security_pattern=%s", secPattern)
			} else if keywords := action.GetString("keywords"); keywords != "" {
				scope := action.GetString("scope", "all")
				negative := action.GetBool("negative")
				matcher = SimplifiedMatcher{
					Type:     "word",
					Patterns: splitMulti(keywords),
					Scope:    scope,
					MatchAll: false,
					Negative: negative,
				}
				matcherDesc = fmt.Sprintf("keywords=%s, scope=%s", keywords, scope)
			} else if regex := action.GetString("regex"); regex != "" {
				scope := action.GetString("scope", "all")
				negative := action.GetBool("negative")
				matcher = SimplifiedMatcher{
					Type:     "regex",
					Patterns: []string{regex},
					Scope:    scope,
					Negative: negative,
				}
				matcherDesc = fmt.Sprintf("regex=%s, scope=%s", regex, scope)
			} else if statusCodes := action.GetString("status_codes"); statusCodes != "" {
				negative := action.GetBool("negative")
				matcher = SimplifiedMatcher{
					Type:     "status",
					Patterns: splitMulti(statusCodes),
					Negative: negative,
				}
				matcherDesc = fmt.Sprintf("status_codes=%s", statusCodes)
			} else {
				operator.Fail("no matching pattern provided")
				return
			}

			// === 2.5. 构建并发送第1行累积流（参数摘要 + 可选思考）===
			thought := action.GetString("human_readable_thought")
			var line1 string
			if thought != "" {
				line1 = fmt.Sprintf("匹配 source=%s, %s | %s", sourceQuery, matcherDesc, thought)
			} else {
				line1 = fmt.Sprintf("匹配 source=%s, %s", sourceQuery, matcherDesc)
			}
			reactloops.EmitActionLog(loop, nodeId, line1)

			// === 3. 执行匹配（流式处理）===
			reactloops.EmitStatus(loop, "匹配流量中 / Matching Flows...")

			var matchedFlows []*schema.HTTPFlow
			var totalCount int
			var discardCount int
			var builder strings.Builder

			log.Infof("[match_flows_simple] matching flows from source '%s' with: %s", sourceQuery, matcherDesc)

			builder.WriteString(fmt.Sprintf("Matching flows from source '%s' with: %s\n\n", sourceQuery, matcherDesc))

			yakMatcher := convertSimplifiedToYakMatcher(&matcher)

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

				matched, err := yakMatcher.Execute(&httptpl.RespForMatch{
					RawPacket:     []byte(flowResponse(flow)),
					RequestPacket: []byte(flowRequest(flow)),
				}, nil)

				if err != nil {
					builder.WriteString(fmt.Sprintf(" - #%d [error] %v\n", flow.ID, err))
					discardCount++
					continue
				}

				if !matched {
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
				fmt.Sprintf("match_simple_%s_%s.txt", matchName, utils.DatetimePretty2()))

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

			// === 6.5. 构建并发送第2行累积流（结果摘要）===
			line2 := fmt.Sprintf("完成: 匹配 %d/%d 条流量",
				len(matchedFlows), totalCount)
			reactloops.EmitActionLog(loop, nodeId, line2, summary)

			// === 7. 记录历史 ===
			recordAction(loop,
				"match_flows_simple",
				fmt.Sprintf("source=%s, %s", sourceQuery, matcherDesc),
				fmt.Sprintf("matched %d/%d flows, saved as '%s'", len(matchedFlows), totalCount, matchName),
				matcherDesc)

			// === 8. 返回结果 ===
			feedbackMsg := fmt.Sprintf("Match '%s' completed: matched %d flows from source '%s' (%d total)\n\nFile: %s\n\n%s",
				matchName, len(matchedFlows), sourceQuery, totalCount, filename, summary)

			invoker := loop.GetInvoker()
			if invoker != nil {
				invoker.AddToTimeline("match_flows_simple", feedbackMsg)
			}

			operator.Feedback(feedbackMsg)
		},
	)
}
