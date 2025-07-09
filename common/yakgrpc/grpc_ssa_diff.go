package yakgrpc

import (
	"context"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type SSARiskDiffRequestStream interface {
	Send(response *ypb.SSARiskDiffResponse) error
	Context() context.Context
}

type wrapperSSARiskDiffStream struct {
	ctx            context.Context
	root           ypb.Yak_SSARiskDiffServer
	RequestHandler func(request *ypb.SSARiskDiffRequest) bool
	sendMutex      *sync.Mutex
}

func newWrapperSSARiskDiffStream(ctx context.Context, stream ypb.Yak_SSARiskDiffServer) *wrapperSSARiskDiffStream {
	return &wrapperSSARiskDiffStream{
		root: stream, ctx: ctx,
		sendMutex: new(sync.Mutex),
	}
}

func (w *wrapperSSARiskDiffStream) Send(r *ypb.SSARiskDiffResponse) error {
	w.sendMutex.Lock()
	defer w.sendMutex.Unlock()
	return w.root.Send(r)
}

func (w *wrapperSSARiskDiffStream) Context() context.Context {
	return w.ctx
}

func (s *Server) SSARiskDiff(req *ypb.SSARiskDiffRequest, server ypb.Yak_SSARiskDiffServer) error {
	stream := newWrapperSSARiskDiffStream(server.Context(), server)
	context := stream.Context()
	if req.GetBaseLine() == nil || req.GetCompare() == nil {
		return utils.Error("base and compare are required")
	}

	if req.GetBaseLine().GetProgramName() == "" && req.GetBaseLine().GetRiskRuntimeId() == "" {
		return utils.Error("base and compare are required")
	}

	base := req.GetBaseLine()
	compare := req.GetCompare()

	res, err := yakit.DoRiskDiff(context, base, compare)
	if err != nil {
		return err
	}

	for re := range res {
		stream.Send(&ypb.SSARiskDiffResponse{
			BaseRisk:    re.BaseValue.ToGRPCModel(),
			CompareRisk: re.NewValue.ToGRPCModel(),
			RuleName:    re.FromRule,
			Status:      string(re.Status),
		})
	}
	return nil
}
