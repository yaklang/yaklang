package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
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
)

func pcapHTTPLargeResponseCapture(t *testing.T, bodySize int) (string, []byte) {
	t.Helper()
	request := []byte("GET /large HTTP/1.1\r\nHost: example.test\r\n\r\n")
	response := append([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n", bodySize)), bytes.Repeat([]byte("x"), bodySize)...)
	type frame struct {
		reverse, syn, ack, fin bool
		seq, ackNo             uint32
		payload                []byte
	}
	frames := []frame{
		{syn: true, seq: 1000},
		{reverse: true, syn: true, ack: true, seq: 2000, ackNo: 1001},
		{ack: true, seq: 1001, ackNo: 2001},
		{ack: true, seq: 1001, ackNo: 2001, payload: request},
	}
	const chunkSize = 1200
	for offset := 0; offset < len(response); offset += chunkSize {
		end := offset + chunkSize
		if end > len(response) {
			end = len(response)
		}
		frames = append(frames, frame{
			reverse: true, ack: true, seq: 2001 + uint32(offset),
			ackNo: 1001 + uint32(len(request)), payload: response[offset:end],
		})
	}
	frames = append(frames, frame{
		reverse: true, ack: true, fin: true, seq: 2001 + uint32(len(response)),
		ackNo: 1001 + uint32(len(request)),
	})
	var capture bytes.Buffer
	writer := pcapgo.NewWriterNanos(&capture)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	for i, frame := range frames {
		ethernet := &layers.Ethernet{
			SrcMAC: net.HardwareAddr{1, 2, 3, 4, 5, 6}, DstMAC: net.HardwareAddr{6, 5, 4, 3, 2, 1},
			EthernetType: layers.EthernetTypeIPv4,
		}
		ip := &layers.IPv4{Version: 4, TTL: 64, SrcIP: net.IPv4(10, 0, 0, 1), DstIP: net.IPv4(8, 8, 8, 8), Protocol: layers.IPProtocolTCP}
		tcp := &layers.TCP{SrcPort: 12345, DstPort: 80, Seq: frame.seq, Ack: frame.ackNo, SYN: frame.syn, ACK: frame.ack, FIN: frame.fin, Window: 65535}
		if frame.reverse {
			ip.SrcIP, ip.DstIP = ip.DstIP, ip.SrcIP
			tcp.SrcPort, tcp.DstPort = tcp.DstPort, tcp.SrcPort
		}
		require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
		buffer := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true},
			ethernet, ip, tcp, gopacket.Payload(frame.payload)))
		raw := append([]byte(nil), buffer.Bytes()...)
		require.NoError(t, writer.WritePacket(gopacket.CaptureInfo{
			Timestamp: time.Unix(1700000000+int64(i), 0), CaptureLength: len(raw), Length: len(raw),
		}, raw))
	}
	path := filepath.Join(t.TempDir(), "large-http.pcap")
	require.NoError(t, os.WriteFile(path, capture.Bytes(), 0600))
	return path, response
}

func findPCAPFieldNode(nodes []*ypb.PCAPFieldNode, name string) *ypb.PCAPFieldNode {
	for _, node := range nodes {
		if node.GetName() == name {
			return node
		}
		if found := findPCAPFieldNode(node.GetChildren(), name); found != nil {
			return found
		}
	}
	return nil
}

func TestPCAPGRPCGetPacketDetailHTTPReassembly(t *testing.T) {
	_, client := newPCAPTestClient(t)
	path, _, response := pcapHTTPViewCapture(t)
	events := importPCAPForTest(t, client, path)
	datasetID := events[len(events)-1].GetDatasetID()
	ctx := context.Background()

	detail, err := client.GetPCAPPacketDetail(ctx, &ypb.GetPCAPPacketDetailRequest{DatasetID: datasetID, PacketID: 6})
	require.NoError(t, err)
	require.EqualValues(t, 6, detail.GetPacket().GetId())
	require.Equal(t, "HTTP", detail.GetPacket().GetProtocol())
	var frameRoot *ypb.PCAPFieldNode
	for _, node := range detail.GetNodes() {
		if strings.HasPrefix(node.GetName(), "Frame 6:") {
			frameRoot = node
			break
		}
	}
	require.NotNil(t, frameRoot)
	sequence := findPCAPFieldNode(detail.GetNodes(), "Sequence number")
	require.NotNil(t, sequence)
	require.NotEmpty(t, sequence.GetRanges())

	require.Len(t, detail.GetMessages(), 1)
	message := detail.GetMessages()[0]
	require.Equal(t, "HTTP", message.GetProtocol())
	require.Contains(t, message.GetSummary(), "HTTP/1.1 200 OK")
	require.True(t, strings.HasPrefix(message.GetSourceID(), "message:"))
	var messageSource *ypb.PCAPByteSource
	for _, source := range detail.GetByteSources() {
		if source.GetID() == message.GetSourceID() {
			messageSource = source
			break
		}
	}
	require.NotNil(t, messageSource)
	require.NotNil(t, findPCAPFieldNode(message.GetNodes(), "HTTP"))
	status := findPCAPFieldNode(message.GetNodes(), "Status Code")
	require.NotNil(t, status)
	require.Len(t, status.GetRanges(), 1)
	require.Equal(t, message.GetSourceID(), status.GetRanges()[0].GetSourceID())

	var frameSource *ypb.PCAPByteSource
	for _, source := range detail.GetByteSources() {
		switch source.GetID() {
		case "frame:6":
			frameSource = source
		}
	}
	require.NotNil(t, messageSource)
	require.NotNil(t, frameSource)
	require.Equal(t, []uint64{5, 6}, messageSource.GetContributorPacketIds())
	require.EqualValues(t, 6, messageSource.GetCompletedInPacketId())
	require.Equal(t, response, messageSource.GetInitialData())
	require.False(t, messageSource.GetInitialTruncated())

	rangeValue, err := client.ReadPCAPBytes(ctx, &ypb.ReadPCAPBytesRequest{
		DatasetID: datasetID, SourceID: status.GetRanges()[0].GetSourceID(),
		Offset: status.GetRanges()[0].GetBitOffset() / 8,
		Length: status.GetRanges()[0].GetBitLength() / 8,
	})
	require.NoError(t, err)
	require.Equal(t, "200", string(rangeValue.GetData()))

	contributor, err := client.GetPCAPPacketDetail(ctx, &ypb.GetPCAPPacketDetailRequest{DatasetID: datasetID, PacketID: 5})
	require.NoError(t, err)
	require.Empty(t, contributor.GetMessages())
	require.Len(t, contributor.GetRelatedMessages(), 1)
	require.Equal(t, message.GetMessageId(), contributor.GetRelatedMessages()[0].GetMessageId())
	require.EqualValues(t, 6, contributor.GetRelatedMessages()[0].GetCompletedInPacketId())
	require.Equal(t, "frame:5", contributor.GetByteSources()[0].GetID())
}

