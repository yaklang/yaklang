package loop_knowledge_enhance

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// makeSearchAction builds a search action for the given mode ("semantic" or "keyword")
// 每次只搜索一个条件，搜索后评估 next_movements 决定是否继续
func makeSearchAction(r aicommon.AIInvokeRuntime, mode string) reactloops.ReActLoopOption {
	desc := "根据用户问题推测相关关键词并搜索指定的知识库，返回相关的知识条目"
	if mode == "keyword" {
		desc = "关键字搜索模式：为语义不擅长的结构化条目（如法条、章节）生成关键字并搜索知识库。每次只搜索一个关键词。"
	} else if mode == "semantic" {
		desc = "语义搜索模式：问题驱动的语义检索，优先返回高相关性知识片段。每次只搜索一个查询语句。"
	}

	toolOpts := []aitool.ToolOption{
		aitool.WithStringArrayParam("knowledge_bases", aitool.WithParam_Description("要搜索的知识库名称列表，必须指定至少一个知识库"), aitool.WithParam_Required(true)),
	}

	if mode == "keyword" {
		toolOpts = append(toolOpts, aitool.WithStringParam("keyword", aitool.WithParam_Description("用于关键字搜索的单条关键词或短语"), aitool.WithParam_Required(true)))
	} else {
		toolOpts = append(toolOpts, aitool.WithStringParam("search_query", aitool.WithParam_Description("用于语义搜索的单条查询语句（完整句子）"), aitool.WithParam_Required(true)))
	}

	return reactloops.WithRegisterLoopAction(
		fmt.Sprintf("search_knowledge_%s", mode),
		desc, toolOpts,
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			loop.LoadingStatus(fmt.Sprintf("验证参数中 - search_knowledge:%s / validating parameters - mode:%s", mode, mode))
			knowledgeBases := action.GetStringSlice("knowledge_bases")
			if len(knowledgeBases) == 0 {
				return utils.Error("knowledge_bases is required and must contain at least one knowledge base name")
			}

			if mode == "keyword" {
				keyword := action.GetString("keyword")
				if keyword == "" {
					return utils.Error("keyword is required for keyword search mode")
				}
			} else {
				searchQuery := action.GetString("search_query")
				if searchQuery == "" {
					return utils.Error("search_query is required for semantic search mode")
				}
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			loop.LoadingStatus(fmt.Sprintf("执行搜索中 - search_knowledge:%s / executing search - mode:%s", mode, mode))

			knowledgeBases := action.GetStringSlice("knowledge_bases")
			searchQuery := action.GetString("search_query")
			keyword := action.GetString("keyword")

			invoker := loop.GetInvoker()
			ctx := loop.GetConfig().GetContext()
			task := loop.GetCurrentTask()
			if task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}

			userQuery := loop.Get("user_query")
			userContext := fmt.Sprintf("用户需求：%s", userQuery)

			// 确定本次搜索的查询条件（只搜索一个）
			var queryToUse string
			if mode == "keyword" {
				queryToUse = strings.TrimSpace(keyword)
			} else {
				queryToUse = strings.TrimSpace(searchQuery)
			}

			if queryToUse == "" {
				op.Feedback("未提供查询条件，无法执行搜索。")
				op.Continue()
				return
			}

			emitter := loop.GetEmitter()
			loop.LoadingStatus(fmt.Sprintf("查询知识库中 - querying: %s", queryToUse))

			// 执行搜索
			enhanceData, err := invoker.EnhanceKnowledgeGetter(ctx, queryToUse, knowledgeBases...)
			if err != nil {
				log.Warnf("enhance getter error for query '%s': %v", queryToUse, err)
				op.Feedback(fmt.Sprintf("搜索失败：%v\n请尝试其他查询条件。", err))
				op.Continue()
				return
			}

			if enhanceData == "" {
				op.Feedback(fmt.Sprintf("搜索 '%s' 未找到相关结果。请尝试其他查询条件。", queryToUse))
				op.Continue()
				return
			}

			loop.LoadingStatus("已获取结果，准备压缩 - result fetched, preparing to compress")

			// 压缩搜索结果
			singleResult := fmt.Sprintf("=== 查询: %s ===\n%s", queryToUse, enhanceData)
			loop.LoadingStatus("压缩搜索结果中 - compressing search result")
			compressedResult := compressKnowledgeResultsWithScore(singleResult, userContext, invoker, loop, 10*1024)
			loop.LoadingStatus("压缩完成 - compression done")

			// 记录到 timeline
			invoker.AddToTimeline("knowledge_fragment_compressed", fmt.Sprintf("Mode: %s\nQuery: %s\n%s", mode, queryToUse, compressedResult))

			// 获取迭代次数
			iteration := loop.GetCurrentIterationIndex()
			if iteration <= 0 {
				iteration = 1
			}

			// 获取当前搜索计数
			searchCountStr := loop.Get("search_count")
			searchCount := 1
			if searchCountStr != "" {
				if c, err := strconv.Atoi(searchCountStr); err == nil {
					searchCount = c + 1
				}
			}
			loop.Set("search_count", fmt.Sprintf("%d", searchCount))

			// 保存压缩结果到 artifact
			artifactFilename := invoker.EmitFileArtifactWithExt(
				fmt.Sprintf("knowledge_round_%d_search_%d_%s", iteration, searchCount, utils.DatetimePretty2()),
				".md",
				"",
			)
			emitter.EmitPinFilename(artifactFilename)

			artifactContent := fmt.Sprintf(`# 知识查询结果 - 第 %d 轮搜索 #%d

查询语句: %s
知识库: %s
模式: %s
查询时间: %s

## 压缩后的关键内容

%s
`, iteration, searchCount, queryToUse, strings.Join(knowledgeBases, ", "), mode, time.Now().Format("2006-01-02 15:04:05"), compressedResult)

			if err := os.WriteFile(artifactFilename, []byte(artifactContent), 0644); err != nil {
				log.Warnf("failed to write knowledge artifact: %v", err)
			} else {
				log.Infof("knowledge artifact saved to: %s", artifactFilename)
			}

			// 记录到 loop 上下文
			loop.Set(fmt.Sprintf("artifact_round_%d_%d", iteration, searchCount), artifactFilename)
			loop.Set(fmt.Sprintf("compressed_result_round_%d_%d", iteration, searchCount), compressedResult)

			// 更新搜索历史
			searchHistory := loop.Get("search_history")
			if searchHistory != "" {
				searchHistory += "\n"
			}
			searchHistory += fmt.Sprintf("[%s] #%d %s: %s -> %d bytes",
				time.Now().Format("15:04:05"), searchCount, mode, queryToUse, len(compressedResult))
			loop.Set("search_history", searchHistory)

			// 累积所有压缩结果
			allResults := loop.Get("all_compressed_results")
			if allResults != "" {
				allResults += "\n\n---\n\n"
			}
			allResults += fmt.Sprintf("### 搜索 #%d: %s\n\n%s", searchCount, queryToUse, compressedResult)
			loop.Set("all_compressed_results", allResults)

			// 使用 LiteForge 评估下一步行动
			loop.LoadingStatus("评估搜索结果与下一步计划 - evaluating next movements")

			evalResult := evaluateNextMovements(ctx, invoker, loop, userQuery, queryToUse, compressedResult, searchCount)

			if evalResult.Finished {
				// 知识收集已完成，保存总结并退出循环
				log.Infof("knowledge collection finished, summary: %s", evalResult.Summary)

				// 保存总结到 loop 上下文
				loop.Set("final_summary", evalResult.Summary)

				feedback := fmt.Sprintf(`=== 搜索完成 ===
查询: %s
模式: %s
结果大小: %d bytes
已保存到: %s

=== 知识收集完成 ===
%s

正在生成最终报告...
`, queryToUse, mode, len(compressedResult), artifactFilename, evalResult.Summary)

				op.Feedback(feedback)
				r.AddToTimeline("knowledge_collection_finished", feedback)
				op.Exit() // 主动结束循环，触发 OnFinished 回调生成报告
			} else {
				// 记录 next_movements
				currentNextMovements := loop.Get("next_movements_summary")
				if currentNextMovements != "" {
					currentNextMovements += "\n\n"
				}
				currentNextMovements += fmt.Sprintf("【搜索 #%d: %s】\n%s", searchCount, queryToUse, evalResult.NextMovements)
				loop.Set("next_movements_summary", currentNextMovements)

				invoker.AddToTimeline("next_movements", fmt.Sprintf("Search #%d: %s\nNext: %s", searchCount, queryToUse, evalResult.NextMovements))

				// 构建反馈信息
				feedback := fmt.Sprintf(`=== 搜索完成 ===
查询: %s
模式: %s
结果大小: %d bytes
已保存到: %s

=== 下一步建议 ===
%s

请根据建议继续搜索。
`, queryToUse, mode, len(compressedResult), artifactFilename, evalResult.NextMovements)

				op.Feedback(feedback)
				r.AddToTimeline("next_movements_feedback", feedback)
				op.Continue() // 继续循环，让 AI 执行下一步搜索
			}
		},
	)
}

