package loop_plan

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func appendWebResults(loop *reactloops.ReActLoop, content string) {
	old := loop.Get(PLAN_WEB_RESULTS_KEY)
	if old == "" {
		loop.Set(PLAN_WEB_RESULTS_KEY, content)
	} else {
		loop.Set(PLAN_WEB_RESULTS_KEY, old+"\n\n"+content)
	}
}

var webSearchAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"web_search",
		"Search the internet for real-time information, best practices, technical documentation, or industry knowledge to support better planning.",
		[]aitool.ToolOption{
			aitool.WithStringParam("query",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Search query for internet search.")),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			invoker := loop.GetInvoker()
			ctx := loop.GetConfig().GetContext()
			task := loop.GetCurrentTask()
			if task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}

			query := action.GetString("query")
			if query == "" {
				query = action.GetString("search_query")
			}
			loop.LoadingStatus(fmt.Sprintf("searching internet: %s", query))

			params := aitool.InvokeParams{"query": query}
			result, _, err := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, "web_search", params)
			if err != nil {
				log.Warnf("web_search tool call failed: %v", err)
				failMsg := fmt.Sprintf(
					"Web search FAILED for '%s': %v. "+
						"You may try web_search again with a different query, "+
						"or use search_knowledge as an alternative.", query, err)
				invoker.AddToTimeline("web_search_failed", failMsg)
				op.Feedback(failMsg)
				op.Continue()
				return
			}

			content := ""
			if result != nil {
				content = utils.InterfaceToString(result.Data)
			}

			if content == "" {
				emptyMsg := fmt.Sprintf(
					"Web search for '%s': returned NO content. "+
						"Consider rephrasing the query or using search_knowledge.", query)
				invoker.AddToTimeline("web_search_empty", emptyMsg)
				op.Feedback(emptyMsg)
				op.Continue()
				return
			}

			entry := fmt.Sprintf("=== Web Search: %s ===\n%s", query, utils.ShrinkString(content, 4096))
			appendWebResults(loop, entry)

			invoker.AddToTimeline("web_search_result",
				fmt.Sprintf("Web search: %s\n\n%s", query, utils.ShrinkString(content, 2048)))

			op.Feedback(fmt.Sprintf("web search completed for: '%s'", query))
			op.Continue()
		},
	)
}
