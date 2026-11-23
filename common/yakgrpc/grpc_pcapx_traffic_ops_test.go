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
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

func TestDeprecatedTrafficRPCsReturnUnimplemented(t *testing.T) {
	_, client := newPCAPTestClient(t)
	ctx := context.Background()

	_, err := client.QueryTrafficSession(ctx, &ypb.QueryTrafficSessionRequest{})
	require.Equal(t, codes.Unimplemented, grpcstatus.Code(err))
	_, err = client.QueryTrafficPacket(ctx, &ypb.QueryTrafficPacketRequest{})
	require.Equal(t, codes.Unimplemented, grpcstatus.Code(err))
	_, err = client.QueryTrafficTCPReassembled(ctx, &ypb.QueryTrafficTCPReassembledRequest{})
	require.Equal(t, codes.Unimplemented, grpcstatus.Code(err))
	_, err = client.ParseTraffic(ctx, &ypb.ParseTrafficRequest{Type: "packet", Id: 1})
	require.Equal(t, codes.Unimplemented, grpcstatus.Code(err))
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
		{syn: true, seq: 1000},
		{reverse: true, syn: true, ack: true, seq: 2000, ackNo: 1001},
		{ack: true, seq: 1001, ackNo: 2001},
		{ack: true, seq: 1001, ackNo: 2001, payload: request},
		{reverse: true, ack: true, seq: 2001, ackNo: 1001 + uint32(len(request)), payload: response[:30]},
		{reverse: true, ack: true, seq: 2031, ackNo: 1001 + uint32(len(request)), payload: response[30:]},
		{reverse: true, ack: true, fin: true, seq: 2001 + uint32(len(response)), ackNo: 1001 + uint32(len(request))},
	}
	var capture bytes.Buffer
	writer := pcapgo.NewWriterNanos(&capture)
	require.NoError(t, writer.WriteFileHeader(65535, layers.LinkTypeEthernet))
	var raws [][]byte
	for index, item := range frames {
		ethernet := &layers.Ethernet{SrcMAC: net.HardwareAddr{1, 2, 3, 4, 5, 6}, DstMAC: net.HardwareAddr{6, 5, 4, 3, 2, 1}, EthernetType: layers.EthernetTypeIPv4}
		ipv4 := &layers.IPv4{Version: 4, TTL: 64, SrcIP: net.IPv4(10, 0, 0, 1), DstIP: net.IPv4(8, 8, 8, 8), Protocol: layers.IPProtocolTCP}
		tcp := &layers.TCP{SrcPort: 12345, DstPort: 80, Seq: item.seq, Ack: item.ackNo, SYN: item.syn, ACK: item.ack, FIN: item.fin, Window: 65535}
		if item.reverse {
			ipv4.SrcIP, ipv4.DstIP = ipv4.DstIP, ipv4.SrcIP
			tcp.SrcPort, tcp.DstPort = tcp.DstPort, tcp.SrcPort
		}
		require.NoError(t, tcp.SetNetworkLayerForChecksum(ipv4))
		buffer := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ethernet, ipv4, tcp, gopacket.Payload(item.payload)))
		raw := append([]byte(nil), buffer.Bytes()...)
		raws = append(raws, raw)
		require.NoError(t, writer.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000+int64(index), 0), CaptureLength: len(raw), Length: len(raw)}, raw))
	}
	path := filepath.Join(t.TempDir(), "http.pcap")
	require.NoError(t, os.WriteFile(path, capture.Bytes(), 0600))
	return path, raws, response
}