// EvaluateResult 评估结果结构体
type EvaluateResult struct {
	NextMovements string // 下一步搜索建议
	Finished      bool   // 是否已完成知识收集
	Summary       string // 当 finished 时的总结
}

// evaluateNextMovements 使用 LiteForge 评估下一步搜索需要补充什么内容
func evaluateNextMovements(
	ctxAny any,
	invoker aicommon.AIInvokeRuntime,
	loop *reactloops.ReActLoop,
	userQuery string,
	currentQuery string,
	currentResult string,
	searchCount int,
) EvaluateResult {
	// 转换 context
	ctx, ok := ctxAny.(context.Context)
	if !ok {
		log.Warnf("evaluateNextMovements: context conversion failed")
		ctx = context.Background()
	}
	dNonce := utils.RandStringBytes(4)

	// 获取搜索历史
	searchHistory := loop.Get("search_history")

	// 如果搜索次数已达上限，直接返回 finished
	if searchCount >= 5 {
		log.Infof("evaluateNextMovements: search count reached limit (%d), stopping", searchCount)
		return EvaluateResult{
			NextMovements: "",
			Finished:      true,
			Summary:       "已达到最大搜索次数限制",
		}
	}

	promptTemplate := `<|USER_QUERY_{{ .nonce }}|>
{{ .userQuery }}
<|USER_QUERY_END_{{ .nonce }}|>

<|SEARCH_HISTORY_{{ .nonce }}|>
{{ .searchHistory }}
<|SEARCH_HISTORY_END_{{ .nonce }}|>

<|CURRENT_SEARCH_{{ .nonce }}|>
查询条件: {{ .currentQuery }}
搜索次数: 第 {{ .searchCount }} 次

搜索结果摘要:
{{ .currentResult }}
<|CURRENT_SEARCH_END_{{ .nonce }}|>

<|INSTRUCT_{{ .nonce }}|>
【评估知识收集进度】

请评估当前搜索结果是否足够回答用户问题，并给出下一步建议：

【评估标准】
1. 当前结果是否直接回答了用户的核心问题？
2. 是否还有重要的知识维度未被覆盖？
3. 是否需要从其他角度补充信息？

【输出要求】
- finished: 布尔值，如果信息已足够则为 true，否则为 false
- next_movements: 如果 finished 为 false，输出具体的搜索建议（用什么关键词/查询语句）；如果 finished 为 true，输出空字符串
- summary: 如果 finished 为 true，简要总结已收集的知识；如果 finished 为 false，输出空字符串

【限制】
- 搜索次数不应超过 5 次
- 避免重复相同或相似的搜索
- 优先考虑用户问题中未被覆盖的方面

请输出 finished、next_movements 和 summary。
<|INSTRUCT_END_{{ .nonce }}|>
`

	// 限制结果长度
	resultPreview := currentResult
	if len(resultPreview) > 2000 {
		resultPreview = resultPreview[:2000] + "\n...(已截断)"
	}

	materials, err := utils.RenderTemplate(promptTemplate, map[string]any{
		"nonce":         dNonce,
		"userQuery":     userQuery,
		"searchHistory": searchHistory,
		"currentQuery":  currentQuery,
		"searchCount":   searchCount,
		"currentResult": resultPreview,
	})

	if err != nil {
		log.Errorf("evaluateNextMovements: template render failed: %v", err)
		return EvaluateResult{Finished: true, Summary: "template render failed"}
	}

	forgeResult, err := invoker.InvokeLiteForge(
		ctx,
		"evaluate-next-movements",
		materials,
		[]aitool.ToolOption{
			aitool.WithBoolParam("finished", aitool.WithParam_Description("是否已完成知识收集，true 表示信息已足够，false 表示需要继续搜索"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("next_movements", aitool.WithParam_Description("下一步搜索建议，如果 finished 为 true 则为空字符串")),
			aitool.WithStringParam("summary", aitool.WithParam_Description("当 finished 为 true 时，简要总结已收集的知识")),
		},
	)

	if err != nil {
		log.Errorf("evaluateNextMovements: LiteForge failed: %v", err)
		return EvaluateResult{Finished: true, Summary: "LiteForge evaluation failed"}
	}

	if forgeResult == nil {
		return EvaluateResult{Finished: true, Summary: "LiteForge returned nil"}
	}

	finished := forgeResult.GetBool("finished")
	nextMovements := strings.TrimSpace(forgeResult.GetString("next_movements"))
	summary := strings.TrimSpace(forgeResult.GetString("summary"))

	return EvaluateResult{
		NextMovements: nextMovements,
		Finished:      finished,
		Summary:       summary,
	}
}

// semantic and keyword action constructors
var searchKnowledgeSemanticAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return makeSearchAction(r, "semantic")
}

var searchKnowledgeKeywordAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return makeSearchAction(r, "keyword")
}
