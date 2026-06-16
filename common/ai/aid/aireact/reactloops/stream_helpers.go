package reactloops

import (
	"fmt"
	"strings"
)

// EmitStatus 发送瞬时状态（状态栏覆盖显示）
// message 格式必须为双语：中文 / English
// 示例: "查询流量中 / Querying Flows..."
func EmitStatus(loop *ReActLoop, message string) {
	if loop == nil || message == "" {
		return
	}
	loop.LoadingStatus(message)
}

// emitProgress 发送进度状态（带百分比和计数）
// actionZh: 中文动作描述，如 "匹配进度"
// actionEn: 英文动作描述，如 "Matching"
func emitProgress(loop *ReActLoop, current, total int, actionZh, actionEn string) {
	if loop == nil || total <= 0 {
		return
	}

	percent := current * 100 / total
	if percent > 100 {
		percent = 100
	}

	message := fmt.Sprintf("%s %d%% (%d/%d) / %s %d%% (%d/%d)",
		actionZh, percent, current, total,
		actionEn, percent, current, total)

	EmitStatus(loop, message)
}

// EmitActionLog 输出 Action 的累积日志
// nodeId: action 专属的 NodeId (如 "http-flow-query")
// lines: 要输出的行
func EmitActionLog(loop *ReActLoop, nodeId string, lines ...string) {
	if loop == nil || nodeId == "" || len(lines) == 0 {
		return
	}

	emitter := loop.GetEmitter()
	if emitter == nil {
		return
	}

	taskID := ""
	if task := loop.GetCurrentTask(); task != nil {
		taskID = task.GetId()
	}

	for _, line := range lines {
		emitter.EmitDefaultStreamEvent(nodeId, strings.NewReader(line), taskID)
	}
}
