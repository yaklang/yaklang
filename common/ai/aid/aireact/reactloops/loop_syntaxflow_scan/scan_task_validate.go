package loop_syntaxflow_scan

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

// ErrSyntaxFlowScanTaskNotFound 附着 task_id 在 SSA 工程库中无对应任务行时返回（errors.Is 可判断）。
var ErrSyntaxFlowScanTaskNotFound = errors.New("syntaxflow scan task not found in SSA DB")

// EnsureSyntaxFlowScanTaskExists 校验 `syntaxflow_scan_task` 中是否存在该 task_id（SSA runtime id）。
// 用于 **attach** 路径在编排层尽早失败，避免进入 phase 后才读库才报错。
func EnsureSyntaxFlowScanTaskExists(db *gorm.DB, taskID string) error {
	if db == nil {
		return fmt.Errorf("SSA 工程库未连接，无法校验 task_id")
	}
	tid := strings.TrimSpace(taskID)
	if tid == "" {
		return fmt.Errorf("task_id 为空，无法执行附着")
	}
	st, err := schema.GetSyntaxFlowScanTaskById(db, tid)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: task_id=%q 在库中无 SyntaxFlow 扫描任务行（请确认已落库或 id 非粘贴错误）: %v",
				ErrSyntaxFlowScanTaskNotFound, tid, err)
		}
		return fmt.Errorf("无法读取扫描任务行 task_id=%q: %w", tid, err)
	}
	if st == nil {
		return fmt.Errorf("%w: task_id=%q", ErrSyntaxFlowScanTaskNotFound, tid)
	}
	return nil
}
