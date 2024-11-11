package yakgrpc

import (
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

func (s *Server) SyntaxFlowScan(stream ypb.Yak_SyntaxFlowScanServer) error {
	firstRequest, err := stream.Recv()
	if err != nil {
		return err
	}

	streamCtx := stream.Context()
	recoverSyntaxFlowScanStatus := func() error {
		taskId := firstRequest.GetResumeTaskId()
		if taskId == "" {
			return utils.Error("get syntaxflow scan manager failed: task id is empty")
		}

		task, err := yakit.GetSyntaxFlowScanTaskById(consts.GetGormProjectDatabase(), taskId)
		if err != nil {
			return err
		}
		result, err := ssadb.GetResultByTaskID(taskId)
		if err != nil {
			return utils.Errorf("get syntaxflow scan manager failed: %s", err)
		}
		risks := result.ToGRPCModelRisk()

		stream.Send(&ypb.SyntaxFlowScanResponse{
			TaskID: task.TaskId,
			Status: task.Status,
			Result: result.ToGRPCModel(),
			Risks:  risks,
		})
		return nil
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
		return recoverSyntaxFlowScanStatus()
	case "resume":
		if err := recoverSyntaxFlowScanStatus(); err != nil {
			return err
		}
		taskId = firstRequest.GetResumeTaskId()
		if taskId == "" {
			return utils.Error("resume task id is empty")
		}
		taskManager, err = CreateSyntaxFlowTask(taskId, streamCtx)
		if err != nil {
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
		RemoveHybridTask(taskId)
		return utils.Error("client canceled")
	}
}
