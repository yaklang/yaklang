package reactloops

import (
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/log"
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
func EmitProgress(loop *ReActLoop, current, total int, actionZh, actionEn string) {
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

// SaveAndPinFile 保存文件内容并 pin 到前端
// filename: 文件路径
// content: 文件内容
// loop: ReActLoop 实例
// 返回：保存成功返回 nil，失败返回 error
func SaveAndPinFile(loop *ReActLoop, filename string, content []byte) error {
	if loop == nil {
		return fmt.Errorf("loop is nil")
	}
	if filename == "" {
		return fmt.Errorf("filename is empty")
	}

	// 保存文件
	if err := os.WriteFile(filename, content, 0644); err != nil {
		log.Errorf("failed to write file %s: %v", filename, err)
		return fmt.Errorf("failed to write file: %w", err)
	}

	log.Infof("file saved: %s (%d bytes)", filename, len(content))

	// Pin 文件到前端
	emitter := loop.GetEmitter()
	if emitter != nil {
		emitter.EmitPinFilename(filename)
	}

	return nil
}
