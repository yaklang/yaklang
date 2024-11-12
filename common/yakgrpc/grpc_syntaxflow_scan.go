package yakgrpc

import (
	"context"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

func (s *Server) SyntaxFlowScan(stream ypb.Yak_SyntaxFlowScanServer) error {
	firstRequest, err := stream.Recv()
	if err != nil {
		return err
	}

	streamCtx := stream.Context()
	getTaskManager := func() (*SyntaxFlowScanManager, error) {
		taskId := firstRequest.GetResumeTaskId()
		if taskId == "" {
			return nil, utils.Error("get syntaxflow scan manager failed: task id is empty")
		}
		taskManager, err := GetSyntaxFlowTask(taskId)
		if err != nil {
			taskManager, err = CreateSyntaxFlowTask(taskId, streamCtx)
			if err != nil {
				return nil, err
			}
		}
		if streamCtx != nil {
			taskManager.ctx = streamCtx
			ctx, cancel := context.WithCancel(streamCtx)
			taskManager.ctx = ctx
			taskManager.cancel = cancel
		}
		return taskManager, nil
	}

	var taskId string
	var taskManager *SyntaxFlowScanManager
	errC := make(chan error)
	switch strings.ToLower(firstRequest.GetControlMode()) {
	case "start":
		taskId = uuid.New().String()
		taskManager, err = CreateSyntaxFlowTask(taskId, streamCtx)
		if err != nil {
			return err
		}
		log.Info("start to create syntaxflow scan")
		go func() {
			err := s.syntaxFlowScanNewTask(taskManager, firstRequest, stream)
			if err != nil {
				utils.TryWriteChannel(errC, err)
			}
			close(errC)
		}()
	case "status":
		taskManager, err = getTaskManager()
		if err != nil {
			return err
		}
		taskId = taskManager.TaskId()
		err = s.syntaxFlowStatusTask(taskManager, stream)
		return err
	case "resume":
		if taskManager, err = getTaskManager(); err != nil {
			return err
		}
		taskManager.Resume()
		go func() {
			err := s.syntaxFlowResumeTask(taskManager, stream)
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
		RemoveSyntaxFlowTask(taskId)
		if ok {
			return err
		}
		return nil
	case <-streamCtx.Done():
		taskManager.Stop()
		RemoveSyntaxFlowTask(taskId)
		return utils.Error("client canceled")
	}
}
