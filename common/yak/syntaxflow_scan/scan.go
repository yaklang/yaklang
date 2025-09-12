package syntaxflow_scan

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type ScanTaskConfig struct {
	*ypb.SyntaxFlowScanRequest
	RuleNames []string `json:"rule_names"`
}

type ScanStream interface {
	Recv() (*ypb.SyntaxFlowScanRequest, error)
	Send(*ypb.SyntaxFlowScanResponse) error
	Context() context.Context
}

func Scan(stream ScanStream) error {
	return ScanWithConfig(stream, &scanInputConfig{})
}

func ScanWithConfig(stream ScanStream, sc *scanInputConfig) error {
	config, err := stream.Recv()
	if err != nil {
		return err
	}

	streamCtx := stream.Context()

	var taskId string
	var m *scanManager
	errC := make(chan error)
	switch strings.ToLower(config.GetControlMode()) {
	case "start":
		taskId = uuid.New().String()
		m, err = createSyntaxflowTaskById(taskId, streamCtx, config, stream, sc)
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
	case "status":
		taskId = config.ResumeTaskId
		m, err = LoadSyntaxflowTaskFromDB(taskId, streamCtx, stream)
		if err != nil {
			return err
		}
		err = m.StatusTask()
		return err
	case "resume":
		taskId = config.GetResumeTaskId()
		m, err = LoadSyntaxflowTaskFromDB(taskId, streamCtx, stream)
		if err != nil {
			return err
		}
		m.Resume()
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
	case <-streamCtx.Done():
		m.Stop()
		RemoveSyntaxFlowTaskByID(taskId)
		m.status = schema.SYNTAXFLOWSCAN_DONE
		m.SaveTask()
		return utils.Error("client canceled")
	}
}
