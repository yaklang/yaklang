package yakscript

import (
	"context"

	"github.com/davecgh/go-spew/spew"
	"google.golang.org/grpc"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// StreamSender matches gRPC stream's Send + Context used for script execution (no Server dependency).
type StreamSender interface {
	Send(*ypb.ExecResult) error
	Context() context.Context
}

type fakeStreamInstance struct {
	ctx     context.Context
	handler func(*ypb.ExecResult) error
	grpc.ServerStream
}

func (f *fakeStreamInstance) Send(result *ypb.ExecResult) error {
	if f == nil {
		log.Error("fakeStreamInstance empty")
		return nil
	}
	if f.handler != nil {
		return f.handler(result)
	}
	log.Infof("*fakeStreamInstance.Send Called with: %v", spew.Sdump(result))
	return nil
}

func (f *fakeStreamInstance) Context() context.Context {
	return f.ctx
}

// NewFakeStream builds a stream that forwards ExecResult to handler (for non-gRPC callers).
func NewFakeStream(ctx context.Context, handler func(result *ypb.ExecResult) error) *fakeStreamInstance {
	return &fakeStreamInstance{
		ctx:     ctx,
		handler: handler,
	}
}
