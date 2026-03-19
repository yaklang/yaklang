package loop_code_security_audit

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

// buildFSAction 获取单个 Yak 文件系统工具并注册为 loop action。
func buildFSAction(r aicommon.AIInvokeRuntime, toolName string) reactloops.ReActLoopOption {
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
	return reactloops.WithRegisterLoopActionFromTool(tool)
}
