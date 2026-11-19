package yakgrpc

import (
	"bytes"
	"context"
	"errors"
	"github.com/samber/lo"
	"github.com/yaklang/pcap"
	bin_parser "github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcapdb"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"runtime"
	"strings"
	"time"
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
	}

	var defaultIfName *ypb.NetInterface
	for _, ifItem := range ifs {
		if ifIns != nil && ifItem.NetInterfaceName == ifIns.Name {
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

type pcapXCaptureResult struct {
	metadata *pcapdb.PCAPFileDBMetadata
	err      error
}

type pcapXCaptureRunner func(context.Context, *pcapdb.InstanceManager, pcapdb.LiveCaptureConfig, func(pcapdb.PCAPFileDBMetadata)) (*pcapdb.PCAPFileDBMetadata, error)

var pcapXRunLiveCapture pcapXCaptureRunner = func(ctx context.Context, manager *pcapdb.InstanceManager, config pcapdb.LiveCaptureConfig, started func(pcapdb.PCAPFileDBMetadata)) (*pcapdb.PCAPFileDBMetadata, error) {
	return manager.RunLiveCapture(ctx, config, started)
}

func pcapXLiveCaptureConfig(req *ypb.PcapXRequest) (pcapdb.LiveCaptureConfig, error) {
	interfaces := utils.StringArrayFilterEmpty(req.GetNetInterfaceList())
	if len(interfaces) != 1 {
		return pcapdb.LiveCaptureConfig{}, grpcstatus.Error(codes.InvalidArgument, "PcapX requires exactly one capture interface")
	}
	if req.GetSnaplen() < 0 || req.GetSnaplen() > 16<<20 {
		return pcapdb.LiveCaptureConfig{}, grpcstatus.Error(codes.InvalidArgument, "Snaplen must be between 0 and 16777216")
	}
	if req.GetBatchSize() < 0 || req.GetBatchSize() > 100000 {
		return pcapdb.LiveCaptureConfig{}, grpcstatus.Error(codes.InvalidArgument, "BatchSize must be between 0 and 100000")
	}
	if req.GetFlushIntervalMilliseconds() < 0 || req.GetDurationMilliseconds() < 0 {
		return pcapdb.LiveCaptureConfig{}, grpcstatus.Error(codes.InvalidArgument, "capture intervals cannot be negative")
	}
	if req.GetDurationMilliseconds() > int64((1<<63-1)/time.Millisecond) {
		return pcapdb.LiveCaptureConfig{}, grpcstatus.Error(codes.InvalidArgument, "DurationMilliseconds is too large")
	}
	return pcapdb.LiveCaptureConfig{
		Interface:     strings.TrimSpace(interfaces[0]),
		BPF:           strings.TrimSpace(req.GetBPF()),
		Snaplen:       int(req.GetSnaplen()),
		Promisc:       req.GetPromisc(),
		Duration:      time.Duration(req.GetDurationMilliseconds()) * time.Millisecond,
		FlushInterval: time.Duration(req.GetFlushIntervalMilliseconds()) * time.Millisecond,
		BatchSize:     int(req.GetBatchSize()),
	}, nil
}

func pcapXTransferKBPerSecond(deltaBytes int64, elapsed time.Duration) int64 {
	if deltaBytes <= 0 || elapsed <= 0 {
		return 0
	}
	return deltaBytes * int64(time.Second) / elapsed.Nanoseconds() / 1024
}

func pcapXResponse(meta *pcapdb.PCAPFileDBMetadata, finished bool, captureErr error, transferKBPerSecond int64) *ypb.PcapXResponse {
	response := &ypb.PcapXResponse{Finished: finished, NetInterfaceTransferKBPerSeconds: transferKBPerSecond}
	if meta != nil {
		response.DatasetID = meta.DatasetID
		response.State = string(meta.State)
		response.PacketFrameCounter = meta.PacketCount
		response.TCPReassembledCount = meta.StreamChunkCount
		response.TrafficSessionCount = meta.SessionCount
		response.BytesIndexed = meta.BytesIndexed
		response.TotalBytes = meta.SourceSize
		response.ProtocolCount = meta.ProtocolCount
		response.SessionCount = meta.SessionCount
		response.StreamCount = meta.StreamCount
		response.StreamChunkCount = meta.StreamChunkCount
	}
	if captureErr != nil {
		response.Error = captureErr.Error()
	}
	return response
}

func (s *Server) PcapX(stream ypb.Yak_PcapXServer) error {
	firstReq, err := stream.Recv()
	if err != nil {
		return err
	}
	config, err := pcapXLiveCaptureConfig(firstReq)
	if err != nil {
		return err
	}
	manager, err := s.getPCAPManager()
	if err != nil {
		return pcapRPCError(err)
	}

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	started := make(chan pcapdb.PCAPFileDBMetadata, 1)
	done := make(chan pcapXCaptureResult, 1)
	go func() {
		meta, runErr := pcapXRunLiveCapture(ctx, manager, config, func(meta pcapdb.PCAPFileDBMetadata) {
			select {
			case started <- meta:
			default:
			}
		})
		done <- pcapXCaptureResult{metadata: meta, err: runErr}
	}()

	var current *pcapdb.PCAPFileDBMetadata
	select {
	case meta := <-started:
		current = &meta
	case result := <-done:
		if result.metadata == nil {
			return pcapRPCError(result.err)
		}
		current = result.metadata
		sendErr := stream.Send(pcapXResponse(current, true, result.err, 0))
		if sendErr != nil {
			return sendErr
		}
		if result.err != nil {
			return pcapRPCError(result.err)
		}
		return nil
	case <-ctx.Done():
		result := <-done
		if result.err != nil && !errors.Is(result.err, context.Canceled) {
			return pcapRPCError(result.err)
		}
		return nil
	}

	if err = stream.Send(pcapXResponse(current, false, nil, 0)); err != nil {
		cancel()
		<-done
		return err
	}
	lastBytes, lastAt := current.BytesIndexed, time.Now()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case result := <-done:
			var transfer int64
			if result.metadata != nil {
				transfer = pcapXTransferKBPerSecond(result.metadata.BytesIndexed-lastBytes, time.Since(lastAt))
			}
			if sendErr := stream.Send(pcapXResponse(result.metadata, true, result.err, transfer)); sendErr != nil {
				return sendErr
			}
			if result.err != nil {
				return pcapRPCError(result.err)
			}
			return nil
		case now := <-ticker.C:
			meta, metadataErr := manager.Metadata(current.DatasetID)
			if metadataErr != nil {
				continue
			}
			transfer := pcapXTransferKBPerSecond(meta.BytesIndexed-lastBytes, now.Sub(lastAt))
			lastBytes, lastAt = meta.BytesIndexed, now
			if sendErr := stream.Send(pcapXResponse(meta, false, nil, transfer)); sendErr != nil {
				cancel()
				<-done
				return sendErr
			}
		case <-ctx.Done():
			cancel()
			result := <-done
			if result.err != nil && !errors.Is(result.err, context.Canceled) {
				return pcapRPCError(result.err)
			}
			return nil
		}
	}
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
	if strings.TrimSpace(req.GetDatasetID()) == "" {
		return nil, grpcstatus.Error(codes.InvalidArgument, "ParseTraffic is deprecated; DatasetID is required, use GetPCAPPacketDetail")
	}
	return s.parsePCAPTraffic(ctx, req)
}
