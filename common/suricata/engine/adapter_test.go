package engine

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestPcapAdapterTCPStreamAndTransaction(t *testing.T) {
	var mu sync.Mutex
	var events []InspectionEvent
	adapter, err := NewPcapAdapter(64, func(event InspectionEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})
	require.NoError(t, err)
	analyzer, err := pcaputil.NewPacketAnalyzer(adapter.Options()...)
	require.NoError(t, err)

	clientSeq, serverSeq := uint32(1000), uint32(5000)
	feed := func(at time.Time, srcPort, dstPort uint16, seq, ack uint32, flags string, payload []byte) {
		raw := serializeTCP(t, srcPort, dstPort, seq, ack, flags, payload)
		require.NoError(t, analyzer.Feed(raw, gopacket.CaptureInfo{Timestamp: at, CaptureLength: len(raw), Length: len(raw), InterfaceIndex: 7}, layers.LinkTypeEthernet))
	}
	base := time.Unix(1700000000, 0)
	feed(base, 40000, 80, clientSeq, 0, "syn", nil)
	feed(base.Add(time.Millisecond), 80, 40000, serverSeq, clientSeq+1, "syn,ack", nil)
	feed(base.Add(2*time.Millisecond), 40000, 80, clientSeq+1, serverSeq+1, "ack", nil)
	request := []byte("GET /split HTTP/1.1\r\nHost: example.com\r\n\r\n")
	cut := 13
	feed(base.Add(4*time.Millisecond), 40000, 80, clientSeq+1+uint32(cut), serverSeq+1, "ack,psh", request[cut:])
	feed(base.Add(3*time.Millisecond), 40000, 80, clientSeq+1, serverSeq+1, "ack,psh", request[:cut])
	feed(base.Add(5*time.Millisecond), 80, 40000, serverSeq+1, clientSeq+1+uint32(len(request)), "ack,psh", []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"))
	require.NoError(t, analyzer.Close())

	mu.Lock()
	defer mu.Unlock()
	var streamBytes []byte
	var transaction bool
	var joinedPacket bool
	for _, event := range events {
		switch event.View {
		case ViewStream:
			if event.Direction == 0 {
				streamBytes = append(streamBytes, event.Payload...)
			}
		case ViewTransaction:
			transaction = transaction || event.Protocol != nil && event.Protocol.Protocol == "http"
		case ViewNormalizedPacket:
			joinedPacket = joinedPacket || event.Flow.Key.Generation != 0 && event.Packet != nil
		}
	}
	require.Equal(t, request, streamBytes)
	require.True(t, transaction)
	require.True(t, joinedPacket)
	stats := adapter.Stats()
	require.Greater(t, stats.JoinHits, uint64(0))
	require.Greater(t, stats.StreamFrames, uint64(0))
	require.Greater(t, stats.Transactions, uint64(0))
}

func TestPcapAdapterOwnsCallbackData(t *testing.T) {
	var saved InspectionEvent
	adapter, err := NewPcapAdapter(1, func(event InspectionEvent) {
		if event.View == ViewPhysicalPacket {
			saved = event
		}
	})
	require.NoError(t, err)
	raw := serializeTCP(t, 1000, 80, 1, 0, "syn", []byte("owned"))
	packet := gopacket.NewPacket(raw, layers.LayerTypeEthernet, gopacket.NoCopy)
	packet.Metadata().CaptureInfo = gopacket.CaptureInfo{Timestamp: time.Unix(1, 0), CaptureLength: len(raw), Length: len(raw)}
	adapter.onPhysicalPacket(packet)
	for index := range raw {
		raw[index] = 0
	}
	require.NotEmpty(t, saved.PacketData)
	require.NotEqual(t, raw, saved.PacketData)
	require.Contains(t, string(saved.PacketData), "owned")
}

func serializeTCP(t testing.TB, srcPort, dstPort uint16, seq, ack uint32, flags string, payload []byte) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: []byte{0, 1, 2, 3, 4, 5}, DstMAC: []byte{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, SrcIP: []byte{192, 0, 2, 1}, DstIP: []byte{192, 0, 2, 2}, Protocol: layers.IPProtocolTCP}
	if srcPort < dstPort {
		ip.SrcIP, ip.DstIP = ip.DstIP, ip.SrcIP
	}
	tcp := &layers.TCP{SrcPort: layers.TCPPort(srcPort), DstPort: layers.TCPPort(dstPort), Seq: seq, Ack: ack, Window: 65535}
	for _, flag := range strings.Split(flags, ",") {
		switch flag {
		case "syn":
			tcp.SYN = true
		case "ack":
			tcp.ACK = true
		case "psh":
			tcp.PSH = true
		case "fin":
			tcp.FIN = true
		case "rst":
			tcp.RST = true
		}
	}
	require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
	buffer := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp, gopacket.Payload(payload)))
	return append([]byte(nil), buffer.Bytes()...)
}
