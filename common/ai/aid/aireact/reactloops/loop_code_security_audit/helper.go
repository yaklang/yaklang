package loop_code_security_audit

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

// buildFSAction 获取单个 Yak 文件系统工具并注册为 loop action。
// identifier 字段由框架层 buildSchema 统一注入，无需各 loop 单独处理。
func buildFSAction(r aicommon.AIInvokeRuntime, toolName string) reactloops.ReActLoopOption {
	return buildFSActionWithCallback(r, toolName, nil)
}

// buildFSActionWithCallback 与 buildFSAction 相同，但在工具成功执行后调用 onSuccess 回调。
// onSuccess 接收 AI 传入的 "file" 参数值（即写出的文件路径）。
func buildFSActionWithCallback(r aicommon.AIInvokeRuntime, toolName string, onSuccess func(filePath string)) reactloops.ReActLoopOption {
	toolMgr := r.GetConfig().GetAiToolManager()
	if toolMgr == nil {
		log.Warnf("[CodeAudit] tool manager not available, skip %q action", toolName)
		return func(r *reactloops.ReActLoop) {}
	}
	tool, err := toolMgr.GetToolByName(toolName)
	if err != nil || tool == nil {
		log.Warnf("[CodeAudit] tool %q not found: %v", toolName, err)
		return func(r *reactloops.ReActLoop) {}
	}
	base := reactloops.ConvertAIToolToLoopAction(tool)

	if onSuccess != nil {
		origHandler := base.ActionHandler
		base.ActionHandler = func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			filePath := action.GetString("file")
			origHandler(loop, action, op)
			if filePath != "" {
				onSuccess(filePath)
			}
		}
	}

	return reactloops.WithOverrideLoopAction(base)
}
