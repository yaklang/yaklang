package yakgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcapdb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestDeprecatedTrafficRPCs(t *testing.T) {
	_, client := newPCAPTestClient(t)
	_, err := client.QueryTrafficPacket(context.Background(), &ypb.QueryTrafficPacketRequest{})
	require.Equal(t, codes.InvalidArgument, grpcstatus.Code(err))
	_, err = client.ParseTraffic(context.Background(), &ypb.ParseTrafficRequest{Type: "packet", Id: 1})
	require.Equal(t, codes.InvalidArgument, grpcstatus.Code(err))
	_, err = client.QueryTrafficSession(context.Background(), &ypb.QueryTrafficSessionRequest{})
	require.Equal(t, codes.Unimplemented, grpcstatus.Code(err))
	_, err = client.QueryTrafficTCPReassembled(context.Background(), &ypb.QueryTrafficTCPReassembledRequest{})
	require.Equal(t, codes.Unimplemented, grpcstatus.Code(err))
}

func TestPCAPGRPCLegacyPacketQuery(t *testing.T) {
	_, client := newPCAPTestClient(t)
	path, _, raw := pcapTestCapture(t, 5)
	events := importPCAPForTest(t, client, path)
	id := events[len(events)-1].GetDatasetID()
	ctx := context.Background()
	first, err := client.QueryTrafficPacket(ctx, &ypb.QueryTrafficPacketRequest{DatasetID: id, Pagination: &ypb.Paging{Limit: 2}})
	require.NoError(t, err)
	require.Len(t, first.Data, 2)
	require.EqualValues(t, 5, first.Total)
	require.EqualValues(t, 2, first.NextCursor)
	require.True(t, first.HasMore)
	for _, p := range first.Data {
		require.Empty(t, p.Raw)
		require.Empty(t, p.Payload)
		require.Empty(t, p.Details)
		require.NotEmpty(t, p.Protocol)
		require.NotEmpty(t, p.Info)
		require.NotContains(t, p.Info, "messages")
		require.NotNil(t, p.TimestampNS)
		require.EqualValues(t, len(raw[0]), p.CapturedLength)
	}
	second, err := client.QueryTrafficPacket(ctx, &ypb.QueryTrafficPacketRequest{DatasetID: id, FromId: first.NextCursor, Pagination: &ypb.Paging{Limit: 2}})
	require.NoError(t, err)
	require.EqualValues(t, 3, second.Data[0].Id)
	require.Zero(t, second.Total, "cursor pages do not repeat the filtered count")
	numbered, err := client.QueryTrafficPacket(ctx, &ypb.QueryTrafficPacketRequest{DatasetID: id, Pagination: &ypb.Paging{Limit: 2, Page: 2}})
	require.NoError(t, err)
	require.Equal(t, second.Data, numbered.Data)
	filtered, err := client.QueryTrafficPacket(ctx, &ypb.QueryTrafficPacketRequest{DatasetID: id, SrcIP: "10.0.0.2", DstIP: "8.8.8.8", SrcPort: "1", DstPort: "53", TransportLayerTypes: "UDP", NetworkLayerTypes: "IPv4", LinkLayerTypes: "Ethernet"})
	require.NoError(t, err)
	require.Len(t, filtered.Data, 1)
	require.EqualValues(t, 1, filtered.Total)
	require.EqualValues(t, 2, filtered.Data[0].Id)
	require.Contains(t, filtered.Data[0].Info, "SrcPort=1 DstPort=53")
	zeroPort, err := client.QueryTrafficPacket(ctx, &ypb.QueryTrafficPacketRequest{DatasetID: id, SrcPort: "0"})
	require.NoError(t, err)
	require.Len(t, zeroPort.Data, 1)
	require.EqualValues(t, 1, zeroPort.Data[0].Id)
	timed, err := client.QueryTrafficPacket(ctx, &ypb.QueryTrafficPacketRequest{DatasetID: id, StartTimestampNS: proto.Int64(1700000001123456789), EndTimestampNS: proto.Int64(1700000002123456789)})
	require.NoError(t, err)
	require.Len(t, timed.Data, 1)
	require.EqualValues(t, 2, timed.Data[0].Id)
	detail, err := client.QueryTrafficPacket(ctx, &ypb.QueryTrafficPacketRequest{DatasetID: id, Id: 2, Full: true})
	require.NoError(t, err)
	require.Len(t, detail.Data, 1)
	require.Equal(t, raw[1], detail.Data[0].Raw)
	var parsed trafficPacketDetails
	require.NoError(t, json.Unmarshal([]byte(detail.Data[0].Details), &parsed))
	require.GreaterOrEqual(t, len(parsed.Result), 4)
	missing, err := client.QueryTrafficPacket(ctx, &ypb.QueryTrafficPacketRequest{DatasetID: id, Id: 99, Full: true})
	require.NoError(t, err)
	require.Empty(t, missing.Data)
	require.Zero(t, missing.Total)
	compat, err := client.ParseTraffic(ctx, &ypb.ParseTrafficRequest{DatasetID: id, Id: 2, Type: "packet"})
	require.NoError(t, err)
	require.True(t, compat.OK)
	var envelope struct {
		RAW    []byte
		Result []trafficField
	}
	require.NoError(t, json.Unmarshal([]byte(compat.Result), &envelope))
	require.Equal(t, raw[1], envelope.RAW)
	require.Len(t, envelope.Result, len(parsed.Result))
	_, err = client.ParseTraffic(ctx, &ypb.ParseTrafficRequest{DatasetID: id, Id: 99, Type: "packet"})
	require.Equal(t, codes.NotFound, grpcstatus.Code(err))
}

