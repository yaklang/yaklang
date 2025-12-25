package loop_knowledge_enhance

import (
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// searchKnowledgeAction creates an action to search the knowledge base using EnhanceKnowledgeGetter
var searchKnowledgeAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"search_knowledge",
		"根据用户问题推测相关关键词并搜索知识库，返回相关的知识条目",
		[]aitool.ToolOption{
			aitool.WithStringArrayParam("search_queries", aitool.WithParam_Description("根据用户问题推测的搜索查询语句列表，支持多个查询"), aitool.WithParam_Required(true)),
			aitool.WithStringArrayParam("keywords", aitool.WithParam_Description("从用户问题中提取的关键词列表，用于严格关键词搜索")),
			aitool.WithNumberParam("limit", aitool.WithParam_Description("返回结果的最大数量，默认为5")),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			// 获取搜索参数
			searchQueries := action.GetStringSlice("search_queries")
			if len(searchQueries) == 0 {
				return utils.Error("search_queries is required and must contain at least one query")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			searchQueries := action.GetStringSlice("search_queries")

			// 严格按照关键词搜索
			keywords := action.GetStringSlice("keywords")

			// 获取 invoker 和 context
			invoker := loop.GetInvoker()
			ctx := loop.GetConfig().GetContext()
			task := loop.GetCurrentTask()
			if task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}

			// TODO: 关键词搜索，暂时合并所有搜索查询和关键词
			allQueries := make([]string, 0, len(searchQueries)+len(keywords))
			allQueries = append(allQueries, searchQueries...)
			allQueries = append(allQueries, keywords...)

			// 收集所有搜索结果
			var allResults []string
			var successCount int

			// 依次使用每个查询搜索
			for _, query := range allQueries {
				if query == "" {
					continue
				}

				// 使用 EnhanceKnowledgeGetter 进行知识库查询
				enhanceData, err := invoker.EnhanceKnowledgeGetter(ctx, query)
				if err != nil {
					continue
				}

				if enhanceData != "" {
					allResults = append(allResults, fmt.Sprintf("=== 查询: %s ===\n%s", query, enhanceData))
					successCount++
				}
			}

			// 格式化搜索结果
			var resultBuilder strings.Builder
			resultBuilder.WriteString(fmt.Sprintf("=== 知识库搜索结果 (查询: %s) ===\n\n", strings.Join(searchQueries, ", ")))

			if len(allResults) == 0 {
				resultBuilder.WriteString("未找到相关知识条目。\n")
				resultBuilder.WriteString("建议：\n")
				resultBuilder.WriteString("1. 尝试使用不同的关键词重新搜索\n")
				resultBuilder.WriteString("2. 使用更通用或更具体的搜索词\n")
			} else {
				for _, result := range allResults {
					resultBuilder.WriteString(result)
					resultBuilder.WriteString("\n\n")
				}
			}

			searchResults := resultBuilder.String()

			// 更新搜索历史
			searchHistory := loop.Get("search_history")
			if searchHistory != "" {
				searchHistory += "\n---\n"
			}
			searchHistory += fmt.Sprintf("[%s] 搜索: %s, 成功查询数: %d", time.Now().Format("15:04:05"), strings.Join(searchQueries, ", "), successCount)
			loop.Set("search_history", searchHistory)

			// 更新当前搜索结果
			loop.Set("search_results", searchResults)

			invoker.AddToTimeline("knowledge_searched", fmt.Sprintf("Searched knowledge base with queries '%v', successful queries: %d", searchQueries, successCount))

			op.Feedback(searchResults)
			op.Continue()
		},
	)
}
