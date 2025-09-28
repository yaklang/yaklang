package syntaxflow_scan

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func Scan(ctx context.Context, option ...ScanOption) error {
	config := NewScanConfig(option...)
	var taskId string
	var m *scanManager

	runningID := uuid.NewString()
	defer func() {
		m.SaveTask()
		m.StatusTask()
		m.Stop(runningID)
	}()
	var err error
	errC := make(chan error)
	switch ControlMode(strings.ToLower(config.GetControlMode())) {
	case ControlModeStart:
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
	case ControlModeStatus:
		taskId = config.ResumeTaskId
		m, err = LoadSyntaxflowTaskFromDB(ctx, runningID, config)
		if err != nil {
			return err
		}
		m.StatusTask()
		close(errC)
	case ControlModeResume:
		taskId = config.ResumeTaskId
		m, err = LoadSyntaxflowTaskFromDB(ctx, runningID, config)
		if err != nil {
			return err
		}
		go func() {
			// err := s.syntaxFlowResumeTask(m, stream)
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
