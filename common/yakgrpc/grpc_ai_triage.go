package yakgrpc

import (
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// StartAITriage retains the RPC contract so older clients receive a migration error.
func (s *Server) StartAITriage(ypb.Yak_StartAITriageServer) error {
	return grpcstatus.Error(codes.Unimplemented, "StartAITriage is deprecated; use StartAIReAct.")
}
