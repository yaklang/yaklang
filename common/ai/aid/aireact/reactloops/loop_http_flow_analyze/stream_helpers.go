package loop_http_flow_analyze

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils"
)

// emitStatus 发送瞬时状态（状态栏覆盖显示）
// message 格式必须为双语：中文 / English
// 示例: "查询流量中 / Querying Flows..."
func emitStatus(loop *reactloops.ReActLoop, message string) {
	if loop == nil || message == "" {
		return
	}
	loop.LoadingStatus(message)
}

// emitProgress 发送进度状态（带百分比和计数）
// actionZh: 中文动作描述，如 "匹配进度"
// actionEn: 英文动作描述，如 "Matching"
func emitProgress(loop *reactloops.ReActLoop, current, total int, actionZh, actionEn string) {
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

	emitStatus(loop, message)
}

// emitActionLog 输出 Action 的累积日志
// 每个 action 最多输出 2 条，格式简洁
// nodeId: action 专属的 NodeId (如 "http-flow-query-result")
// lines: 要输出的行（1-2行）
func emitActionLog(loop *reactloops.ReActLoop, nodeId string, lines ...string) {
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

	// 合并所有行，每行一个换行符
	var builder strings.Builder
	for _, line := range lines {
		if line != "" {
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}

	content := builder.String()
	if content != "" {
		pr, pw := utils.NewPipe()
		emitter.EmitDefaultStreamEvent(nodeId, pr, taskID)

		go func() {
			defer pw.Close()
			pw.WriteString(content)
		}()
	}
}
