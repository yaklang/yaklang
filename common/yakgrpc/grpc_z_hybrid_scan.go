package yakgrpc

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

func (s *Server) HybridScan(stream ypb.Yak_HybridScanServer) error {
	firstRequest, err := stream.Recv()
	if err != nil {
		return err
	}

	if !firstRequest.Control {
		return utils.Errorf("first request must be control request")
	}

	switch strings.ToLower(firstRequest.HybridScanMode) {
	case "new":
		log.Info("start to create new hybrid scan task")
		return s.hybridScanNewTask(stream, firstRequest)
	default:
		return utils.Error("not implemented")
	}
}
