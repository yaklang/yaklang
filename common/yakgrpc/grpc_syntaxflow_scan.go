package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type SyntaxFlowScanTaskConfig struct {
	*ypb.SyntaxFlowScanRequest
	RuleNames []string `json:"rule_names"`
}

func (s *Server) SyntaxFlowScan(stream ypb.Yak_SyntaxFlowScanServer) error {
	wrapperStream := newWrapperSyntaxFlowScanStream(stream.Context(), stream)
	return syntaxflow_scan.Scan(wrapperStream)
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

var _ syntaxflow_scan.ScanStream = (*wrapperSyntaxFlowScanStream)(nil)

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

var _ syntaxflow_scan.ScanStream = (*syntaxFlowScanStreamImpl)(nil)

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
	// log.Infof("resp : %v", resp)
	select {
	case <-s.ctx.Done():
		// log.Infof("context canceled")
		return utils.Error("context canceled")
	default:
		if s.stream != nil {
			return s.stream(resp)
		}
	}
	return nil
}

func SyntaxFlowScan(ctx context.Context, config *ypb.SyntaxFlowScanRequest, callBack syntaxFlowScanStreamCallback) {
	stream := NewSyntaxFlowScanStream(ctx, callBack)
	stream.request <- config
	syntaxflow_scan.Scan(stream)
	stream.Done()
}