func TestPCAPGRPCLegacyPacketValidation(t *testing.T) {
	_, client := newPCAPTestClient(t)
	for _, req := range []*ypb.QueryTrafficPacketRequest{
		{DatasetID: "../bad"}, {DatasetID: "00000000-0000-0000-0000-000000000000", SrcPort: "-1"},
		{DatasetID: "00000000-0000-0000-0000-000000000000", SrcIP: "x' OR 1=1"},
		{DatasetID: "00000000-0000-0000-0000-000000000000", Pagination: &ypb.Paging{Order: "desc"}},
		{DatasetID: "00000000-0000-0000-0000-000000000000", FromId: 2, Pagination: &ypb.Paging{Page: 2}},
		{DatasetID: "00000000-0000-0000-0000-000000000000", TimestampNow: 123},
	} {
		_, err := client.QueryTrafficPacket(context.Background(), req)
		require.Equal(t, codes.InvalidArgument, grpcstatus.Code(err))
	}
}

func pcapHTTPViewCapture(t *testing.T) (string, [][]byte, []byte) {
	t.Helper()
	request := []byte("GET /demo HTTP/1.1\r\nHost: example.test\r\n\r\n")
	response := []byte("HTTP/1.1 200 OK\r\nContent-Length: 11\r\n\r\nhello world")
	type frame struct {
		reverse, syn, ack, fin bool
		seq, ackNo             uint32
		payload                []byte
	}
	frames := []frame{
		{syn: true, seq: 1000}, {reverse: true, syn: true, ack: true, seq: 2000, ackNo: 1001}, {ack: true, seq: 1001, ackNo: 2001},
		{ack: true, seq: 1001, ackNo: 2001, payload: request},
		{reverse: true, ack: true, seq: 2001, ackNo: 1001 + uint32(len(request)), payload: response[:30]},
		{reverse: true, ack: true, seq: 2031, ackNo: 1001 + uint32(len(request)), payload: response[30:]},
		{reverse: true, ack: true, fin: true, seq: 2001 + uint32(len(response)), ackNo: 1001 + uint32(len(request))},
	}
	var capture bytes.Buffer
	w := pcapgo.NewWriterNanos(&capture)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
	var raws [][]byte
	for i, f := range frames {
		eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{1, 2, 3, 4, 5, 6}, DstMAC: net.HardwareAddr{6, 5, 4, 3, 2, 1}, EthernetType: layers.EthernetTypeIPv4}
		ip := &layers.IPv4{Version: 4, TTL: 64, SrcIP: net.IPv4(10, 0, 0, 1), DstIP: net.IPv4(8, 8, 8, 8), Protocol: layers.IPProtocolTCP}
		tcp := &layers.TCP{SrcPort: 12345, DstPort: 80, Seq: f.seq, Ack: f.ackNo, SYN: f.syn, ACK: f.ack, FIN: f.fin, Window: 65535}
		if f.reverse {
			ip.SrcIP, ip.DstIP = ip.DstIP, ip.SrcIP
			tcp.SrcPort, tcp.DstPort = tcp.DstPort, tcp.SrcPort
		}
		require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
		buf := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp, gopacket.Payload(f.payload)))
		raw := append([]byte(nil), buf.Bytes()...)
		raws = append(raws, raw)
		require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000+int64(i), 0), CaptureLength: len(raw), Length: len(raw)}, raw))
	}
	path := filepath.Join(t.TempDir(), "http.pcap")
	require.NoError(t, os.WriteFile(path, capture.Bytes(), 0600))
	return path, raws, response
}

