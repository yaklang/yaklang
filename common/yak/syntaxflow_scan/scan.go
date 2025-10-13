package syntaxflow_scan

import (
	"context"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func Scan(ctx context.Context, option ...ssaconfig.Option) error {
	config, err := NewConfig(option...)
	if err != nil {
		return err
	}
	var taskId string
	var m *scanManager

	runningID := uuid.NewString()
	defer func() {
		m.SaveTask()
		m.StatusTask()
		m.Stop(runningID)
	}()
	errC := make(chan error)
	switch ssaconfig.ControlMode(config.GetScanControlMode()) {
	case ssaconfig.ControlModeStart:
		taskId = uuid.New().String()
		m, err = createSyntaxflowTaskById(ctx, runningID, taskId, config)
		if err != nil {
			return err
		}
		log.Info("start to create syntaxflow scan")
		go func() {
			err := m.ScanNewTask()
			if err != nil {
				utils.TryWriteChannel(errC, err)
			}
			close(errC)
		}()
	case ssaconfig.ControlModeStatus:
		taskId = config.GetScanResumeTaskId()
		m, err = LoadSyntaxflowTaskFromDB(ctx, runningID, config)
		if err != nil {
			return err
		}
		m.StatusTask()
		close(errC)
	case ssaconfig.ControlModeResume:
		taskId = config.GetScanResumeTaskId()
		m, err = LoadSyntaxflowTaskFromDB(ctx, runningID, config)
		if err != nil {
			return err
		}
		go func() {
			err := m.ResumeTask()
			if err != nil {
				utils.TryWriteChannel(errC, err)
			}
			close(errC)
		}()
	default:
		return utils.Error("invalid syntaxFlow scan mode")
	}
	RemoveSyntaxFlowTaskByID(taskId)

	// wait result
	select {
	case err, ok := <-errC:
		if ok {
			m.status = schema.SYNTAXFLOWSCAN_ERROR
			return err
		}
		return nil
	case <-ctx.Done():
		m.status = schema.SYNTAXFLOWSCAN_DONE
		return utils.Error("client canceled")
	}
}
