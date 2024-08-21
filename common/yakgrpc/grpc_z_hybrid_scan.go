package yakgrpc

import (
	"context"
	"encoding/json"
	uuid "github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strconv"
	"strings"
	"sync"
)

type HybridScanRequestStream interface {
	Send(response *ypb.HybridScanResponse) error
	Recv() (*ypb.HybridScanRequest, error)
	Context() context.Context
}

type wrapperHybridScanStream struct {
	ctx            context.Context
	root           ypb.Yak_HybridScanServer
	RequestHandler func(request *ypb.HybridScanRequest) bool
	sendMutex      *sync.Mutex
}

func newWrapperHybridScanStream(ctx context.Context, stream ypb.Yak_HybridScanServer) *wrapperHybridScanStream {
	return &wrapperHybridScanStream{
		root: stream, ctx: ctx,
		sendMutex: new(sync.Mutex),
	}
}

func (w *wrapperHybridScanStream) Send(r *ypb.HybridScanResponse) error {
	w.sendMutex.Lock()
	defer w.sendMutex.Unlock()
	return w.root.Send(r)
}

func (w *wrapperHybridScanStream) Recv() (*ypb.HybridScanRequest, error) {
	req, err := w.root.Recv()
	if err != nil {
		return nil, err
	}
	if w.RequestHandler != nil {
		if !w.RequestHandler(req) {
			return w.Recv()
		}
	}
	return req, nil
}

func (w *wrapperHybridScanStream) Context() context.Context {
	return w.ctx
}

