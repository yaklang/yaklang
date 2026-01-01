package loop_report_generating

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// searchKnowledgeAction creates an action for searching knowledge bases
var searchKnowledgeAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"search_knowledge",
		"Search knowledge bases for relevant information using semantic search. Use this to gather background information and references for your report.",
		[]aitool.ToolOption{
			aitool.WithStringArrayParam("knowledge_bases", aitool.WithParam_Description("Names of knowledge bases to search"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("search_query", aitool.WithParam_Description("The search query - should be a complete question or statement for semantic matching"), aitool.WithParam_Required(true)),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			kbs := action.GetStringSlice("knowledge_bases")
			if len(kbs) == 0 {
				return utils.Error("knowledge_bases is required and must contain at least one knowledge base name")
			}

			query := action.GetString("search_query")
			if query == "" {
				return utils.Error("search_query is required")
			}

			log.Infof("search_knowledge: verifying search in knowledge bases %v with query: %s", kbs, query)
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			kbs := action.GetStringSlice("knowledge_bases")
			query := action.GetString("search_query")

			log.Infof("search_knowledge: searching knowledge bases %v with query: %s", kbs, query)

			invoker := loop.GetInvoker()
			ctx := loop.GetConfig().GetContext()
			task := loop.GetCurrentTask()
			if task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}

			// 使用 EnhanceKnowledgeGetter 进行知识库搜索
			enhanceData, err := invoker.EnhanceKnowledgeGetter(ctx, query, kbs...)
			if err != nil {
				log.Errorf("search_knowledge: failed to search knowledge base: %v", err)
				op.Feedback(fmt.Sprintf("Knowledge search failed: %v", err))
				op.Continue()
				return
			}

			if enhanceData == "" {
				log.Infof("search_knowledge: no results found for query: %s", query)
				op.Feedback(fmt.Sprintf("No results found for query: %s\nTry different keywords or queries.", query))
				op.Continue()
				return
			}

			log.Infof("search_knowledge: found results, size=%d bytes", len(enhanceData))

			// 构建结果
			var resultBuilder strings.Builder
			resultBuilder.WriteString(fmt.Sprintf("=== Knowledge Search Results ===\n"))
			resultBuilder.WriteString(fmt.Sprintf("Knowledge Bases: %s\n", strings.Join(kbs, ", ")))
			resultBuilder.WriteString(fmt.Sprintf("Query: %s\n\n", query))
			resultBuilder.WriteString(enhanceData)

			resultContent := resultBuilder.String()

			// 限制内容大小
			const maxContentSize = 30 * 1024 // 30KB
			if len(resultContent) > maxContentSize {
				resultContent = resultContent[:maxContentSize] + "\n\n[... results truncated ...]"
				log.Warnf("search_knowledge: results truncated to %d bytes", maxContentSize)
			}

			// 将搜索结果添加到已收集的参考资料中
			existingRefs := loop.Get("collected_references")
			loop.Set("collected_references", existingRefs+"\n"+resultContent)

			// 添加到时间线
			invoker.AddToTimeline("knowledge_searched", fmt.Sprintf("Searched knowledge bases %v with query: %s", kbs, query))

			// 反馈结果
			op.Feedback(resultContent)

			log.Infof("search_knowledge: completed, results added to collected references")
		},
	)
}
