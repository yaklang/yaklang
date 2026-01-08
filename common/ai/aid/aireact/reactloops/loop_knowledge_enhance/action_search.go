package loop_knowledge_enhance

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

// compressKnowledgeResults compresses and refines knowledge search results using AI, filtering by text coordinates
func compressKnowledgeResults(resultStr string, queries []string, userContext string, invoker aicommon.AIInvokeRuntime, op *reactloops.LoopActionHandlerOperator, loop *reactloops.ReActLoop) string {
	if len(resultStr) == 0 {
		return resultStr
	}

	resultEditor := memedit.NewMemEditor(resultStr)
	dNonce := utils.RandStringBytes(4)

	promptTemplate := `
{{ if .userContext }}<|USER_CONTEXT_{{ .nonce }}|>
{{ .userContext }}
<|USER_CONTEXT_END_{{ .nonce }}|>

{{ end }}<|KNOWLEDGE_RESULTS_{{ .nonce }}|>
{{ .samples }}
<|KNOWLEDGE_RESULTS_END_{{ .nonce }}|>

<|INSTRUCT_{{ .nonce }}|>
【智能知识内容提取与精炼】

请严格根据用户查询从知识库搜索结果中提取最有价值的知识条目，按相关性和重要性排序：

【核心原则】
{{ if .userContext }}- 必须与用户需求直接相关
- 过滤掉所有无关的知识条目
- 优先选择能直接回答用户问题的知识
{{ else }}- 提取最具代表性和价值的知识内容
- 按主题相关性排序
- 去除重复和冗余信息
{{ end }}
【提取要求】
1. 最多提取 8 个最相关的知识条目
2. 每个条目应包含完整的上下文和关键信息
3. 按相关性从高到低排序（rank: 1最相关，数字越大越不相关）
4. 严格过滤无关内容

【重要性评判标准】（按优先级排序）
🔥 最高优先级 (rank 1-2)：
- 直接回答用户查询的核心知识
- 包含关键概念定义和解释
- 展示最佳实践和解决方案

⭐ 高优先级 (rank 3-5)：
- 包含重要补充信息和细节
- 相关示例和应用场景
- 重要的技术规范和要求

📝 中等优先级 (rank 6-8)：
- 辅助性信息和背景知识
- 相关术语解释和概念澄清
- 补充性的技术细节

【输出格式】
返回JSON数组，每个元素包含：
{
  "range": "startLine-endLine",
  "rank": 数字(1-8),
  "reason": "选择理由，例如：包含xxx核心概念"
}

【严格要求】
- 总输出控制在60行以内
- 避免重复或相似的知识条目
- 确保每个条目都有实际参考价值
{{ if .userContext }}- 必须与用户需求相关，无关内容一律排除{{ end }}

请按相关性排序输出ranges数组。
<|INSTRUCT_END_{{ .nonce }}|>
`

	materials, err := utils.RenderTemplate(promptTemplate, map[string]any{
		"nonce":       dNonce,
		"samples":     utils.PrefixLinesWithLineNumbers(resultStr),
		"queries":     strings.Join(queries, ", "),
		"userContext": userContext,
	})

	if err != nil {
		log.Errorf("compressKnowledgeResults: template render failed: %v", err)
		return resultStr
	}

	var context = invoker.GetConfig().GetContext()
	if op != nil {
		context = op.GetTask().GetContext()
	}

	forgeResult, err := invoker.InvokeLiteForge(
		context,
		"extract-ranked-knowledge",
		materials,
		[]aitool.ToolOption{
			aitool.WithStructArrayParam(
				"ranges",
				[]aitool.PropertyOption{
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("要提取的知识条目范围"),
				},
				nil,
				aitool.WithStringParam("range", aitool.WithParam_Required(true), aitool.WithParam_Description("行数范围，格式：startLine-endLine")),
				aitool.WithIntegerParam("rank", aitool.WithParam_Description("相关性排名，1-8，1最相关")),
				aitool.WithStringParam("reason", aitool.WithParam_Description("选择该条目的理由")),
			),
		},
	)

	if err != nil {
		log.Errorf("compressKnowledgeResults: forge failed: %v", err)
		return resultStr
	}

	if forgeResult == nil {
		log.Warnf("compressKnowledgeResults: forge result is nil")
		return resultStr
	}

	// 解析提取的ranges
	ranges := forgeResult.GetInvokeParamsArray("ranges")
	if len(ranges) == 0 {
		log.Warnf("compressKnowledgeResults: no ranges extracted")
		return resultStr
	}

	type RankedRange struct {
		StartLine int
		EndLine   int
		Rank      int
		Reason    string
	}

	var rankedRanges []RankedRange
	var totalLines int

	for _, r := range ranges {
		rangeStr := r.GetString("range")
		rank := r.GetInt("rank")
		reason := r.GetString("reason")

		if rangeStr == "" {
			log.Warnf("compressKnowledgeResults: empty range")
			continue
		}

		// 解析范围字符串
		parts := strings.Split(rangeStr, "-")
		if len(parts) != 2 {
			log.Warnf("compressKnowledgeResults: invalid range format: %s", rangeStr)
			continue
		}

		startLine, err1 := strconv.Atoi(parts[0])
		endLine, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			log.Errorf("compressKnowledgeResults: parse range failed: %s, errors: %v, %v", rangeStr, err1, err2)
			continue
		}

		if startLine < 1 || endLine < startLine {
			log.Warnf("compressKnowledgeResults: invalid range values: %s (start=%d, end=%d)", rangeStr, startLine, endLine)
			continue
		}

		// 检查是否有实际内容
		text := resultEditor.GetTextFromPositionInt(startLine, 1, endLine, 1)
		if strings.TrimSpace(text) == "" {
			log.Warnf("compressKnowledgeResults: empty text for range: %s", rangeStr)
			continue
		}

		// 控制总行数不超过100行
		lineCount := endLine - startLine + 1
		if totalLines+lineCount > 100 {
			log.Warnf("compressKnowledgeResults: would exceed 100 lines limit, stopping at range: %s", rangeStr)
			break
		}

		rankedRanges = append(rankedRanges, RankedRange{
			StartLine: startLine,
			EndLine:   endLine,
			Rank:      int(rank),
			Reason:    reason,
		})
		totalLines += lineCount
	}

	if len(rankedRanges) == 0 {
		log.Warnf("compressKnowledgeResults: no valid ranges extracted")
		return resultStr
	}

	// 按rank排序
	sort.Slice(rankedRanges, func(i, j int) bool {
		return rankedRanges[i].Rank < rankedRanges[j].Rank
	})

	// 构建压缩后的结果
	var finalResult strings.Builder
	finalResult.WriteString("【AI智能提取】按相关性排序的知识条目：\n\n")

	emitter := loop.GetEmitter()

	for i, r := range rankedRanges {
		text := resultEditor.GetTextFromPositionInt(r.StartLine, 1, r.EndLine, 1)
		finalResult.WriteString(fmt.Sprintf("=== [相关性排名 #%d] ===\n", i+1))
		finalResult.WriteString(fmt.Sprintf("选择理由：%s\n", r.Reason))
		finalResult.WriteString("内容：\n")
		finalResult.WriteString(text)
		finalResult.WriteString("\n\n")

		// 为重要知识条目创建 artifacts
		if r.Rank <= 3 {
			iteration := loop.GetCurrentIterationIndex()
			if iteration <= 0 {
				iteration = 1
			}
			loopDir := loop.Get("loop_directory")
			if loopDir == "" {
				filename := invoker.EmitFileArtifactWithExt(fmt.Sprintf("key_knowledge_rank_%d_iter_%d", r.Rank, iteration), ".txt", "")
				emitter.EmitPinFilename(filename)

				// 写入文件内容，包含元信息
				artifactContent := fmt.Sprintf("迭代轮数：%d\n相关性排名：#%d\n选择理由：%s\n\n内容：\n%s", iteration, r.Rank, r.Reason, text)
				err := os.WriteFile(filename, []byte(artifactContent), 0644)
				if err != nil {
					log.Warnf("failed to write key knowledge artifact rank %d: %v", r.Rank, err)
				}
			} else {
				artifactDir := filepath.Join(loopDir, "key_knowledge", fmt.Sprintf("iter_%d", iteration))
				if err := os.MkdirAll(artifactDir, 0755); err != nil {
					log.Warnf("failed to create key knowledge directory: %v", err)
				}
				filename := filepath.Join(artifactDir, fmt.Sprintf("key_knowledge_rank_%d_iter_%d_%s.txt", r.Rank, iteration, utils.DatetimePretty2()))
				emitter.EmitPinFilename(filename)

				// 写入文件内容，包含元信息
				artifactContent := fmt.Sprintf("迭代轮数：%d\n相关性排名：#%d\n选择理由：%s\n\n内容：\n%s", iteration, r.Rank, r.Reason, text)
				err := os.WriteFile(filename, []byte(artifactContent), 0644)
				if err != nil {
					log.Warnf("failed to write key knowledge artifact rank %d: %v", r.Rank, err)
				}
			}
		}
	}

	// 如果有创建的artifacts，在结果中提及
	if len(rankedRanges) > 0 && rankedRanges[0].Rank <= 3 {
		finalResult.WriteString("📌 重要知识条目已保存到 artifacts 中，可供后续详细查看。\n")
	}

	log.Infof("compressKnowledgeResults: compressed from %d chars to %d chars, %d ranges",
		len(resultStr), len(finalResult.String()), len(rankedRanges))

	return finalResult.String()
}