func (s *Server) HybridScan(stream ypb.Yak_HybridScanServer) error {
	defer func() {
		if err := recover(); err != nil {
			log.Error(err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	firstRequest, err := stream.Recv()
	if err != nil {
		return err
	}
	if !firstRequest.Control {
		return utils.Errorf("first request must be control request")
	}

	streamCtx := stream.Context()
	var taskCtx context.Context
	if firstRequest.GetDetach() {
		taskCtx = context.Background()
	} else {
		var taskCancel context.CancelFunc
		taskCtx, taskCancel = context.WithCancel(context.Background())
		go func() {
			select {
			case <-streamCtx.Done():
				//time.Sleep(3 * time.Second)
				taskCancel()
			}
		}()
	}

	var taskStream = newWrapperHybridScanStream(taskCtx, stream)
	taskStream.RequestHandler = func(request *ypb.HybridScanRequest) bool {
		//if request.Control {
		//	return false
		//}
		return true
	}

	errC := make(chan error)
	var taskId string
	var taskManager *HybridScanTaskManager

	recoverHybridScanStatus := func() error {
		taskId = firstRequest.GetResumeTaskId()
		if taskId == "" {
			return utils.Error("resume task id is empty")
		}
		t, err := yakit.GetHybridScanByTaskId(s.GetProjectDatabase(), taskId)
		if err != nil {
			return err
		}
		risks, err := yakit.GetRisksByRuntimeId(s.GetProjectDatabase(), taskId)
		if err != nil {
			return err
		}

		var scanConfig ypb.HybridScanRequest
		err = json.Unmarshal(t.ScanConfig, &scanConfig)
		if err != nil {
			return err
		}

		stream.Send(&ypb.HybridScanResponse{
			TotalTargets:     t.TotalTargets,
			TotalPlugins:     t.TotalPlugins,
			TotalTasks:       t.TotalTargets * t.TotalPlugins,
			FinishedTasks:    t.FinishedTasks,
			FinishedTargets:  t.FinishedTargets,
			HybridScanTaskId: t.TaskId,
			Status:           t.Status,
			HybridScanConfig: &scanConfig,
		})

		client := yaklib.NewVirtualYakitClient(func(result *ypb.ExecResult) error {
			result.RuntimeID = taskId
			return stream.Send(&ypb.HybridScanResponse{
				TotalTargets:     t.TotalTargets,
				TotalPlugins:     t.TotalPlugins,
				TotalTasks:       t.TotalTargets * t.TotalPlugins,
				FinishedTasks:    t.FinishedTasks,
				FinishedTargets:  t.FinishedTargets,
				HybridScanTaskId: t.TaskId,
				ExecResult:       result,
				Status:           t.Status,
			})
		})

		err = client.Output(&yaklib.YakitStatusCard{ // card
			Id: "漏洞/风险/指纹", Data: strconv.Itoa(len(risks)), Tags: nil,
		})
		if err != nil {
			return err
		}

		for _, riskInfo := range risks { // risks table
			err := client.Output(riskInfo)
			if err != nil {
				return err
			}
		}
		return nil
	}

	switch strings.ToLower(firstRequest.HybridScanMode) {
	case "status": // 查询任务状态
		return recoverHybridScanStatus()
	case "resume":
		if err := recoverHybridScanStatus(); err != nil {
			return err
		}
		taskId = firstRequest.GetResumeTaskId()
		if taskId == "" {
			return utils.Error("resume task id is empty")
		}
		taskManager, err = CreateHybridTask(taskId, taskCtx)
		taskManager.Resume()
		if err != nil {
			return err
		}
		go func() {
			err := s.hybridScanResume(taskManager, taskStream)
			if err != nil {
				utils.TryWriteChannel(errC, err)
			}
			close(errC)
		}()
	case "new":
		taskId = uuid.New().String()
		taskManager, err = CreateHybridTask(taskId, taskCtx)
		if err != nil {
			return err
		}
		log.Info("start to create new hybrid scan task")
		go func() {
			err := s.hybridScanNewTask(taskManager, taskStream, firstRequest)
			if err != nil {
				utils.TryWriteChannel(errC, err)
			}
			close(errC)
		}()
	default:
		return utils.Error("invalid hybrid scan mode")
	}

	// wait result
	select {
	case err, ok := <-errC:
		RemoveHybridTask(taskId)
		if ok {
			return err
		}
		return nil
	case <-streamCtx.Done():
		if !firstRequest.GetDetach() {
			taskManager.Stop()
			RemoveHybridTask(taskId)
			return utils.Error("client canceled")
		}
		return nil
	}
}

func (s *Server) QueryHybridScanTask(ctx context.Context, request *ypb.QueryHybridScanTaskRequest) (*ypb.QueryHybridScanTaskResponse, error) {
	p, tasks, err := yakit.QueryHybridScan(s.GetProjectDatabase(), request)
	if err != nil {
		return nil, err
	}
	var data []*ypb.HybridScanTask
	data = lo.Map(tasks, func(item *schema.HybridScanTask, index int) *ypb.HybridScanTask {

		var firstTarget = "未知目标"
		var targets []*HybridScanTarget
		err = json.Unmarshal([]byte(item.Targets), &targets)
		if err == nil && len(targets) > 0 {
			firstTarget = utils.ExtractHost(targets[0].Url)
		}

		return &ypb.HybridScanTask{
			Id:              int64(item.ID),
			CreatedAt:       item.CreatedAt.Unix(),
			UpdatedAt:       item.UpdatedAt.Unix(),
			TaskId:          item.TaskId,
			Status:          item.Status,
			TotalTargets:    item.TotalTargets,
			TotalPlugins:    item.TotalPlugins,
			TotalTasks:      item.TotalTasks,
			FinishedTasks:   item.FinishedTasks,
			FinishedTargets: item.FinishedTargets,
			FirstTarget:     firstTarget,
			Reason:          item.Reason,
		}
	})
	return &ypb.QueryHybridScanTaskResponse{
		Pagination: request.GetPagination(),
		Data:       data,
		Total:      int64(p.TotalRecord),
	}, nil
}

func (s *Server) DeleteHybridScanTask(ctx context.Context, request *ypb.DeleteHybridScanTaskRequest) (*ypb.Empty, error) {
	db := s.GetProjectDatabase().Unscoped()
	if request.GetDeleteAll() {
		if err := db.Where("true").Delete(&schema.HybridScanTask{}).Error; err != nil {
			return nil, err
		}
		return &ypb.Empty{}, nil
	}

	db = yakit.FilterHybridScan(db, request.GetFilter())
	if err := db.Delete(&schema.HybridScanTask{}).Error; err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}
