package yakgrpc

import (
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// StartAITask retains the RPC contract so older clients receive a migration error.
func (s *Server) StartAITask(ypb.Yak_StartAITaskServer) error {
	return grpcstatus.Error(codes.Unimplemented, "StartAITask is deprecated; use StartAIReAct.")
}
