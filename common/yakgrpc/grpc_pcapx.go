package yakgrpc

import (
	"github.com/google/gopacket"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) PcapX(stream ypb.Yak_PcapXServer) error {
	firstReq, err := stream.Recv()
	if err != nil {
		return err
	}

	//
	err = pcaputil.Start(
		pcaputil.WithEveryPacket(func(a gopacket.Packet) {
			a.Dump()
		}),
		pcaputil.WithDevice(firstReq.GetNetInterfaceList()...),
	)
	if err != nil {
		return err
	}

	return nil
}