func TestPCAPGRPCGetPacketDetailValidation(t *testing.T) {
	_, client := newPCAPTestClient(t)
	path, _, _ := pcapTestCapture(t, 1)
	events := importPCAPForTest(t, client, path)
	datasetID := events[len(events)-1].GetDatasetID()
	for _, tc := range []struct {
		req  *ypb.GetPCAPPacketDetailRequest
		code codes.Code
	}{
		{&ypb.GetPCAPPacketDetailRequest{DatasetID: datasetID}, codes.InvalidArgument},
		{&ypb.GetPCAPPacketDetailRequest{DatasetID: datasetID, PacketID: -1}, codes.InvalidArgument},
		{&ypb.GetPCAPPacketDetailRequest{DatasetID: datasetID, PacketID: 99}, codes.NotFound},
		{&ypb.GetPCAPPacketDetailRequest{DatasetID: "invalid", PacketID: 1}, codes.InvalidArgument},
	} {
		_, err := client.GetPCAPPacketDetail(context.Background(), tc.req)
		require.Equal(t, tc.code, grpcstatus.Code(err), tc.req.String())
	}
}

func TestPCAPGRPCGetPacketDetailLargeMessage(t *testing.T) {
	_, client := newPCAPTestClient(t)
	path, response := pcapHTTPLargeResponseCapture(t, 180_000)
	events := importPCAPForTest(t, client, path)
	datasetID := events[len(events)-1].GetDatasetID()
	completionID := events[len(events)-1].GetPacketCount() - 1
	detail, err := client.GetPCAPPacketDetail(context.Background(), &ypb.GetPCAPPacketDetailRequest{
		DatasetID: datasetID, PacketID: completionID,
	})
	require.NoError(t, err)
	require.Len(t, detail.GetMessages(), 1)
	var source *ypb.PCAPByteSource
	for _, candidate := range detail.GetByteSources() {
		if candidate.GetID() == detail.GetMessages()[0].GetSourceID() {
			source = candidate
			break
		}
	}
	require.NotNil(t, source)
	require.EqualValues(t, len(response), source.GetLength())
	require.Len(t, source.GetInitialData(), pcapDetailSourceInitialBytes)
	require.True(t, source.GetInitialTruncated())
	next, err := client.ReadPCAPBytes(context.Background(), &ypb.ReadPCAPBytesRequest{
		DatasetID: datasetID, SourceID: source.GetID(),
		Offset: int64(len(source.GetInitialData())), Length: int64(len(response) - len(source.GetInitialData())),
	})
	require.NoError(t, err)
	combined := append(append([]byte(nil), source.GetInitialData()...), next.GetData()...)
	require.Equal(t, response, combined)
	require.True(t, next.GetEOF())
}

func TestPCAPDecodeMessageNodesKeepsParsedFieldsOnDisplayError(t *testing.T) {
	nodes, truncated, diagnostic := pcapDecodeMessageNodes(pcapdb.ProtocolMessage{
		FieldsJSON:        []byte(`{"Method":"GET","Nested":{"Code":200}}`),
		DisplayFieldsJSON: []byte(`{broken`),
	}, "message:1", nil, 0)
	require.False(t, truncated)
	require.Contains(t, diagnostic, "display fields")
	require.NotNil(t, findPCAPFieldNode(nodes, "Parsed fields"))
	require.NotNil(t, findPCAPFieldNode(nodes, "Method"))
}
