package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) GetExecBatchYakScriptUnfinishedTask(ctx context.Context, req *ypb.Empty) (*ypb.GetExecBatchYakScriptUnfinishedTaskResponse, error) {
	_, data, err := yakit.QueryProgress(s.GetProjectDatabase(), &ypb.Paging{
		Limit: 30,
	}, &ypb.UnfinishedTaskFilter{
		ProgressSource: []string{KEY_ProgressManager},
	})
	if err != nil {
		return nil, err
	}
	tasks := funk.Map(data, func(i *yakit.Progress) *ypb.ExecBatchYakScriptUnfinishedTask {
		return &ypb.ExecBatchYakScriptUnfinishedTask{
			Percent:              i.CurrentProgress,
			CreatedAt:            i.CreatedAt.Unix(),
			Uid:                  i.RuntimeId,
			YakScriptOnlineGroup: i.YakScriptOnlineGroup,
			TaskName:             i.TaskName,
		}
	}).([]*ypb.ExecBatchYakScriptUnfinishedTask)
	return &ypb.GetExecBatchYakScriptUnfinishedTaskResponse{Tasks: tasks}, nil
}

func (s *Server) GetExecBatchYakScriptUnfinishedTaskByUid(ctx context.Context, req *ypb.GetExecBatchYakScriptUnfinishedTaskByUidRequest) (*ypb.ExecBatchYakScriptRequest, error) {
	return GetBatchYakScriptRequestByRuntimeId(s.GetProjectDatabase(), req.GetUid())
}

func (s *Server) PopExecBatchYakScriptUnfinishedTaskByUid(ctx context.Context, req *ypb.GetExecBatchYakScriptUnfinishedTaskByUidRequest) (*ypb.ExecBatchYakScriptRequest, error) {
	return DeleteBatchYakScriptRequestByRuntimeId(s.GetProjectDatabase(), req.GetUid())
}

func (s *Server) GetSimpleDetectUnfinishedTask(ctx context.Context, req *ypb.Empty) (*ypb.GetSimpleDetectUnfinishedTaskResponse, error) {
	_, data, err := yakit.QueryProgress(s.GetProjectDatabase(), &ypb.Paging{
		Limit: 30,
	}, &ypb.UnfinishedTaskFilter{
		ProgressSource: []string{KEY_SimpleDetectManager},
	})
	if err != nil {
		return nil, err
	}
	tasks := funk.Map(data, func(i *yakit.Progress) *ypb.SimpleDetectUnfinishedTask {
		return &ypb.SimpleDetectUnfinishedTask{
			Percent:              i.CurrentProgress,
			CreatedAt:            i.CreatedAt.Unix(),
			Uid:                  i.RuntimeId,
			YakScriptOnlineGroup: i.YakScriptOnlineGroup,
			TaskName:             i.TaskName,
		}
	}).([]*ypb.SimpleDetectUnfinishedTask)
	return &ypb.GetSimpleDetectUnfinishedTaskResponse{Tasks: tasks}, nil
}

func (s *Server) GetSimpleDetectUnfinishedTaskByUid(ctx context.Context, req *ypb.GetExecBatchYakScriptUnfinishedTaskByUidRequest) (*ypb.RecordPortScanRequest, error) {
	return GetSimpleDetectUnfinishedTaskByUid(s.GetProjectDatabase(), req.GetUid())
}

func (s *Server) PopSimpleDetectUnfinishedTaskByUid(ctx context.Context, req *ypb.GetExecBatchYakScriptUnfinishedTaskByUidRequest) (*ypb.RecordPortScanRequest, error) {
	return DeleteSimpleDetectUnfinishedTaskByUid(s.GetProjectDatabase(), req.GetUid())
}
