package yakgrpc

import (
	"context"
	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

func pcapIftoYpbIf(item *pcap.Interface, index int) *ypb.NetInterface {
	var is4, is6 = false, false
	var addr []string
	var ip string
	for _, a := range item.Addresses {
		addr = append(addr, a.IP.String())
		if !is4 {
			ip = a.IP.String()
			is4 = utils.IsIPv4(a.IP.String())
		}
		if !is6 {
			is6 = utils.IsIPv6(a.IP.String())
		}
	}
	return &ypb.NetInterface{
		Name:   item.Name,
		Addr:   strings.Join(addr, ", "),
		IP:     ip,
		IsIpv4: is4,
		IsIpv6: is6,
	}
}

func (s *Server) GetPcapMetadata(ctx context.Context, req *ypb.PcapMetadataRequest) (*ypb.PcapMetadata, error) {
	ifs := lo.Map(
		pcaputil.AllDevices(),
		pcapIftoYpbIf,
	)

	ifIns, _, _, err := netutil.GetPublicRoute()
	if err != nil {
		log.Errorf("get public route failed: %s", err)
		return nil, err
	}

	var defaultIfName *ypb.NetInterface
	for _, ifItem := range ifs {
		name, err := pcaputil.IfaceNameToPcapIfaceName(ifIns.Name)
		if err != nil {
			return nil, err
		}
		if ifItem.Name == name {
			defaultIfName = ifItem
			break
		}
	}

	return &ypb.PcapMetadata{
		AvailablePcapDevices: ifs,
		AvailableSessionTypes: []*ypb.KVPair{
			{Key: "tcp", Value: "TCP"},
			{Key: "icmp", Value: "ICMP"},
			{Key: "arp", Value: "ARP"},
		},
		AvailableLinkLayerTypes: []*ypb.KVPair{
			{Key: "ethernet", Value: "以太网"},
			{Key: "arp", Value: "ARP"},
			{Key: "", Value: "本地"},
		},
		AvailableNetworkLayerTypes: []*ypb.KVPair{
			{Key: "ipv4", Value: "IPv4"},
			{Key: "ipv6", Value: "IPv6"},
			{Key: "icmp", Value: "ICMP"},
			{Key: "icmpv6", Value: "ICMPv6"},
		},
		AvailableTransportLayerTypes: []*ypb.KVPair{
			{Key: "tcp", Value: "TCP"},
		},
		DefaultPublicNetInterface: defaultIfName,
	}, nil
}

func (s *Server) PcapX(stream ypb.Yak_PcapXServer) error {
	firstReq, err := stream.Recv()
	if err != nil {
		return err
	}

	list := utils.StringArrayFilterEmpty(firstReq.GetNetInterfaceList())

	storageManager := yakit.NewTrafficStorageManager(consts.GetGormProjectDatabase())

	// run pcap
	err = pcaputil.Start(
		pcaputil.WithContext(stream.Context()),
		pcaputil.WithEveryPacket(func(a gopacket.Packet) {
			err := storageManager.SaveRawPacket(a)
			if err != nil {
				log.Errorf("save raw packet failed: %s", err)
			}
		}),
		pcaputil.WithDevice(list...),
		pcaputil.WithOnTrafficFlowCreated(func(flow *pcaputil.TrafficFlow) {
			err := storageManager.CreateTCPReassembledFlow(flow)
			if err != nil {
				log.Errorf("create flow failed: %s", err)
			}
		}),
		pcaputil.WithOnTrafficFlowOnDataFrameReassembled(func(flow *pcaputil.TrafficFlow, conn *pcaputil.TrafficConnection, frame *pcaputil.TrafficFrame) {
			err := storageManager.SaveTCPReassembledFrame(flow, frame)
			if err != nil {
				log.Errorf("save frame failed: %s", err)
			}
		}),
		pcaputil.WithOnTrafficFlowClosed(func(reason pcaputil.TrafficFlowCloseReason, flow *pcaputil.TrafficFlow) {
			var err error
			switch reason {
			case pcaputil.TrafficFlowCloseReason_INACTIVE:
				err = storageManager.CloseTCPFlow(flow, false)
			case pcaputil.TrafficFlowCloseReason_FIN:
				err = storageManager.CloseTCPFlow(flow, false)
			case pcaputil.TrafficFlowCloseReason_RST:
				err = storageManager.CloseTCPFlow(flow, true)
			}
			if err != nil {
				log.Errorf("close flow failed: %s", err)
			}
		}),
	)
	if err != nil {
		return err
	}

	return nil
}
