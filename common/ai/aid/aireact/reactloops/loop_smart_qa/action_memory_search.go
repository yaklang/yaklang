package loop_smart_qa

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

const memoryMaxTokenLimit = 10240

func makeMemorySearchAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	desc := "Search the AI persistent memory system bound to the current session. " +
		"Supports semantic (embedding-based), BM25 (keyword relevance), keyword (tag-based), or combined search. " +
		"Results include timestamps."

	toolOpts := []aitool.ToolOption{
		aitool.WithStringParam("query",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("The search query to find relevant memories.")),
		aitool.WithStringParam("search_mode",
			aitool.WithParam_Description("Search mode: 'semantic', 'bm25', 'keyword', or 'all'. Default: 'all'."),
			aitool.WithParam_Default("all")),
		aitool.WithIntegerParam("limit",
			aitool.WithParam_Description("Maximum number of results. Default: 10."),
			aitool.WithParam_Default(10)),
		aitool.WithIntegerParam("bytes_limit",
			aitool.WithParam_Description("Maximum content size in tokens (max 10240). Default: 4096."),
			aitool.WithParam_Default(4096)),
	}

	return reactloops.WithRegisterLoopAction(
		"search_persistent_memory",
		desc, toolOpts,
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if strings.TrimSpace(action.GetString("query")) == "" {
				return utils.Error("query is required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			query := strings.TrimSpace(action.GetString("query"))
			searchMode := strings.ToLower(strings.TrimSpace(action.GetString("search_mode")))
			if searchMode == "" {
				searchMode = "all"
			}
			limit := int(action.GetInt("limit"))
			if limit <= 0 {
				limit = 10
			}
			tokenLimit := int(action.GetInt("bytes_limit"))
			if tokenLimit <= 0 {
				tokenLimit = 4096
			}
			if tokenLimit > memoryMaxTokenLimit {
				tokenLimit = memoryMaxTokenLimit
			}

			invoker := loop.GetInvoker()
			loop.UserStatus(
				"正在回顾相关信息",
				"Reviewing relevant information",
				aicommon.WithStatusCode("answer.memory.searching"),
				aicommon.WithStatusDetail(fmt.Sprintf("正在回顾与「%s」相关的内容", query), fmt.Sprintf("Reviewing information about %q", query)),
			)

			// Both the ordinary tool and this compatibility action execute
			// the same Yak script; retrieval and formatting are not duplicated.
			switch searchMode {
			case "semantic":
				searchMode = "vector"
			case "keyword":
				searchMode = "bm25"
			case "all":
				searchMode = "hybrid"
			}
			result, _, err := invoker.ExecuteToolRequiredAndCallWithoutRequired(op.GetTask().GetContext(), "search_memory", aitool.InvokeParams{
				"query": query, "search_mode": searchMode, "limit": min(limit, 20), "token_limit": min(max(tokenLimit, 64), 8192),
			}, aicommon.WithToolCaller_Reason("搜索历史记忆"))
			if err != nil {
				op.Feedback(fmt.Sprintf("memory search failed: %v", err))
				op.Continue()
				return
			}
			if result != nil {
				appendMemoryResults(loop, result.String())
			}

			op.Feedback(fmt.Sprintf("memory search completed for: '%s'", query))
			op.Continue()
		},
	)
}

var memorySearchAction = makeMemorySearchAction
