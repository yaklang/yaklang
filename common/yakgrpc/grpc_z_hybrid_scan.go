package yakgrpc

import (
	"context"
	"github.com/samber/lo"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strconv"
	"strings"
	"sync"
	"time"
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
				time.Sleep(3 * time.Second)
				taskCancel()
			}
		}()
	}

	var taskStream = newWrapperHybridScanStream(taskCtx, stream)
	taskStream.RequestHandler = func(request *ypb.HybridScanRequest) bool {
		if request.Control {
			return false
		}
		return true
	}

	errC := make(chan error)
	var taskId string
	var taskManager *HybridScanTaskManager
	switch strings.ToLower(firstRequest.HybridScanMode) {
	case "status": // 查询任务状态
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

		stream.Send(&ypb.HybridScanResponse{
			TotalTargets:     t.TotalTargets,
			TotalPlugins:     t.TotalPlugins,
			TotalTasks:       t.TotalTargets * t.TotalPlugins,
			FinishedTasks:    t.FinishedTasks,
			FinishedTargets:  t.FinishedTargets,
			HybridScanTaskId: t.TaskId,
			ScanConfig:       string(t.ScanConfig),
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
	case "resume":
		taskId = firstRequest.GetResumeTaskId()
		if taskId == "" {
			return utils.Error("resume task id is empty")
		}
		taskManager, err = CreateHybridTask(taskId, taskCtx)
		if err != nil {
			return err
		}
		go func() {
			err := s.hybridScanResume(taskManager, taskStream)
			if err != nil {
				utils.TryWriteChannel(errC, err)
			}
		}()
	case "new":
		taskId = uuid.NewV4().String()
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
		taskManager.PauseEffect()
		taskManager.Stop()
		taskManager.Resume()
		RemoveHybridTask(taskId)
		return utils.Error("client canceled")
	}
}

func (s *Server) QueryHybridScanTask(ctx context.Context, request *ypb.QueryHybridScanTaskRequest) (*ypb.QueryHybridScanTaskResponse, error) {
	p, tasks, err := yakit.QueryHybridScan(s.GetProjectDatabase(), request)
	if err != nil {
		return nil, err
	}
	var data []*ypb.HybridScanTask
	data = lo.Map(tasks, func(item *yakit.HybridScanTask, index int) *ypb.HybridScanTask {
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
		}
	})
	return &ypb.QueryHybridScanTaskResponse{
		Pagination: request.GetPagination(),
		Data:       data,
		Total:      int64(p.TotalRecord),
	}, nil
}

func (s *Server) DeleteHybridScanTask(ctx context.Context, request *ypb.DeleteHybridScanTaskRequest) (*ypb.Empty, error) {
	if request.GetDeleteAll() {
		if err := s.GetProjectDatabase().Unscoped().Where("true").Delete(&yakit.HybridScanTask{}).Error; err != nil {
			return nil, err
		}
		return &ypb.Empty{}, nil
	}

	if t := request.GetTaskId(); t != "" {
		if err := s.GetProjectDatabase().Unscoped().Where("task_id = ?", t).Delete(&yakit.HybridScanTask{}).Error; err != nil {
			return nil, err
		}
		return &ypb.Empty{}, nil
	}
	return &ypb.Empty{}, nil
}
