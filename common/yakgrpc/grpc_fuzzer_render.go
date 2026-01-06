//go:build !yakit_exclude

package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ! 已弃用
func (s *Server) RenderHTTPFuzzerPacket(ctx context.Context, req *ypb.RenderHTTPFuzzerPacketRequest) (*ypb.RenderHTTPFuzzerPacketResponse, error) {
	packet := req.GetPacket()
	res, err := mutate.FuzzTagExec(packet, mutate.Fuzz_WithEnableDangerousTag())
	if err != nil || len(res) == 0 {
		return nil, utils.Wrapf(err, "cannot render fuzztag: %v", packet)
	}
	newPacket := res[0]
	return &ypb.RenderHTTPFuzzerPacketResponse{
		Packet: lowhttp.ConvertHTTPRequestToFuzzTag([]byte(newPacket)),
	}, nil
}
