package yakgrpc

import (
	"context"
	"net"
	"strconv"
	"strings"

	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx/pcapdb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func pcapQueryTerms(values []string, normalize func(string) (string, error)) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if normalize != nil {
			var err error
			value, err = normalize(value)
			if err != nil {
				return nil, err
			}
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func pcapPacketQueryFilter(req *ypb.QueryPCAPPacketsRequest) (pcapdb.PacketViewFilter, *ypb.Paging, error) {
	paging, err := pcapPagination(req.GetPagination())
	if err != nil {
		return pcapdb.PacketViewFilter{}, nil, err
	}
	filter := pcapdb.PacketViewFilter{Limit: int(paging.GetLimit()), AfterID: paging.GetAfterId()}
	f := req.GetFilter()
	if f.GetId() < 0 {
		return filter, nil, grpcstatus.Error(codes.InvalidArgument, "packet Id cannot be negative")
	}
	filter.ID = f.GetId()

	lower := func(value string) (string, error) { return strings.ToLower(value), nil }
	if filter.SourceIPs, err = pcapQueryTerms(f.GetSourceIP(), func(value string) (string, error) {
		ip := net.ParseIP(value)
		if ip == nil {
			return "", grpcstatus.Error(codes.InvalidArgument, "SourceIP must contain individual IPv4 or IPv6 addresses")
		}
		return ip.String(), nil
	}); err != nil {
		return filter, nil, err
	}
	if filter.DestinationIPs, err = pcapQueryTerms(f.GetDestinationIP(), func(value string) (string, error) {
		ip := net.ParseIP(value)
		if ip == nil {
			return "", grpcstatus.Error(codes.InvalidArgument, "DestinationIP must contain individual IPv4 or IPv6 addresses")
		}
		return ip.String(), nil
	}); err != nil {
		return filter, nil, err
	}
	for _, ports := range []struct {
		values []int32
		dst    *[]int
	}{
		{f.GetSourcePort(), &filter.SourcePorts},
		{f.GetDestinationPort(), &filter.DestinationPorts},
	} {
		seen := make(map[int]struct{}, len(ports.values))
		for _, port := range ports.values {
			if port < 0 || port > 65535 {
				return filter, nil, grpcstatus.Error(codes.InvalidArgument, "packet ports must be in 0..65535")
			}
			if _, ok := seen[int(port)]; ok {
				continue
			}
			seen[int(port)] = struct{}{}
			*ports.dst = append(*ports.dst, int(port))
		}
	}
	if filter.Transports, err = pcapQueryTerms(f.GetTransportLayerTypes(), lower); err != nil {
		return filter, nil, err
	}
	if filter.Protocols, err = pcapQueryTerms(f.GetApplicationLayerTypes(), lower); err != nil {
		return filter, nil, err
	}
	for _, value := range f.GetNetworkLayerTypes() {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "":
		case "ipv4", "4":
			filter.IPVersions = append(filter.IPVersions, 4)
		case "ipv6", "6":
			filter.IPVersions = append(filter.IPVersions, 6)
		default:
			return filter, nil, grpcstatus.Error(codes.InvalidArgument, "NetworkLayerTypes supports IPv4 and IPv6")
		}
	}
	seenVersions := make(map[int]struct{}, len(filter.IPVersions))
	versions := filter.IPVersions[:0]
	for _, version := range filter.IPVersions {
		if _, ok := seenVersions[version]; ok {
			continue
		}
		seenVersions[version] = struct{}{}
		versions = append(versions, version)
	}
	filter.IPVersions = versions
	for _, value := range f.GetLinkLayerTypes() {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		linkType := -1
		switch value {
		case "null":
			linkType = 0
		case "ethernet":
			linkType = int(layers.LinkTypeEthernet)
		case "raw":
			linkType = int(layers.LinkTypeRaw)
		case "linuxsll":
			linkType = int(layers.LinkTypeLinuxSLL)
		case "linuxsll2":
			linkType = int(layers.LinkTypeLinuxSLL2)
		default:
			parsed, parseErr := strconv.Atoi(value)
			if parseErr != nil || parsed < 0 || parsed > 65535 {
				return filter, nil, grpcstatus.Error(codes.InvalidArgument, "LinkLayerTypes contains an unsupported link type")
			}
			linkType = parsed
		}
		filter.LinkTypes = append(filter.LinkTypes, linkType)
	}
	if f != nil {
		if f.StartTimestampNS != nil {
			filter.StartNS = proto.Int64(f.GetStartTimestampNS())
		}
		if f.EndTimestampNS != nil {
			filter.EndNS = proto.Int64(f.GetEndTimestampNS())
		}
	}
	if filter.StartNS != nil && filter.EndNS != nil && *filter.EndNS < *filter.StartNS {
		return filter, nil, grpcstatus.Error(codes.InvalidArgument, "capture time range must satisfy start <= end")
	}
	filter.Keyword = strings.TrimSpace(f.GetKeyword())
	if req.IncludeTotal != nil {
		filter.SkipTotal = !req.GetIncludeTotal()
		filter.ForceTotal = req.GetIncludeTotal()
	} else {
		filter.SkipTotal = filter.AfterID != 0
	}
	return filter, paging, nil
}

func pcapPacketSummaryModel(row pcapdb.PacketView) *ypb.PCAPPacketSummary {
	packet := row.Packet
	protocol := row.Protocol
	if protocol == "" {
		protocol = strings.ToUpper(packet.Transport)
	}
	if protocol == "" {
		protocol = packetNetworkLayer(packet.SourceIP)
	}
	if protocol == "" {
		protocol = layers.LinkType(packet.LinkType).String()
	}
	return &ypb.PCAPPacketSummary{
		Id:                   int64(packet.ID),
		TimestampNS:          packet.TimestampNS,
		CapturedLength:       int64(packet.CapturedLength),
		OriginalLength:       int64(packet.OriginalLength),
		LinkType:             int64(packet.LinkType),
		Section:              packet.Section,
		Interface:            int64(packet.Interface),
		SourceMAC:            packet.SourceMAC,
		DestinationMAC:       packet.DestinationMAC,
		VLANStack:            packet.VLANStack,
		SourceIP:             packet.SourceIP,
		DestinationIP:        packet.DestinationIP,
		SourcePort:           int32(packet.SourcePort),
		DestinationPort:      int32(packet.DestinationPort),
		Transport:            packet.Transport,
		Protocol:             protocol,
		ApplicationProtocols: append([]string(nil), row.RelatedProtocols...),
		Info:                 row.Info,
		DecodeError:          packet.DecodeError,
	}
}

func (s *Server) QueryPCAPPackets(ctx context.Context, req *ypb.QueryPCAPPacketsRequest) (*ypb.QueryPCAPPacketsResponse, error) {
	if err := validatePCAPDatasetID(req.GetDatasetID()); err != nil {
		return nil, err
	}
	filter, paging, err := pcapPacketQueryFilter(req)
	if err != nil {
		return nil, err
	}
	manager, err := s.getPCAPManager()
	if err != nil {
		return nil, pcapRPCError(err)
	}
	handle, err := manager.Acquire(ctx, req.GetDatasetID())
	if err != nil {
		return nil, pcapRPCError(err)
	}
	defer handle.Close()
	page, err := handle.QueryPacketView(filter)
	if err != nil {
		return nil, pcapRPCError(err)
	}
	paging.AfterId = page.NextCursor
	response := &ypb.QueryPCAPPacketsResponse{
		Pagination: paging,
		Data:       make([]*ypb.PCAPPacketSummary, 0, len(page.Packets)),
	}
	if page.TotalKnown {
		response.Total = proto.Int64(page.Total)
	}
	for _, row := range page.Packets {
		response.Data = append(response.Data, pcapPacketSummaryModel(row))
	}
	return response, nil
}
