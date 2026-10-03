package coordinator_test

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"strings"
)

// The scripted provider authors decisions only. Report creation, gates, file
// storage and the host's final transition remain real runtime behavior.
func reportResponse(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest, native, exists bool) (*aicommon.AIResponse, error) {
	if !strings.Contains(req.GetPrompt(), "当前任务图和关键消息已经收尾") {
		return protocolResponse(cfg, req, native, "directly_answer", map[string]any{"answer_payload": "核对本批次新增事实与任务结果，随后编写报告。"})
	}
	if !exists {
		return protocolResponse(cfg, req, native, "create_report", map[string]any{"title": "执行检查报告", "document": "# 执行检查报告\n所有必需任务已经审核，实际证据与结果保存在共享 Timeline 中。"})
	}
	return protocolResponse(cfg, req, native, "submit_report", map[string]any{"summary": "任务已审核，交付最新报告。"})
}
