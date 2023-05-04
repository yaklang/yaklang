package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) Echo(ctx context.Context, req *ypb.EchoRequest) (*ypb.EchoResposne, error) {
	return &ypb.EchoResposne{Result: req.GetText()}, nil
}
