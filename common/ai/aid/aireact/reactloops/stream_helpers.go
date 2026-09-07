package reactloops

import (
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
)

// EmitStatus 发送瞬时状态（状态栏覆盖显示）。历史双语字符串会拆分为
// value（默认中文）与 value_i18n。
// Deprecated: 仅供外部旧调用兼容；生产代码应使用 EmitStatusI18n。
func EmitStatus(loop *ReActLoop, message string) {
	if loop == nil || message == "" {
		return
	}
	zh, en := aicommon.SplitLegacyStatusI18n(message)
	loop.UserStatus(zh, en)
}

// EmitStatusI18n emits a product-facing status with structured metadata while
// retaining a Chinese string value for legacy clients.
func EmitStatusI18n(loop *ReActLoop, zh, en string, options ...aicommon.StatusOption) {
	if loop == nil || (zh == "" && en == "") {
		return
	}
	loop.UserStatus(zh, en, options...)
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

	EmitStatusI18n(
		loop,
		fmt.Sprintf("%s %d%%（%d/%d）", actionZh, percent, current, total),
		fmt.Sprintf("%s %d%% (%d/%d)", actionEn, percent, current, total),
		aicommon.WithStatusCode("progress"),
		aicommon.WithStatusProgress(int64(current), int64(total), "item"),
	)
}

// EmitActionLog 输出 Action 的累积日志
// nodeId: action 专属的 NodeId (如 "http-flow-query")
func EmitActionLog(loop *ReActLoop, nodeId string, lines string, reference ...string) {
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

	streamEvent, err := emitter.EmitDefaultStreamEvent(nodeId, strings.NewReader(lines), taskID)
	if err != nil {
		log.Warnf("EmitActionLog: failed to emit stream event for nodeId %s: %v", nodeId, err)
		return
	}

	if len(reference) > 0 {
		streamId := streamEvent.GetStreamEventWriterId()
		for _, ref := range reference {
			emitter.EmitTextReferenceMaterial(streamId, ref)
		}
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
