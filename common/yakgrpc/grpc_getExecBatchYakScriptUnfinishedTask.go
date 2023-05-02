package yakgrpc

import (
	"context"
	"yaklang.io/yaklang/common/go-funk"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
)

func (s *Server) GetExecBatchYakScriptUnfinishedTask(ctx context.Context, req *ypb.Empty) (*ypb.GetExecBatchYakScriptUnfinishedTaskResponse, error) {
	manager := NewProgressManager(s.GetProjectDatabase())
	tasks := funk.Map(manager.GetProgressFromDatabase(KEY_ProgressManager), func(i *Progress) *ypb.ExecBatchYakScriptUnfinishedTask {
		return &ypb.ExecBatchYakScriptUnfinishedTask{
			Percent:              i.CurrentProgress,
			CreatedAt:            i.CreatedAt,
			Uid:                  i.Uid,
			YakScriptOnlineGroup: i.YakScriptOnlineGroup,
			TaskName:             i.TaskName,
		}
	}).([]*ypb.ExecBatchYakScriptUnfinishedTask)
	return &ypb.GetExecBatchYakScriptUnfinishedTaskResponse{Tasks: tasks}, nil
}

func (s *Server) GetExecBatchYakScriptUnfinishedTaskByUid(ctx context.Context, req *ypb.GetExecBatchYakScriptUnfinishedTaskByUidRequest) (*ypb.ExecBatchYakScriptRequest, error) {
	manager := NewProgressManager(s.GetProjectDatabase())
	reqResult, err := manager.GetProgressByUid(req.GetUid(), false)
	if err != nil {
		return nil, err
	}
	return reqResult, nil
}

func (s *Server) PopExecBatchYakScriptUnfinishedTaskByUid(ctx context.Context, req *ypb.GetExecBatchYakScriptUnfinishedTaskByUidRequest) (*ypb.ExecBatchYakScriptRequest, error) {
	manager := NewProgressManager(s.GetProjectDatabase())
	reqResult, err := manager.GetProgressByUid(req.GetUid(), true)
	if err != nil {
		return nil, err
	}
	return reqResult, nil
}

func (s *Server) GetSimpleDetectUnfinishedTask(ctx context.Context, req *ypb.Empty) (*ypb.GetSimpleDetectUnfinishedTaskResponse, error) {
	manager := NewProgressManager(s.GetProjectDatabase())
	tasks := funk.Map(manager.GetProgressFromDatabase(KEY_SimpleDetectManager), func(i *Progress) *ypb.SimpleDetectUnfinishedTask {
		return &ypb.SimpleDetectUnfinishedTask{
			Percent:              i.CurrentProgress,
			CreatedAt:            i.CreatedAt,
			Uid:                  i.Uid,
			YakScriptOnlineGroup: i.YakScriptOnlineGroup,
			TaskName:             i.TaskName,
			LastRecordPtr:        i.LastRecordPtr,
		}
	}).([]*ypb.SimpleDetectUnfinishedTask)
	return &ypb.GetSimpleDetectUnfinishedTaskResponse{Tasks: tasks}, nil
}

func (s *Server) GetSimpleDetectUnfinishedTaskByUid(ctx context.Context, req *ypb.GetExecBatchYakScriptUnfinishedTaskByUidRequest) (*ypb.RecordPortScanRequest, error) {
	manager := NewProgressManager(s.GetProjectDatabase())
	reqResult, err := manager.GetSimpleProgressByUid(req.GetUid(), false)
	if err != nil {
		return nil, err
	}
	return reqResult, nil
}

func (s *Server) PopSimpleDetectUnfinishedTaskByUid(ctx context.Context, req *ypb.GetExecBatchYakScriptUnfinishedTaskByUidRequest) (*ypb.RecordPortScanRequest, error) {
	manager := NewProgressManager(s.GetProjectDatabase())
	reqResult, err := manager.GetSimpleProgressByUid(req.GetUid(), true)
	if err != nil {
		return nil, err
	}
	return reqResult, nil
}
