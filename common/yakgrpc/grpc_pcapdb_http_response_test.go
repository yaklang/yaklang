package yakgrpc

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func pcapHTTPResponseWithoutRequestCapture(t *testing.T) string {
	t.Helper()
	response := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nConnection: close\r\n\r\nThe response starts before this capture.")
	type frame struct {
		reverse, syn, ack, fin bool
		seq, ackNo             uint32
		payload                []byte
	}
	frames := []frame{
		{syn: true, seq: 1000},
		{reverse: true, syn: true, ack: true, seq: 2000, ackNo: 1001},
		{ack: true, seq: 1001, ackNo: 2001},
		{reverse: true, ack: true, seq: 2001, ackNo: 1001, payload: response},
		{reverse: true, ack: true, fin: true, seq: 2001 + uint32(len(response)), ackNo: 1001},
		{ack: true, fin: true, seq: 1001, ackNo: 2002 + uint32(len(response))},
	}
	var capture bytes.Buffer
	writer := pcapgo.NewWriterNanos(&capture)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	for i, frame := range frames {
		ethernet := &layers.Ethernet{
			SrcMAC: net.HardwareAddr{1, 2, 3, 4, 5, 6}, DstMAC: net.HardwareAddr{6, 5, 4, 3, 2, 1},
			EthernetType: layers.EthernetTypeIPv4,
		}
		ip := &layers.IPv4{
			Version: 4, TTL: 64, SrcIP: net.IPv4(10, 0, 0, 1), DstIP: net.IPv4(8, 8, 8, 8),
			Protocol: layers.IPProtocolTCP,
		}
		tcp := &layers.TCP{
			SrcPort: 12345, DstPort: 80, Seq: frame.seq, Ack: frame.ackNo,
			SYN: frame.syn, ACK: frame.ack, FIN: frame.fin, Window: 65535,
		}
		if frame.reverse {
			ip.SrcIP, ip.DstIP = ip.DstIP, ip.SrcIP
			tcp.SrcPort, tcp.DstPort = tcp.DstPort, tcp.SrcPort
		}
		require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
		buffer := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(
			buffer,
			gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true},
			ethernet, ip, tcp, gopacket.Payload(frame.payload),
		))
		raw := buffer.Bytes()
		require.NoError(t, writer.WritePacket(
			gopacket.CaptureInfo{Timestamp: time.Unix(1700000000+int64(i), 0), CaptureLength: len(raw), Length: len(raw)},
			raw,
		))
	}
	path := filepath.Join(t.TempDir(), "http-response-without-request.pcap")
	require.NoError(t, os.WriteFile(path, capture.Bytes(), 0600))
	return path
}

func TestPCAPGRPCQueriesHTTPResponseWithoutRequest(t *testing.T) {
	_, client := newPCAPTestClient(t)
	events := importPCAPForTest(t, client, pcapHTTPResponseWithoutRequestCapture(t))
	datasetID := events[len(events)-1].GetDatasetID()
	ctx := context.Background()

	page, err := client.QueryPCAPPackets(ctx, &ypb.QueryPCAPPacketsRequest{
		DatasetID: datasetID,
		Filter:    &ypb.PCAPPacketsFilter{ApplicationLayerTypes: []string{"http"}},
	})
	require.NoError(t, err)
	require.Len(t, page.GetData(), 1)
	require.EqualValues(t, 1, page.GetTotal())
	require.EqualValues(t, 4, page.GetData()[0].GetId())
	require.Equal(t, "HTTP", page.GetData()[0].GetProtocol())
	require.Contains(t, page.GetData()[0].GetInfo(), "HTTP/1.1 200 OK")

	detail, err := client.GetPCAPPacketDetail(ctx, &ypb.GetPCAPPacketDetailRequest{
		DatasetID: datasetID,
		PacketID:  page.GetData()[0].GetId(),
	})
	require.NoError(t, err)
	require.Len(t, detail.GetMessages(), 1)
	require.Equal(t, "HTTP", detail.GetMessages()[0].GetProtocol())
	require.Contains(t, detail.GetMessages()[0].GetSummary(), "HTTP/1.1 200 OK")
	require.Equal(t, "decoded", detail.GetMessages()[0].GetStatus())
	require.Equal(t, "message", detail.GetMessages()[0].GetCompleteness())
}