func TestPCAPGRPCLegacyPacketReassembly(t *testing.T) {
	_, client := newPCAPTestClient(t)
	path, raws, response := pcapHTTPViewCapture(t)
	events := importPCAPForTest(t, client, path)
	id := events[len(events)-1].GetDatasetID()
	ctx := context.Background()
	all, err := client.QueryTrafficPacket(ctx, &ypb.QueryTrafficPacketRequest{DatasetID: id})
	require.NoError(t, err)
	require.Len(t, all.Data, len(raws))
	require.EqualValues(t, len(raws), all.Total)
	require.Equal(t, "[SYN] Seq=1000 Ack=0 Win=65535 Len=0", all.Data[0].Info)
	require.Equal(t, "[SYN, ACK] Seq=2000 Ack=1001 Win=65535 Len=0", all.Data[1].Info)
	require.Equal(t, "[ACK] Seq=1001 Ack=2001 Win=65535 Len=0", all.Data[2].Info)
	require.Equal(t, "TCP", all.Data[0].Protocol)
	require.Contains(t, all.Data[3].Info, "GET /demo")
	require.Equal(t, "HTTP", all.Data[3].Protocol)
	require.Equal(t, "HTTP", all.Data[3].ApplicationLayerType)
	require.Equal(t, "TCP", all.Data[3].TransportLayerType)
	require.Contains(t, all.Data[4].Info, "[ACK] Seq=2001")
	require.Contains(t, all.Data[4].Info, "Len=30")
	require.NotContains(t, all.Data[4].Info, "HTTP segment")
	require.Equal(t, "TCP", all.Data[4].Protocol)
	require.Empty(t, all.Data[4].ApplicationLayerType)
	require.Equal(t, "HTTP", all.Data[5].Protocol)
	require.Equal(t, "HTTP/1.1 200 OK", all.Data[5].Info)
	require.Equal(t, "HTTP", all.Data[5].ApplicationLayerType)
	http, err := client.QueryTrafficPacket(ctx, &ypb.QueryTrafficPacketRequest{DatasetID: id, ApplicationLayerTypes: "http", Pagination: &ypb.Paging{Limit: 1}})
	require.NoError(t, err)
	require.EqualValues(t, 2, http.Total)
	require.Len(t, http.Data, 1)
	require.EqualValues(t, 4, http.Data[0].Id)
	require.Equal(t, "HTTP", http.Data[0].Protocol)
	require.Equal(t, "HTTP", http.Data[0].ApplicationLayerType)
	require.True(t, http.HasMore)
	httpNext, err := client.QueryTrafficPacket(ctx, &ypb.QueryTrafficPacketRequest{DatasetID: id, ApplicationLayerTypes: "http", FromId: http.NextCursor, Pagination: &ypb.Paging{Limit: 1}})
	require.NoError(t, err)
	require.Zero(t, httpNext.Total, "cursor pages do not repeat the filtered count")
	require.Len(t, httpNext.Data, 1)
	require.EqualValues(t, 6, httpNext.Data[0].Id)
	require.Equal(t, "HTTP", httpNext.Data[0].Protocol)
	require.False(t, httpNext.HasMore)
	for _, packetID := range []int64{5, 6} {
		detail, err := client.QueryTrafficPacket(ctx, &ypb.QueryTrafficPacketRequest{DatasetID: id, Id: packetID, Full: true})
		require.NoError(t, err)
		require.Len(t, detail.Data, 1)
		require.Equal(t, raws[packetID-1], detail.Data[0].Raw)
		var parsed trafficPacketDetails
		require.NoError(t, json.Unmarshal([]byte(detail.Data[0].Details), &parsed))
		require.True(t, parsed.ProtocolsIndexed)
		require.NotEmpty(t, parsed.Messages)
		var found bool
		for _, m := range parsed.Messages {
			require.Equal(t, len(m.RAW), m.DataLength)
			if m.Protocol == "HTTP" && bytes.Equal(m.RAW, response) {
				found = true
				require.Equal(t, []uint64{5, 6}, m.PacketIds)
				require.EqualValues(t, 6, m.CompletedInPacketId)
				require.NotEmpty(t, m.Result)
			}
		}
		require.True(t, found, "both contributor and completion frames must link to the saved complete HTTP response")
		if packetID == 5 {
			require.Equal(t, "TCP", detail.Data[0].Protocol)
			require.NotContains(t, detail.Data[0].Info, "HTTP segment")
			require.Contains(t, detail.Data[0].Info, "Seq=")
			require.Empty(t, detail.Data[0].ApplicationLayerType)
		} else {
			require.Equal(t, "HTTP", detail.Data[0].Protocol)
			require.Equal(t, "HTTP/1.1 200 OK", detail.Data[0].Info)
			require.Equal(t, "HTTP", detail.Data[0].ApplicationLayerType)
		}
		require.Equal(t, "TCP", detail.Data[0].TransportLayerType)
	}
}

