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
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	config := NewScanConfig(option...)
	var taskId string
	var m *scanManager
	var err error
	errC := make(chan error)
	switch ControlMode(strings.ToLower(config.GetControlMode())) {
	case ControlModeStart:
		taskId = uuid.New().String()
		m, err = createSyntaxflowTaskById(taskId, ctx, config)
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
		m, err = LoadSyntaxflowTaskFromDB(taskId, ctx)
		if err != nil {
			return err
		}
		m.StatusTask()
		return err
	case ControlModeResume:
		taskId = config.GetResumeTaskId()
		m, err = LoadSyntaxflowTaskFromDB(taskId, ctx)
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

	// wait result
	select {
	case err, ok := <-errC:
		RemoveSyntaxFlowTaskByID(taskId)
		if ok {
			return err
		}
		return nil
	case <-ctx.Done():
		m.Stop()
		RemoveSyntaxFlowTaskByID(taskId)
		m.status = schema.SYNTAXFLOWSCAN_DONE
		m.SaveTask()
		return utils.Error("client canceled")
	}
}
