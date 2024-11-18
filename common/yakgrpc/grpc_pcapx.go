package yakgrpc

import (
	"bytes"
	"context"
	"errors"
	"github.com/gopacket/gopacket"
	"github.com/samber/lo"
	"github.com/yaklang/pcap"
	bin_parser2 "github.com/yaklang/yaklang/common/bin-parser"
	bin_parser "github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"runtime"
	"strconv"
	"strings"
)

func pcapIftoYpbIf(item *pcap.Interface, index int) (*ypb.NetInterface, bool) {
	var is4, is6 = false, false
	var addr []string
	var ip, netName string
	for _, a := range item.Addresses {
		if a.IP == nil {
			continue
		}
		iface, err := netutil.FindInterfaceByIP(a.IP.String())
		if err != nil {
			log.Errorf("find interface by ip failed: %s", err)
			continue
		}
		netName = iface.Name
		addr = append(addr, a.IP.String())
		if !is4 {
			ip = a.IP.String()
			is4 = utils.IsIPv4(a.IP.String())
		}
		if !is6 {
			is6 = utils.IsIPv6(a.IP.String())
		}
	}
	var desc string
	switch runtime.GOOS {
	case "windows":
		desc = item.Description
	case "linux":
		fallthrough
	case "darwin":
		desc = item.Name
	}
	if len(ip) != 0 {
		return &ypb.NetInterface{
			Name:             item.Name,
			NetInterfaceName: netName,
			Description:      desc,
			Addr:             strings.Join(addr, ", "),
			IP:               ip,
			IsIpv4:           is4,
			IsIpv6:           is6,
		}, true
	}
	return nil, false
}

func (s *Server) GetPcapMetadata(ctx context.Context, req *ypb.PcapMetadataRequest) (*ypb.PcapMetadata, error) {
	ifs := lo.FilterMap(
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
		if ifItem.NetInterfaceName == ifIns.Name {
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
func ParseReassembledTraffic(data []byte) ([]any, error) {
	for _, ruleName := range []string{"http", "tls"} {
		info, err := ParseTraffic(data, "application-layer."+ruleName)
		if err == nil {
			return info, nil
		}
	}
	return nil, errors.New("parse traffic failed")
}
func ParseTraffic(data []byte, rule string) ([]any, error) {
	var packageRootNodes []any
	var toTreeData func(d any) any
	toTreeData = func(d any) any {
		switch ret := d.(type) {
		case []byte:
			return codec.EncodeToHex(d)
		case []*base.NodeValue:
			nodeValue := ret
			res := []any{}
			for i := 0; i < len(nodeValue); i++ {
				nodeRes := toTreeData(nodeValue[i])
				if nodeRes == nil {
					continue
				}
				res = append(res, nodeRes)
			}
			return res
		case *base.NodeValue:
			var result any
			if ret.IsValue() {
				res := map[string]any{}
				res["leaf"] = true
				verbose := ""
				switch ret := ret.Value.(type) {
				case []byte:
					verbose = "0x" + codec.EncodeToHex(ret)
				default:
					verbose = utils.InterfaceToString(ret)
				}
				res["verbose"] = verbose
				if v, ok := ret.Origin.Cfg.GetItem(stream_parser.CfgNodeResult).([2]uint64); ok {
					res["scope"] = v
				} else {
					res["scope"] = [2]uint64{0, 0}
				}
				result = map[string]any{
					"name":  ret.Name,
					"value": res,
				}
			} else {
				result = map[string]any{
					"name":  ret.Name,
					"value": toTreeData(ret.Value),
				}
			}
			if ret.Origin.Cfg.GetBool("package-child") {
				packageRootNodes = append(packageRootNodes, result)
				return nil
			}
			return result
		default:
			return d
		}
	}
	node, err := bin_parser.ParseBinary(bytes.NewReader(data), rule)
	if err != nil {
		return nil, err
	}
	standardResult, err := node.Result()
	if err != nil {
		return nil, err
	}
	res := toTreeData(standardResult)
	_ = res
	for i, j := 0, len(packageRootNodes)-1; i < j; i, j = i+1, j-1 {
		packageRootNodes[i], packageRootNodes[j] = packageRootNodes[j], packageRootNodes[i]
	}
	return packageRootNodes, nil
}
func (s *Server) ParseTraffic(ctx context.Context, req *ypb.ParseTrafficRequest) (*ypb.ParseTrafficResponse, error) {
	rsp := &ypb.ParseTrafficResponse{}
	var payload []byte
	pagination := &ypb.Paging{
		Limit: 1,
	}
	finalResult := map[string]any{}
	switch req.GetType() {
	case "session":
		_, sessions, err := yakit.QueryTrafficSession(consts.GetGormProjectDatabase(), &ypb.QueryTrafficSessionRequest{
			Pagination: pagination,
			FromId:     req.GetId() - 1,
		})
		if err != nil {
			return nil, err
		}
		if len(sessions) != 1 {
			return nil, utils.Error("invalid session id")
		}
		//payload = sessions[0].
	case "packet":
		_, packet, err := yakit.QueryTrafficPacket(consts.GetGormProjectDatabase(), &ypb.QueryTrafficPacketRequest{
			Pagination: pagination,
			FromId:     req.GetId() - 1,
		})
		if err != nil {
			return nil, err
		}
		if len(packet) != 1 {
			return nil, utils.Error("invalid packet id")
		}
		raw, _ := strconv.Unquote(packet[0].QuotedRaw)
		packageRootNodes, err := ParseTraffic([]byte(raw), "ethernet")
		if err == nil {
			finalResult["Result"] = packageRootNodes
		}
		finalResult["RAW"] = codec.EncodeBase64(raw)
		rsp.OK = true
		resJson, err := bin_parser2.ResultToJson(finalResult)
		if err != nil {
			return nil, err
		}
		rsp.Result = resJson
		return rsp, nil
	case "reassembled":
		_, sessions, err := yakit.QueryTrafficTCPReassembled(consts.GetGormProjectDatabase(), &ypb.QueryTrafficTCPReassembledRequest{
			Pagination: pagination,
			FromId:     req.GetId() - 1,
		})
		if err != nil {
			return nil, err
		}
		if len(sessions) != 1 {
			return nil, utils.Error("invalid session id")
		}
		payload = codec.StrConvUnquoteForce(sessions[0].QuotedData)
		packageRootNodes, err := ParseReassembledTraffic([]byte(payload))
		if err == nil {
			finalResult["Result"] = packageRootNodes
		}
		finalResult["RAW"] = codec.EncodeBase64(payload)
		rsp.OK = true
		resJson, err := bin_parser2.ResultToJson(finalResult)
		if err != nil {
			return nil, err
		}
		rsp.Result = resJson
		return rsp, nil
	}
	rsp.OK = false
	return rsp, errors.New("unknown type")
}
