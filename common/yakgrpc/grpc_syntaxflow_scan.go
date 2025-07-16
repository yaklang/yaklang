package yakgrpc

import (
	"context"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type SyntaxFlowScanTaskConfig struct {
	*ypb.SyntaxFlowScanRequest
	RuleNames []string `json:"rule_names"`
}

func (s *Server) SyntaxFlowScan(stream ypb.Yak_SyntaxFlowScanServer) error {
	wrapperStream := newWrapperSyntaxFlowScanStream(stream.Context(), stream)
	return syntaxFlowScan(wrapperStream)
}

type SyntaxFlowScanStream interface {
	Recv() (*ypb.SyntaxFlowScanRequest, error)
	Send(*ypb.SyntaxFlowScanResponse) error
	Context() context.Context
}

func syntaxFlowScan(stream SyntaxFlowScanStream) error {
	config, err := stream.Recv()
	if err != nil {
		return err
	}

	streamCtx := stream.Context()

	var taskId string
	var m *SyntaxFlowScanManager
	errC := make(chan error)
	switch strings.ToLower(config.GetControlMode()) {
	case "start":
		taskId = uuid.New().String()
		m, err = CreateSyntaxflowTaskById(taskId, streamCtx, config, stream)
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

type syntaxFlowScanStreamImpl struct {
	ctx    context.Context
	stream syntaxFlowScanStreamCallback

	request   chan *ypb.SyntaxFlowScanRequest
	sendMutex *sync.Mutex
}

type syntaxFlowScanStreamCallback func(*ypb.SyntaxFlowScanResponse) error

func NewSyntaxFlowScanStream(ctx context.Context, callback syntaxFlowScanStreamCallback) *syntaxFlowScanStreamImpl {
	ctx = context.WithoutCancel(ctx)
	ret := &syntaxFlowScanStreamImpl{
		ctx:       ctx,
		stream:    callback,
		sendMutex: new(sync.Mutex),
	}
	ret.request = make(chan *ypb.SyntaxFlowScanRequest, 1)
	return ret
}

var _ SyntaxFlowScanStream = (*wrapperSyntaxFlowScanStream)(nil)

type wrapperSyntaxFlowScanStream struct {
	ctx            context.Context
	root           ypb.Yak_SyntaxFlowScanServer
	RequestHandler func(request *ypb.SyntaxFlowScanRequest) bool
	sendMutex      *sync.Mutex
}

func (w *wrapperSyntaxFlowScanStream) Recv() (*ypb.SyntaxFlowScanRequest, error) {
	return w.root.Recv()
}

func newWrapperSyntaxFlowScanStream(ctx context.Context, stream ypb.Yak_SyntaxFlowScanServer) *wrapperSyntaxFlowScanStream {
	return &wrapperSyntaxFlowScanStream{
		root: stream, ctx: ctx,
		sendMutex: new(sync.Mutex),
	}
}

func (w *wrapperSyntaxFlowScanStream) Send(r *ypb.SyntaxFlowScanResponse) error {
	w.sendMutex.Lock()
	defer w.sendMutex.Unlock()
	return w.root.Send(r)
}

func (w *wrapperSyntaxFlowScanStream) Context() context.Context {
	return w.ctx
}

func (s *syntaxFlowScanStreamImpl) Done() {
	s.ctx.Done()
}

var _ SyntaxFlowScanStream = (*syntaxFlowScanStreamImpl)(nil)

func (s *syntaxFlowScanStreamImpl) Recv() (*ypb.SyntaxFlowScanRequest, error) {
	select {
	case <-s.ctx.Done():
		return nil, utils.Error("context canceled")
	default:
		if s.request != nil {
			return <-s.request, nil
		}
	}
	return nil, utils.Error("no request")
}

func (s *syntaxFlowScanStreamImpl) Context() context.Context {
	return s.ctx
}

func (s *syntaxFlowScanStreamImpl) Send(resp *ypb.SyntaxFlowScanResponse) error {
	select {
	case <-s.ctx.Done():
		return utils.Error("context canceled")
	default:
		if s.stream != nil {
			s.sendMutex.Lock()
			defer s.sendMutex.Unlock()
			return s.stream(resp)
		}
	}
	return nil
}

func SyntaxFlowScan(ctx context.Context, config *ypb.SyntaxFlowScanRequest, callBack syntaxFlowScanStreamCallback) {
	stream := NewSyntaxFlowScanStream(ctx, callBack)
	stream.request <- config
	syntaxFlowScan(stream)
	stream.Done()
}