// makeSearchAction builds a search action for the given mode ("semantic" or "keyword")
func makeSearchAction(r aicommon.AIInvokeRuntime, mode string) reactloops.ReActLoopOption {
	desc := "根据用户问题推测相关关键词并搜索指定的知识库，返回相关的知识条目"
	if mode == "keyword" {
		desc = "关键字搜索模式：为语义不擅长的结构化条目（如法条、章节）生成关键字并搜索知识库"
	} else if mode == "semantic" {
		desc = "语义搜索模式：问题驱动的语义检索，优先返回高相关性知识片段"
	}

	toolOpts := []aitool.ToolOption{
		aitool.WithStringArrayParam("knowledge_bases", aitool.WithParam_Description("要搜索的知识库名称列表，必须指定至少一个知识库"), aitool.WithParam_Required(true)),
		aitool.WithStringArrayParam("search_queries", aitool.WithParam_Description("用于搜索的多条查询语句，支持多角度检索（优先使用）")),
	}

	if mode == "keyword" {
		toolOpts = append(toolOpts, aitool.WithStringParam("keyword", aitool.WithParam_Description("用于关键字优先搜索的单条关键词或短语")))
	} else {
		toolOpts = append(toolOpts, aitool.WithStringParam("search_query", aitool.WithParam_Description("用于语义搜索的单条查询语句（完整句子）")))
	}

	return reactloops.WithRegisterLoopAction(
		fmt.Sprintf("search_knowledge_%s", mode),
		desc, toolOpts,
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			// Provide a bilingual, user-friendly loading status similar to exec.loadingStatus usage
			loop.LoadingStatus(fmt.Sprintf("验证参数中 - search_knowledge:%s / validating parameters - mode:%s", mode, mode))
			knowledgeBases := action.GetStringSlice("knowledge_bases")
			if len(knowledgeBases) == 0 {
				return utils.Error("knowledge_bases is required and must contain at least one knowledge base name")
			}
			// search_queries 可选，如果未提供，AI 将尝试生成
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			// Indicate start of execution with a clear bilingual status for clients
			loop.LoadingStatus(fmt.Sprintf("执行搜索中 - search_knowledge:%s / executing search - mode:%s", mode, mode))
			// 获取参数
			knowledgeBases := action.GetStringSlice("knowledge_bases")
			searchQueries := action.GetStringSlice("search_queries")
			searchQuery := action.GetString("search_query")
			keyword := action.GetString("keyword")

			invoker := loop.GetInvoker()
			ctx := loop.GetConfig().GetContext()
			task := loop.GetCurrentTask()
			if task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}

			// 单条查询/关键词处理：优先使用 action 中提供的单条 query/keyword；
			// 如果未提供，则尝试使用 loop 上次保存的 last 搜索（减少 AI 生成）
			userContext := fmt.Sprintf("用户需求：%s", loop.Get("user_query"))
			verifyQuery := buildVerifyQueryWithCoreSummary(loop)
			var queriesToUse []string
			if mode == "keyword" {
				if len(searchQueries) > 0 {
					queriesToUse = append(queriesToUse, searchQueries...)
				}
				if keyword != "" {
					queriesToUse = append(queriesToUse, keyword)
				}
				if len(queriesToUse) == 0 {
					lastQuery := loop.Get("last_keyword_search_query")
					if lastQuery != "" {
						queriesToUse = append(queriesToUse, lastQuery)
					}
				}
			} else {
				if len(searchQueries) > 0 {
					queriesToUse = append(queriesToUse, searchQueries...)
				}
				if searchQuery != "" {
					queriesToUse = append(queriesToUse, searchQuery)
				}
				if len(queriesToUse) == 0 {
					lastQuery := loop.Get("last_semantic_search_query")
					if lastQuery != "" {
						queriesToUse = append(queriesToUse, lastQuery)
					}
				}
			}

			if len(queriesToUse) == 0 {
				op.Feedback("未提供查询，无法执行搜索。请提供单条或多条查询语句。")
				op.Continue()
				return
			}

			// Emit search conditions to default stream so clients can show progress/filters
			emitter := loop.GetEmitter()
			// Announce prepared search conditions
			loop.LoadingStatus(fmt.Sprintf("准备执行搜索 - preparing search: %s", strings.Join(queriesToUse, "; ")))

			var allResults []string
			var successCount int
			loop.LoadingStatus(fmt.Sprintf("查询知识库中 - querying knowledge bases for: %s", strings.Join(queriesToUse, "; ")))

			for _, queryToUse := range queriesToUse {
				queryToUse = strings.TrimSpace(queryToUse)
				if queryToUse == "" {
					continue
				}
				enhanceData, err := invoker.EnhanceKnowledgeGetter(ctx, queryToUse, knowledgeBases...)
				if err != nil {
					log.Warnf("enhance getter error for query '%s': %v", queryToUse, err)
					loop.LoadingStatus(fmt.Sprintf("查询失败 - query failed for: %s", queryToUse))
					emitter.EmitDefaultStreamEvent(
						"search_progress",
						strings.NewReader(fmt.Sprintf("stage:query_failed\nquery:%s\nerror:%v", queryToUse, err)),
						loop.GetCurrentTask().GetIndex(),
						func() {},
					)
					continue
				}
				if enhanceData == "" {
					continue
				}

				loop.LoadingStatus("已获取结果，准备压缩 - result fetched, preparing to compress")

				singleResult := fmt.Sprintf("=== 查询: %s ===\n%s", queryToUse, enhanceData)
				// compressKnowledgeResults expects []string for queries context
				loop.LoadingStatus("压缩搜索结果中 - compressing search result")
				compressedSingle := compressKnowledgeResults(singleResult, []string{queryToUse}, userContext, invoker, op, loop)
				loop.LoadingStatus("压缩完成 - compression done")

				invoker.AddToTimeline("knowledge_fragment_compressed", fmt.Sprintf("Mode: %s\nQuery: %s\n%s", mode, queryToUse, compressedSingle))
				allResults = append(allResults, compressedSingle)
				successCount++

				// 验证满意度
				loop.LoadingStatus("验证用户满意度中 - verifying user satisfaction")
				vr, verr := invoker.VerifyUserSatisfaction(ctx, verifyQuery, false, compressedSingle)
				if verr != nil {
					log.Warnf("verify error for query '%s': %v", queryToUse, verr)
					loop.LoadingStatus("验证失败 - verify error")
				} else {
					invoker.AddToTimeline("verify_user_satisfaction_reasoning", fmt.Sprintf("Mode: %s\nQuery: %s\nReasoning: %s", mode, queryToUse, vr.Reasoning))
					if vr.NextMovements != "" {
						invoker.AddToTimeline("verify_user_next_movements", fmt.Sprintf("Mode: %s\nQuery: %s\nNextMovements: %s", mode, queryToUse, vr.NextMovements))
					}
					loop.PushSatisfactionRecordWithCompletedTaskIndex(vr.Satisfied, vr.Reasoning, vr.CompletedTaskIndex, vr.NextMovements)
					loop.LoadingStatus(fmt.Sprintf("验证完成 - verify done, satisfied=%v", vr.Satisfied))
					// Verification completed
					if vr.Satisfied {
						op.Exit()
						return
					}
				}
			}

			// 汇总并进一步压缩整体结果
			var resultBuilder strings.Builder
			resultBuilder.WriteString("=== 知识库搜索结果 ===\n")
			resultBuilder.WriteString(fmt.Sprintf("模式: %s\n", mode))
			resultBuilder.WriteString(fmt.Sprintf("知识库: %s\n", strings.Join(knowledgeBases, ", ")))
			resultBuilder.WriteString(fmt.Sprintf("查询: %s\n\n", strings.Join(queriesToUse, "; ")))
			if len(allResults) == 0 {
				resultBuilder.WriteString("未找到相关知识条目。\n")
			} else {
				for _, r := range allResults {
					resultBuilder.WriteString(r)
					resultBuilder.WriteString("\n\n")
				}
			}
			searchResults := resultBuilder.String()

			// 再次整体压缩
			if len(allResults) > 2 {
				compressedResult := compressKnowledgeResults(searchResults, queriesToUse, userContext, invoker, op, loop)
				if len(compressedResult) < len(searchResults) {
					searchResults = compressedResult
				}
			}

			// 更新历史与上下文
			searchHistory := loop.Get("search_history")
			if searchHistory != "" {
				searchHistory += "\n---\n"
			}
			searchHistory += fmt.Sprintf("[%s] 模式: %s, 知识库: %s, 查询数: %d", time.Now().Format("15:04:05"), mode, strings.Join(knowledgeBases, ", "), successCount)
			loop.Set("search_history", searchHistory)
			loop.Set("search_results", searchResults)

			invoker.AddToTimeline("knowledge_searched", fmt.Sprintf("Mode: %s, Searched knowledge bases '%v' with queries '%s', successful queries: %d", mode, knowledgeBases, strings.Join(queriesToUse, "; "), successCount))

			// 验证整体满足度
			verifyResult, err := invoker.VerifyUserSatisfaction(ctx, verifyQuery, false, searchResults)
			if err != nil {
				log.Warnf("failed to verify user satisfaction: %v", err)
			} else {
				loop.PushSatisfactionRecordWithCompletedTaskIndex(verifyResult.Satisfied, verifyResult.Reasoning, verifyResult.CompletedTaskIndex, verifyResult.NextMovements)
				if verifyResult.Satisfied {
					op.Exit()
					return
				}
				// 不满足则把 summary 和 next movements 放入 timeline 并继续
				invoker.AddToTimeline("verify_user_summary", verifyResult.Reasoning)
				if verifyResult.NextMovements != "" {
					invoker.AddToTimeline("verify_user_next_movements", verifyResult.NextMovements)
				}
			}

			op.Feedback(searchResults)
			op.Continue()
		},
	)
}

// semantic and keyword action constructors
var searchKnowledgeSemanticAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return makeSearchAction(r, "semantic")
}

var searchKnowledgeKeywordAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return makeSearchAction(r, "keyword")
}

func buildVerifyQueryWithCoreSummary(loop *reactloops.ReActLoop) string {
	userQuery := loop.Get("user_query")
	coreSummary := strings.TrimSpace(loop.Get("knowledge_core_summary"))
	if coreSummary == "" {
		return userQuery
	}
	return fmt.Sprintf("%s\n\n补充要求：验证时需确保提交的payload覆盖以下知识库核心内容。\n%s", userQuery, coreSummary)
}
