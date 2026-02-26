package loop_smart_qa

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func makeKnowledgeSearchAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	desc := "Search attached knowledge bases using semantic or keyword queries via the invoker's knowledge enhancement infrastructure."

	toolOpts := []aitool.ToolOption{
		aitool.WithStringParam("search_query",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("The query to search for in knowledge bases.")),
		aitool.WithStringArrayParam("knowledge_bases",
			aitool.WithParam_Description("Names of knowledge bases to search. Searches all if empty.")),
		aitool.WithStringParam("mode",
			aitool.WithParam_Description("Search mode: 'semantic' or 'keyword'. Default: 'semantic'."),
			aitool.WithParam_Default("semantic")),
	}

	return reactloops.WithRegisterLoopAction(
		"search_knowledge",
		desc, toolOpts,
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if strings.TrimSpace(action.GetString("search_query")) == "" {
				return utils.Error("search_query is required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			searchQuery := strings.TrimSpace(action.GetString("search_query"))
			mode := strings.ToLower(strings.TrimSpace(action.GetString("mode")))
			if mode == "" {
				mode = "semantic"
			}

			knowledgeBases := action.GetStringSlice("knowledge_bases")
			if len(knowledgeBases) == 0 {
				loadedKBs := loop.Get("knowledge_bases")
				if loadedKBs != "" {
					knowledgeBases = strings.Split(loadedKBs, ",")
				}
			}

			invoker := loop.GetInvoker()

			if len(knowledgeBases) == 0 {
				insufficientMsg := fmt.Sprintf(
					"Knowledge search for '%s': NO knowledge bases available. "+
						"Local knowledge cannot answer this query. "+
						"You MUST use the web_search action to find the answer from the internet. "+
						"Do NOT retry search_knowledge.", searchQuery)
				invoker.AddToTimeline("knowledge_search_insufficient", insufficientMsg)
				op.Feedback(insufficientMsg)
				op.Continue()
				return
			}

			ctx := loop.GetConfig().GetContext()
			task := loop.GetCurrentTask()
			if task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}

			loop.LoadingStatus(fmt.Sprintf("searching knowledge base: %s", searchQuery))

			var enhancePlans []string
			if mode == "keyword" {
				enhancePlans = []string{"exact_keyword_search"}
			} else {
				enhancePlans = []string{"hypothetical_answer", "generalize_query", "split_query"}
			}

			enhanceData, err := invoker.EnhanceKnowledgeGetterEx(ctx, searchQuery, enhancePlans, knowledgeBases...)
			if err != nil {
				log.Warnf("knowledge search failed for query '%s': %v", searchQuery, err)
				insufficientMsg := fmt.Sprintf(
					"Knowledge search FAILED for '%s': %v. "+
						"The knowledge base could not provide results. "+
						"You MUST use the web_search action to search the internet instead. "+
						"Do NOT retry search_knowledge for the same query.", searchQuery, err)
				invoker.AddToTimeline("knowledge_search_insufficient", insufficientMsg)
				op.Feedback(insufficientMsg)
				op.Continue()
				return
			}
			if enhanceData == "" {
				insufficientMsg := fmt.Sprintf(
					"Knowledge search for '%s': NO RESULTS found in knowledge base. "+
						"The local knowledge base does not contain information about this topic. "+
						"You MUST use the web_search action to find the answer from the internet. "+
						"Do NOT retry search_knowledge for the same query.", searchQuery)
				invoker.AddToTimeline("knowledge_search_insufficient", insufficientMsg)
				op.Feedback(insufficientMsg)
				op.Continue()
				return
			}

			compressed, err := invoker.CompressLongTextWithDestination(ctx, enhanceData, searchQuery, 8*1024)
			if err != nil {
				log.Warnf("compress knowledge results failed: %v", err)
				compressed = enhanceData
			}
			if compressed == "" {
				insufficientMsg := fmt.Sprintf(
					"Knowledge search for '%s': returned empty after compression. "+
						"The knowledge base does not have relevant content for this topic. "+
						"You MUST use the web_search action to find the answer from the internet.", searchQuery)
				invoker.AddToTimeline("knowledge_search_insufficient", insufficientMsg)
				op.Feedback(insufficientMsg)
				op.Continue()
				return
			}

			appendSearchResults(loop, compressed)
			appendSearchHistory(loop, fmt.Sprintf("[knowledge:%s] %s -> %d bytes", mode, searchQuery, len(compressed)))

			invoker.AddToTimeline("knowledge_search_result",
				fmt.Sprintf("Knowledge search (%s): %s\n\n%s", mode, searchQuery, utils.ShrinkString(compressed, 2048)))

			op.Feedback(fmt.Sprintf("knowledge search completed: %d bytes for '%s'", len(compressed), searchQuery))
			op.Continue()
		},
	)
}

var knowledgeSearchAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return makeKnowledgeSearchAction(r)
}
