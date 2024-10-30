package yakgrpc

import (
	"strings"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) SyntaxFlowScan(stream ypb.Yak_SyntaxFlowScanServer) error {
	firstRequest, err := stream.Recv()
	if err != nil {
		return err
	}

	// TODO: Implement the context cancel logic of SyntaxFlowScan
	streamCtx := stream.Context()
	_ = streamCtx

	switch strings.ToLower(firstRequest.GetControlMode()) {
	case "start":
		if manager, err := CreateSyntaxFlowScanManager(streamCtx, stream, firstRequest); err != nil {
			return err
		} else {
			if err := manager.Start(); err != nil {
				return err
			}
		}
	}

	return nil
}
