package loopinfra

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// RegisterBuiltinFSToolLoopAction registers a builtin filesystem tool as a loop action
// via ConvertAIToolToLoopAction, with explore/audit-style feedback after execution.
func RegisterBuiltinFSToolLoopAction(
	r aicommon.AIInvokeRuntime,
	ownerTag string,
	toolName string,
	onDone func(action *aicommon.Action),
) reactloops.ReActLoopOption {
	toolMgr := r.GetConfig().GetAiToolManager()
	if toolMgr == nil {
		log.Warnf("[%s] tool manager not available, skip %q action", ownerTag, toolName)
		return func(r *reactloops.ReActLoop) {}
	}
	tool, err := toolMgr.GetToolByName(toolName)
	if err != nil || tool == nil {
		log.Warnf("[%s] tool %q not found: %v", ownerTag, toolName, err)
		return func(r *reactloops.ReActLoop) {}
	}

	return reactloops.WithRegisterLoopActionFromToolCustomized(tool, func(action *reactloops.LoopAction) {
		action.ActionHandler = builtinFSToolFeedbackHandler(ownerTag, tool, onDone)
	})
}

func builtinFSToolFeedbackHandler(
	ownerTag string,
	tool *aitool.Tool,
	onDone func(action *aicommon.Action),
) reactloops.LoopActionHandlerFunc {
	toolName := tool.GetName()
	return func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		invoker := loop.GetInvoker()
		ctx := loop.GetConfig().GetContext()
		if task := loop.GetCurrentTask(); task != nil && !utils.IsNil(task.GetContext()) {
			ctx = task.GetContext()
		}

		params := reactloops.BuildLoopActionToolInvokeParams(action, tool)
		if allow, guardMsg := reactloops.CheckToolInvokeGuard(loop, toolName, params); !allow {
			op.Feedback(guardMsg)
			op.Continue()
			return
		}
		params = reactloops.ApplyToolInvokeParamsMutators(loop, toolName, params)

		result, _, execErr := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, toolName, params)
		if execErr != nil {
			log.Warnf("[%s] tool %q failed: %v", ownerTag, toolName, execErr)
			op.Feedback(fmt.Sprintf("[工具执行失败] %s: %v，请尝试其他方法。", toolName, execErr))
			op.Continue()
			return
		}

		resultContent := ""
		if result != nil {
			resultContent = utils.InterfaceToString(result.Data)
		}

		if toolName == "grep" && isGrepEmptyResult(resultContent) {
			searchPath := action.GetString("path")
			if searchPath == "" {
				searchPath = action.GetParams().GetString("path")
			}
			pattern := action.GetString("pattern")
			if pattern == "" {
				pattern = action.GetParams().GetString("pattern")
			}
			hint := fmt.Sprintf(
				"[grep 结果为空] 在路径 %q 中未找到模式 %q 的匹配。可能原因：\n"+
					"  1. 搜索范围太大导致超时（仓库根目录不适合直接 grep）\n"+
					"  2. 文件扩展名未过滤，扫描了大量无关文件\n"+
					"建议措施：\n"+
					"  - 用 find_file 先定位包含入口的文件，再对具体文件/小目录 grep\n"+
					"  - 添加 include-ext 参数（如 include-ext=\".go\" 或 \".java\"）\n"+
					"  - 缩小 path 范围（如改为 %q/cmd 或 %q/src）\n"+
					"  - 检查 pattern 是否正确（如 Go 应用 \"func main()\" 而不是 \"main()\"）",
				searchPath, pattern, searchPath, searchPath,
			)
			log.Infof("[%s] grep returned empty results for path=%q pattern=%q", ownerTag, searchPath, pattern)
			invoker.AddToTimeline("[GREP_EMPTY_RESULT]", hint)
			op.Feedback(hint)
			op.Continue()
			return
		}

		summary, _ := reactloops.SpillLongContent(loop, toolName, resultContent)
		invoker.AddToTimeline(fmt.Sprintf("[%s]", toolName),
			fmt.Sprintf("%s 完成: %d bytes\n%s", toolName, len(resultContent), summary))
		op.Feedback(fmt.Sprintf("[%s 完成] %d bytes\n%s", toolName, len(resultContent), summary))
		op.Continue()
		if onDone != nil {
			onDone(action)
		}
	}
}

func isGrepEmptyResult(content string) bool {
	if content == "" {
		return true
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return isGrepStdoutEmpty(content)
	}
	stdout, _ := parsed["stdout"].(string)
	return isGrepStdoutEmpty(stdout)
}

func isGrepStdoutEmpty(stdout string) bool {
	if stdout == "" {
		return true
	}
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "[info]") &&
			!strings.HasPrefix(line, "[warn]") &&
			!strings.HasPrefix(line, "[error]") &&
			!strings.HasPrefix(line, "[debug]") {
			return false
		}
	}
	return true
}