func TestPCAPGRPCLegacyPacketReindex(t *testing.T) {
	s, client := newPCAPTestClient(t)
	path, _, _ := pcapHTTPViewCapture(t)
	manager, err := s.getPCAPManager()
	require.NoError(t, err)
	handle, err := manager.GetOrCreateHandle(context.Background(), path, pcapdb.WithProtocols(false), pcapdb.WithStreams(false))
	require.NoError(t, err)
	id := handle.ID
	require.NoError(t, handle.Close())
	old, err := client.QueryTrafficPacket(context.Background(), &ypb.QueryTrafficPacketRequest{DatasetID: id, Id: 6, Full: true})
	require.NoError(t, err)
	var parsed trafficPacketDetails
	require.NoError(t, json.Unmarshal([]byte(old.Data[0].Details), &parsed))
	require.False(t, parsed.ProtocolsIndexed)
	require.Empty(t, parsed.Messages)
	_, err = client.QueryTrafficPacket(context.Background(), &ypb.QueryTrafficPacketRequest{DatasetID: id, ApplicationLayerTypes: "HTTP"})
	require.Equal(t, codes.FailedPrecondition, grpcstatus.Code(err))
	indexed := importPCAPForTest(t, client, path)
	require.Equal(t, id, indexed[len(indexed)-1].DatasetID)
	fresh, err := client.QueryTrafficPacket(context.Background(), &ypb.QueryTrafficPacketRequest{DatasetID: id, Id: 6, Full: true})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(fresh.Data[0].Details), &parsed))
	require.True(t, parsed.ProtocolsIndexed)
	require.NotEmpty(t, parsed.Messages)
}
