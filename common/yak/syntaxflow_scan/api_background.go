package syntaxflow_scan

import (
	"context"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// StartScanInBackground 创建并启动 SyntaxFlow 扫描，立即返回 task_id，不等待扫描结束。
// 与 StartScan 使用相同的建任务与扫尾逻辑（见 Scan 中 ControlModeStart 分支）。
func StartScanInBackground(ctx context.Context, opts ...ssaconfig.Option) (string, error) {
	opts = append(opts,
		ssaconfig.WithScanControlMode(ssaconfig.ControlModeStart),
		ssaconfig.WithScanConcurrencyDefault(5),
	)
	config, err := NewConfig(opts...)
	if err != nil {
		return "", err
	}
	taskID := uuid.New().String()
	runningID := uuid.NewString()
	m, err := createSyntaxflowTaskById(ctx, runningID, taskID, config)
	if err != nil {
		return "", err
	}
	RemoveSyntaxFlowTaskByID(taskID)
	var success bool
	go func() {
		defer func() {
			if success && m != nil && m.status != schema.SYNTAXFLOWSCAN_PAUSED {
				m.SetFinishedQuery(m.GetTotalQuery())
			}
			if m != nil {
				_ = m.SaveTask()
				m.StatusTask()
				m.Stop(runningID)
				m.saveReport()
			}
		}()
		if err := m.ScanNewTask(); err != nil {
			log.Errorf("StartScanInBackground: %v", err)
			if m != nil {
				m.status = schema.SYNTAXFLOWSCAN_ERROR
			}
			return
		}
		success = true
	}()
	return taskID, nil
}
