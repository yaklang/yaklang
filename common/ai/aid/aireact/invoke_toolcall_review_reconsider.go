package aireact

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// The task is captured at admission. Never consult the mutable current-task
// pointer when a scalar or batch review returns.
func (r *ReAct) reconsiderToolReviewForTask(ctx context.Context, task aicommon.AIStatefulTask, tool *aitool.Tool, params, feedback aitool.InvokeParams) string {
	loop := toolBatchLoop(task)
	if loop == nil {
		loop = reactloops.NewMinimalReActLoop(r.config, r)
	}
	names := []string{tool.Name}
	if strings.EqualFold(strings.TrimSpace(feedback.GetString("suggestion")), "wrong_tool") {
		names = append(names, utils.PrettifyListFromStringSplited(feedback.GetString("suggestion_tool"), ",")...)
		if keyword := strings.TrimSpace(feedback.GetString("suggestion_tool_keyword")); keyword != "" {
			// Review must not invoke the AI-backed searcher. Let the owning loop
			// choose from deterministic matches among already enabled tools.
			if tools, err := loop.GetConfig().GetAiToolManager().GetEnableTools(); err == nil {
				var matches []string
				for _, candidate := range tools {
					if candidate != nil && strings.Contains(strings.ToLower(strings.Join([]string{candidate.Name, candidate.Description, candidate.VerboseName, candidate.VerboseNameZh, strings.Join(candidate.Keywords, " ")}, " ")), strings.ToLower(keyword)) {
						matches = append(matches, candidate.Name)
					}
				}
				sort.Strings(matches)
				names = append(names, matches...)
			}
		}
	}
	if len(names) > aicommon.DefaultToolBatchMaxCalls {
		names = names[:aicommon.DefaultToolBatchMaxCalls]
	}
	loaded := loop.LoadToolSchemas(ctx, names)
	for _, entry := range loaded {
		if entry["status"] == "schema_loaded" {
			entry["detail"] = "Schema 已加载，可按任务需要复用，未执行工具。"
		}
	}
	owner := ""
	if task != nil {
		owner = task.GetIndex()
	}
	hint := fmt.Sprintf("用户拒绝工具 %q 的本次提案，未执行工具。返回所属任务 %q 根据反馈重新决策；拒绝提案不代表停止任务。\n用户反馈：\n%s\n原提案参数：\n%s\nSchema 加载：\n%v\n若仍需工具，依据 CACHE_TOOL_CALL 构造完整参数并 directly_call_tool；若信息不足则澄清，若已可回答则回答。不要请求辅助模型修参。", tool.Name, owner, feedback.Dump(), params.Dump(), loaded)
	loop.GetInvoker().AddToTimeline("tool_review_reconsider", hint)
	return hint
}
