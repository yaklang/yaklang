package yakgrpc

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"strings"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx/pcapdb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func trafficFilterValues(value string) []string {
	values := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return r == ',' || r == '|' || r == ' ' })
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func pcapTrafficFilter(req *ypb.QueryTrafficPacketRequest) (pcapdb.PacketViewFilter, *ypb.Paging, error) {
	p := req.GetPagination()
	f := pcapdb.PacketViewFilter{ID: req.GetId(), AfterID: req.GetFromId(), Limit: 100,
		Transports: trafficFilterValues(req.GetTransportLayerTypes()), Protocols: trafficFilterValues(req.GetApplicationLayerTypes()),
		SourceIPs: trafficFilterValues(req.GetSrcIP()), DestinationIPs: trafficFilterValues(req.GetDstIP()),
		StartNS: req.StartTimestampNS, EndNS: req.EndTimestampNS, Keyword: strings.TrimSpace(req.GetKeyword())}
	invalid := func(message string) (pcapdb.PacketViewFilter, *ypb.Paging, error) {
		return f, nil, grpcstatus.Error(codes.InvalidArgument, message)
	}
	if p.GetLimit() < 0 || p.GetLimit() > 1000 || p.GetPage() < 0 || p.GetAfterId() < 0 || req.GetId() < 0 || req.GetFromId() < 0 ||
		p.GetBeforeId() != 0 || p.GetRawOrder() != "" || (p.GetOrder() != "" && !strings.EqualFold(p.GetOrder(), "asc")) || (p.GetOrderBy() != "" && p.GetOrderBy() != "id") {
		return invalid("PCAP packets require ID-ascending pagination, nonnegative IDs, and Limit <= 1000")
	}
	if req.GetSessionId() != "" || req.GetTimestampNow() != 0 {
		return invalid("PCAP mode uses DatasetID and capture timestamps, not legacy SessionId/TimestampNow")
	}
	if f.StartNS != nil && f.EndNS != nil && *f.EndNS < *f.StartNS {
		return invalid("capture time range must satisfy start <= end")
	}
	if p.GetLimit() > 0 {
		f.Limit = int(p.GetLimit())
	}
	if p.GetAfterId() > 0 {
		if f.AfterID != 0 && f.AfterID != p.GetAfterId() {
			return invalid("FromId and AfterId disagree")
		}
		f.AfterID = p.GetAfterId()
	}
	page := p.GetPage()
	if page == 0 {
		page = 1
	}
	if page > 1 && f.AfterID != 0 {
		return invalid("use Page or a packet cursor, not both")
	}
	if page > 1_000_000 {
		return invalid("page is too large; use a packet cursor")
	}
	f.Offset = (int(page) - 1) * f.Limit
	if f.ID > 0 {
		f.Limit, f.Offset = 1, 0
	}
	for _, ips := range [][]string{f.SourceIPs, f.DestinationIPs} {
		for i, ip := range ips {
			parsed := net.ParseIP(ip)
			if parsed == nil {
				return invalid("invalid packet endpoint IP")
			}
			ips[i] = parsed.String()
		}
	}
	for _, ports := range []struct {
		raw string
		dst *[]int
	}{{req.GetSrcPort(), &f.SourcePorts}, {req.GetDstPort(), &f.DestinationPorts}} {
		for _, v := range trafficFilterValues(ports.raw) {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 || n > 65535 {
				return invalid("port must be in 0..65535")
			}
			*ports.dst = append(*ports.dst, n)
		}
	}
	for _, v := range trafficFilterValues(req.GetLinkLayerTypes()) {
		var n int
		switch v {
		case "ethernet":
			n = 1
		case "raw":
			n = 101
		case "linuxsll":
			n = 113
		case "linuxsll2":
			n = 276
		case "null":
			n = 0
		default:
			var err error
			n, err = strconv.Atoi(v)
			if err != nil || n < 0 || n > 65535 {
				return invalid("unsupported link layer filter")
			}
		}
		f.LinkTypes = append(f.LinkTypes, n)
	}
	for _, v := range trafficFilterValues(req.GetNetworkLayerTypes()) {
		switch v {
		case "ipv4":
			f.IPVersions = append(f.IPVersions, 4)
		case "ipv6":
			f.IPVersions = append(f.IPVersions, 6)
		default:
			return invalid("network layer filter supports IPv4 and IPv6")
		}
	}
	return f, &ypb.Paging{Page: page, Limit: int64(f.Limit), AfterId: f.AfterID, OrderBy: "id", Order: "asc"}, nil
}

