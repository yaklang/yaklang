package yakgrpc

import (
	"context"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
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
}

func newWrapperHybridScanStream(ctx context.Context, stream ypb.Yak_HybridScanServer) *wrapperHybridScanStream {
	return &wrapperHybridScanStream{
		root: stream, ctx: ctx,
	}
}

func (w *wrapperHybridScanStream) Send(r *ypb.HybridScanResponse) error {
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

	rootCtx := stream.Context()
	taskCtx := rootCtx
	if firstRequest.GetDetach() {
		taskCtx = context.Background()
	}

	var taskStream = newWrapperHybridScanStream(taskCtx, stream)
	taskStream.RequestHandler = func(request *ypb.HybridScanRequest) bool {
		if request.Control {
			return false
		}
		return true
	}

	switch strings.ToLower(firstRequest.HybridScanMode) {
	case "new":
		taskId := uuid.NewV4().String()
		taskManager, err := CreateHybridTask(taskId, taskCtx)
		if err != nil {
			return err
		}
		log.Info("start to create new hybrid scan task")
		errC := make(chan error)
		go func() {
			err := s.hybridScanNewTask(taskManager, taskStream, firstRequest)
			if err != nil {
				utils.TryWriteChannel(errC, err)
			}
		}()
		select {
		case err, ok := <-errC:
			if ok {
				return err
			}
		case <-rootCtx.Done():
			return utils.Error("client canceled")
		}
		return nil
	default:
		return utils.Error("invalid hybrid scan mode")
	}
}
