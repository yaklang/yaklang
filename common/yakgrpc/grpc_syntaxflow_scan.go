package yakgrpc

import (
	"context"
	"sync"

	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"

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