func (s *Server) queryPCAPTrafficPacket(ctx context.Context, req *ypb.QueryTrafficPacketRequest) (*ypb.QueryTrafficPacketResponse, error) {
	if err := validatePCAPDatasetID(req.GetDatasetID()); err != nil {
		return nil, err
	}
	filter, paging, err := pcapTrafficFilter(req)
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
	rsp := &ypb.QueryTrafficPacketResponse{Pagination: paging, Total: page.Total, HasMore: page.HasMore, NextCursor: filter.AfterID, Data: make([]*ypb.TrafficPacket, 0, len(page.Packets))}
	used := 128
	for _, row := range page.Packets {
		if err := ctx.Err(); err != nil {
			return nil, pcapRPCError(err)
		}
		p := row.Packet
		network := packetNetworkLayer(p.SourceIP)
		link := layers.LinkType(p.LinkType).String()
		protocol := row.Protocol
		applicationProtocol := strings.Join(row.RelatedProtocols, ", ")
		if protocol == "" {
			protocol = strings.ToUpper(p.Transport)
		}
		if protocol == "" {
			protocol = network
		}
		if protocol == "" {
			protocol = link
		}
		item := &ypb.TrafficPacket{Id: int64(p.ID), TimestampNS: p.TimestampNS, CapturedLength: int64(p.CapturedLength), OriginalLength: int64(p.OriginalLength),
			LinkType: int64(p.LinkType), Section: p.Section, Interface: int64(p.Interface), LinkLayerType: link, NetworkLayerType: network, TransportLayerType: strings.ToUpper(p.Transport),
			EthernetEndpointHardwareAddrSrc: p.SourceMAC, EthernetEndpointHardwareAddrDst: p.DestinationMAC, NetworkEndpointIPSrc: p.SourceIP, NetworkEndpointIPDst: p.DestinationIP,
			TransportEndpointPortSrc: int64(p.SourcePort), TransportEndpointPortDst: int64(p.DestinationPort), IsIpv4: network == "IPv4", IsIpv6: network == "IPv6",
			Protocol: protocol, ApplicationLayerType: applicationProtocol, Info: row.Info, DecodeError: p.DecodeError}
		var raw []byte
		// Legacy datasets predate packets.summary. Keep them readable without
		// making newly imported datasets read the raw BLOB for every list row.
		if item.Info == "" && p.CapturedLength <= pcapPacketSummaryRawBytes {
			if err := ctx.Err(); err != nil {
				return nil, pcapRPCError(err)
			}
			var readErr error
			raw, readErr = handle.ReadPacket(p.ID)
			if readErr != nil {
				return nil, pcapRPCError(readErr)
			}
			item.Info = pcapdb.SummarizePacketData(raw, p)
		}
		if req.GetFull() {
			if p.CapturedLength > pcapPacketResponseBytes-used {
				return nil, pcapRPCError(pcapdb.ErrResultTooLarge)
			}
			if raw == nil {
				item.Raw, err = handle.ReadPacket(p.ID)
				if err != nil {
					return nil, pcapRPCError(err)
				}
			} else {
				item.Raw = raw
			}
			details, err := pcapTrafficDetails(handle, p, item.Raw)
			if err != nil {
				return nil, pcapRPCError(err)
			}
			encoded, err := json.Marshal(details)
			if err != nil {
				return nil, pcapRPCError(err)
			}
			item.Details = string(encoded)
			decoded := gopacket.NewPacket(item.Raw, layers.LinkType(p.LinkType), gopacket.Default)
			if app := decoded.ApplicationLayer(); app != nil {
				item.Payload = app.Payload()
			}
		}
		size := proto.Size(item) + 16
		if used+size > pcapPacketResponseBytes {
			if len(rsp.Data) == 0 {
				return nil, pcapRPCError(pcapdb.ErrResultTooLarge)
			}
			rsp.Truncated, rsp.HasMore = true, true
			break
		}
		rsp.Data = append(rsp.Data, item)
		rsp.NextCursor = item.Id
		used += size
	}
	return rsp, nil
}

func trafficEndpoint(ip, mac string, port int) string {
	if ip == "" {
		return mac
	}
	if port == 0 {
		return ip
	}
	return net.JoinHostPort(ip, strconv.Itoa(port))
}

const pcapPacketSummaryRawBytes = 64 << 10

func packetNetworkLayer(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}
	if parsed.To4() != nil {
		return "IPv4"
	}
	return "IPv6"
}

func (s *Server) parsePCAPTraffic(ctx context.Context, req *ypb.ParseTrafficRequest) (*ypb.ParseTrafficResponse, error) {
	if req.GetType() != "packet" || req.GetId() <= 0 {
		return nil, grpcstatus.Error(codes.InvalidArgument, "PCAP details require Type=packet and an exact positive Id")
	}
	rsp, err := s.queryPCAPTrafficPacket(ctx, &ypb.QueryTrafficPacketRequest{DatasetID: req.GetDatasetID(), Id: req.GetId(), Full: true})
	if err != nil {
		return nil, err
	}
	if len(rsp.Data) != 1 {
		return nil, grpcstatus.Error(codes.NotFound, "packet not found")
	}
	var result map[string]any
	decoder := json.NewDecoder(strings.NewReader(rsp.Data[0].Details))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return nil, pcapRPCError(err)
	}
	result["RAW"] = rsp.Data[0].Raw
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, pcapRPCError(err)
	}
	return &ypb.ParseTrafficResponse{OK: true, Result: string(encoded)}, nil
}
