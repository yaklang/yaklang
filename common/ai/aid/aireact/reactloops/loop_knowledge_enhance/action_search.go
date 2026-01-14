package loop_knowledge_enhance

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

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
				// Use new scoring-based compression (10KB limit per result)
				loop.LoadingStatus("压缩搜索结果中 - compressing search result")
				compressedSingle := compressKnowledgeResultsWithScore(singleResult, userContext, invoker, loop, 10*1024)
				loop.LoadingStatus("压缩完成 - compression done")

				invoker.AddToTimeline("knowledge_fragment_compressed", fmt.Sprintf("Mode: %s\nQuery: %s\n%s", mode, queryToUse, compressedSingle))
				allResults = append(allResults, compressedSingle)
				successCount++

				// Save to artifacts immediately after compression
				iteration := loop.GetCurrentIterationIndex()
				if iteration <= 0 {
					iteration = 1
				}

				// Save compressed result to artifact file
				artifactFilename := invoker.EmitFileArtifactWithExt(
					fmt.Sprintf("knowledge_round_%d_%s", iteration, utils.DatetimePretty2()),
					".md",
					"",
				)
				emitter.EmitPinFilename(artifactFilename)

				artifactContent := fmt.Sprintf(`# 知识查询结果 - 第 %d 轮

查询语句: %s
知识库: %s
模式: %s
查询时间: %s

## 压缩后的关键内容

%s
`, iteration, queryToUse, strings.Join(knowledgeBases, ", "), mode, time.Now().Format("2006-01-02 15:04:05"), compressedSingle)

				if err := os.WriteFile(artifactFilename, []byte(artifactContent), 0644); err != nil {
					log.Warnf("failed to write knowledge artifact: %v", err)
				} else {
					log.Infof("knowledge artifact saved to: %s", artifactFilename)
				}

				// Record in loop context for later aggregation
				loop.Set(fmt.Sprintf("artifact_round_%d_%d", iteration, successCount), artifactFilename)
				loop.Set(fmt.Sprintf("compressed_result_round_%d_%d", iteration, successCount), compressedSingle)

				// 记录查询完成，不再进行验证（避免死循环）
				log.Infof("query '%s' completed, result saved to: %s", queryToUse, artifactFilename)
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

			// Check total size and compress if exceeds 20KB
			const maxContextBytes = 20 * 1024 // 20KB context limit
			if len(searchResults) > maxContextBytes {
				log.Infof("search results too large (%d bytes), compressing to 10KB", len(searchResults))
				searchResults = compressKnowledgeResultsWithScore(searchResults, userContext, invoker, loop, 10*1024)
			} else if len(allResults) > 2 {
				// For multiple results, use legacy compression as a fallback
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

			// 直接输出结果并退出，不再验证用户满意度（避免死循环）
			// 搜索完成后让 AI 自行决定是否需要继续搜索
			log.Infof("knowledge search completed: mode=%s, queries=%d, success=%d, result_size=%d bytes",
				mode, len(queriesToUse), successCount, len(searchResults))

			op.Feedback(searchResults)
			op.Exit()
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
