package pcaputil

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

var binMQTTConnect = []byte{0x10, 14, 0, 4, 'M', 'Q', 'T', 'T', 4, 2, 0, 60, 0, 2, 'i', 'd'}

func binMQTTPublish(size int) []byte {
	n := size + 3
	w := []byte{0x30}
	for {
		b := byte(n % 128)
		n /= 128
		if n > 0 {
			b |= 128
		}
		w = append(w, b)
		if n == 0 {
			break
		}
	}
	w = append(w, 0, 1, 'a')
	return append(w, bytes.Repeat([]byte{'x'}, size)...)
}

func binTestPcap(t testing.TB, steps []tcpStep, port layers.TCPPort, ipv6 bool, ng bool) []byte {
	t.Helper()
	var output bytes.Buffer
	var write func(gopacket.CaptureInfo, []byte) error
	var flush func() error
	if ng {
		w, err := pcapgo.NewNgWriter(&output, layers.LinkTypeEthernet)
		require.NoError(t, err)
		write, flush = w.WritePacket, w.Flush
	} else {
		w := pcapgo.NewWriterNanos(&output)
		require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
		write = w.WritePacket
	}
	for i, s := range steps {
		src, dst := net.IP{192, 0, 2, 1}, net.IP{192, 0, 2, 2}
		if ipv6 {
			src, dst = net.ParseIP("2001:db8::1"), net.ParseIP("2001:db8::2")
		}
		sport, dport := layers.TCPPort(12345), port
		if s.reverse {
			src, dst, sport, dport = dst, src, dport, sport
		}
		eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
		var ip gopacket.SerializableLayer = &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: src, DstIP: dst}
		if ipv6 {
			eth.EthernetType = layers.EthernetTypeIPv6
			ip = &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: layers.IPProtocolTCP, SrcIP: src, DstIP: dst}
		}
		tcp := &layers.TCP{SrcPort: sport, DstPort: dport, Seq: s.seq, SYN: s.syn, FIN: s.fin, RST: s.rst, ACK: !s.syn}
		require.NoError(t, tcp.SetNetworkLayerForChecksum(ip.(gopacket.NetworkLayer)))
		buf := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp, gopacket.Payload(s.data)))
		raw := buf.Bytes()
		require.NoError(t, write(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, int64(i)*1234), CaptureLength: len(raw), Length: len(raw)}, raw))
	}
	if flush != nil {
		require.NoError(t, flush())
	}
	return output.Bytes()
}

func binReplay(t testing.TB, pcap []byte, workers int, options ...CaptureOption) ([]*BinParserEvent, BinParserStats, error) {
	t.Helper()
	var mu sync.Mutex
	var events []*BinParserEvent
	var stats BinParserStats
	opts := []CaptureOption{WithBinParser(func(e *BinParserEvent) { mu.Lock(); events = append(events, e); mu.Unlock() }), WithBinParserStats(func(s BinParserStats) { stats = s }), WithTCPReassemblyWorkers(workers)}
	opts = append(opts, options...)
	err := ReplayPcap(bytes.NewReader(pcap), opts...)
	return events, stats, err
}

func TestBinParserReplayReorderingOwnership(t *testing.T) {
	publish := binMQTTPublish(512)
	wire := string(append(append(append([]byte{}, binMQTTConnect...), publish...), publish...))
	steps := []tcpStep{{seq: 99, syn: true}, {seq: 111, data: wire[11:400]}, {seq: 100, data: wire[:11]}, {seq: 111, data: wire[11:400]}, {seq: 500, data: wire[400:]}, {seq: 100 + uint32(len(wire)), fin: true}}
	for _, workers := range []int{1, 2, 4} {
		for _, ipv6 := range []bool{false, true} {
			for _, ng := range []bool{false, true} {
				t.Run(fmt.Sprintf("w%d/ipv6=%v/ng=%v", workers, ipv6, ng), func(t *testing.T) {
					pcap := binTestPcap(t, steps, 1883, ipv6, ng)
					events, stats, err := binReplay(t, pcap, workers)
					require.NoError(t, err)
					require.Len(t, events, 3)
					require.Equal(t, uint64(3), stats.Decoded)
					require.Equal(t, uint64(len(wire)), stats.InputBytes)
					require.Zero(t, stats.BufferedBytes)
					for i := range pcap {
						pcap[i] = 0
					}
					require.Equal(t, binMQTTConnect, events[0].Raw)
					require.Equal(t, publish, events[1].Raw)
					require.Equal(t, uint64(len(binMQTTConnect)), events[1].Offset)
					require.NotEmpty(t, events[1].Structured["fields"])
					require.Equal(t, "mqtt", events[1].Protocol)
					require.True(t, time.Unix(1700000000, 2*1234).Equal(events[0].Timestamp))
				})
			}
		}
	}
}

func TestBinParserUnknownLimitsAndIncomplete(t *testing.T) {
	unknown := strings.Repeat("?", 1000)
	steps := []tcpStep{{seq: 0, syn: true}, {seq: 1, data: unknown}, {seq: 1001, data: unknown}}
	events, stats, err := binReplay(t, binTestPcap(t, steps, 443, false, false), 1)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "unrecognized", events[0].Status)
	require.Equal(t, uint64(1), stats.ProbeCalls)
	require.Equal(t, uint64(2000), stats.UnclassifiedBytes)
	require.Len(t, events[0].Raw, 64)
	steps = []tcpStep{{seq: 0, syn: true}, {seq: 1, data: string(binMQTTConnect) + string([]byte{0x30, 0xff, 0xff, 0x7f})}}
	events, stats, err = binReplay(t, binTestPcap(t, steps, 1883, false, false), 1)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, "limited", events[1].Status)
	require.Zero(t, stats.BufferedBytes)
	steps = []tcpStep{{seq: 0, syn: true}, {seq: 1, data: string(binMQTTConnect) + string(binMQTTPublish(100)[:10])}}
	events, stats, err = binReplay(t, binTestPcap(t, steps, 1883, false, false), 1)
	require.NoError(t, err)
	require.Equal(t, "incomplete", events[1].Status)
	require.Zero(t, stats.BufferedBytes)
}

func TestBinParserHTTPContextAndChunks(t *testing.T) {
	request := "HEAD / HTTP/1.1\r\nHost: example.test\r\n\r\nGET / HTTP/1.1\r\nHost: example.test\r\n\r\n"
	response := "HTTP/1.1 200 OK\r\nContent-Length: 999\r\n\r\nHTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n4\r\nWiki\r\n0\r\n\r\n"
	steps := []tcpStep{{seq: 0, syn: true}, {seq: 0, syn: true, reverse: true}, {seq: 1, data: request}, {seq: 1, data: response[:19], reverse: true}, {seq: 20, data: response[19:], reverse: true}}
	events, stats, err := binReplay(t, binTestPcap(t, steps, 80, false, false), 1)
	require.NoError(t, err)
	require.Len(t, events, 4)
	for _, e := range events {
		require.Equal(t, "decoded", e.Status, e.Error)
		require.Equal(t, "http", e.Protocol)
	}
	require.Equal(t, uint64(4), stats.Decoded)
	require.Zero(t, stats.BufferedBytes)
	_, stats, err = binReplay(t, binTestPcap(t, steps, 80, false, false), 2, WithBinParserDeferred(true))
	require.NoError(t, err)
	require.Equal(t, uint64(4), stats.Deferred)
}

func TestBinParserInspectorAndCallbackFailure(t *testing.T) {
	wire := append(append([]byte{}, binMQTTConnect...), binMQTTPublish(64)...)
	pcap := binTestPcap(t, []tcpStep{{seq: 0, syn: true}, {seq: 1, data: string(wire)}}, 1883, false, false)
	v, err := NewBinParserInspector(1, 1024)
	require.NoError(t, err)
	err = ReplayPcap(bytes.NewReader(pcap), WithBinParserDeferred(true), WithBinParser(v.OnEvent))
	require.NoError(t, err)
	require.Equal(t, uint64(1), v.Evicted())
	rows := v.Rows("mqtt", 0)
	require.Len(t, rows, 1)
	require.Empty(t, rows[0].Raw)
	detail, err := v.Details(rows[0].ID)
	require.NoError(t, err)
	require.NotEmpty(t, detail.Structured)
	detail.Raw[0] = 0
	again, err := v.Details(rows[0].ID)
	require.NoError(t, err)
	require.Equal(t, byte(0x30), again.Raw[0])
	for _, workers := range []int{1, 2, 4} {
		err = ReplayPcap(bytes.NewReader(pcap), WithTCPReassemblyWorkers(workers), WithBinParser(func(*BinParserEvent) { panic("consumer failed") }))
		require.ErrorContains(t, err, "consumer failed")
	}
}

func TestBinParserReplayOptionsAndCorruptFile(t *testing.T) {
	pcap := binTestPcap(t, []tcpStep{{seq: 0, syn: true}}, 80, false, false)
	require.Error(t, ReplayPcap(bytes.NewReader(pcap), WithBPFFilter("tcp")))
	require.Error(t, ReplayPcap(bytes.NewReader(pcap[:len(pcap)-1])))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, ReplayPcap(bytes.NewReader(pcap), WithContext(ctx)))
	bad := append([]byte{}, pcap...)
	binary.LittleEndian.PutUint32(bad[24+8:], 0xffffffff)
	require.Error(t, ReplayPcap(bytes.NewReader(bad)))
}

func TestBinParserHTTPUpgradeAndCloseBoundary(t *testing.T) {
	request := "CONNECT example.test:443 HTTP/1.1\r\nHost: example.test\r\n\r\n"
	response := "HTTP/1.1 200 Connected\r\n\r\n"
	record := string([]byte{23, 3, 3, 0, 3, 1, 2, 3})
	steps := []tcpStep{{seq: 0, syn: true}, {seq: 0, syn: true, reverse: true}, {seq: 1, data: request}, {seq: 1, data: response + record, reverse: true}, {seq: 1 + uint32(len(request)), data: record}}
	events, stats, err := binReplay(t, binTestPcap(t, steps, 8080, false, false), 1)
	require.NoError(t, err)
	require.Len(t, events, 4)
	require.Equal(t, uint64(4), stats.Decoded)
	require.Equal(t, "http", events[1].Protocol)
	require.Equal(t, "tls", events[2].Protocol)
	require.Equal(t, events[0].FlowID, events[3].FlowID)
	request = "GET / HTTP/1.0\r\n\r\n"
	response = "HTTP/1.0 200 OK\r\n\r\nbody"
	steps = []tcpStep{{seq: 0, syn: true}, {seq: 0, syn: true, reverse: true}, {seq: 1, data: request}, {seq: 1, data: response, reverse: true}}
	events, _, err = binReplay(t, binTestPcap(t, steps, 80, false, false), 1)
	require.NoError(t, err)
	require.Equal(t, "incomplete", events[1].Status)
	steps = append(steps, tcpStep{seq: 1 + uint32(len(request)), fin: true}, tcpStep{seq: 1 + uint32(len(response)), fin: true, reverse: true})
	events, stats, err = binReplay(t, binTestPcap(t, steps, 80, false, false), 1)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, uint64(2), stats.Decoded)
	require.Equal(t, "decoded", events[1].Status, events[1].Error)
}

func TestBinParserCoalescedMessageLimitAndBindingIndex(t *testing.T) {
	var events []*BinParserEvent
	var stats BinParserStats
	config := BinParserConfig{MaxMessageBytes: 64, MaxBufferedBytes: 128, OnEvent: func(e *BinParserEvent) { events = append(events, e) }, OnStats: func(s BinParserStats) { stats = s }}
	publish := binMQTTPublish(58)
	wire := string(append(append([]byte{}, publish...), publish...))
	steps := []tcpStep{{seq: 0, syn: true}, {seq: 1, data: string(binMQTTConnect)}, {seq: 17, data: wire[:50]}, {seq: 67, data: wire[50:]}}
	require.NoError(t, ReplayPcap(bytes.NewReader(binTestPcap(t, steps, 1883, false, false)), WithBinParserConfig(config)))
	require.Len(t, events, 3)
	require.Equal(t, uint64(3), stats.Decoded)
	require.Zero(t, stats.BufferedBytes)
	require.LessOrEqual(t, stats.PeakBufferedBytes, int64(128))
	var probes int
	config = BinParserConfig{OnEvent: func(*BinParserEvent) {}}
	for port := 10000; port < 10600; port++ {
		config.Bindings = append(config.Bindings, BinParserBinding{Port: uint16(port), Protocol: "mqtt-explicit", Rule: "application-layer.mqtt_fields", Entry: "MQTT311PacketFields", Probe: func(w []byte) bool { probes++; return len(w) >= 16 && w[0] == 0x10 }, Frame: func(w []byte) (int, error) { n, _, err := mqttLength(w); return n, err }})
	}
	require.NoError(t, ReplayPcap(bytes.NewReader(binTestPcap(t, steps, 10001, false, false)), WithBinParserConfig(config)))
	require.Equal(t, 1, probes, "other 599 port bindings must not be probed; affinity must persist")
}

func TestBinParserPcapngBounds(t *testing.T) {
	ng := binTestPcap(t, []tcpStep{{seq: 0, syn: true}}, 80, false, true)
	require.NoError(t, ReplayPcap(bytes.NewReader(ng)))
	bad := append([]byte{}, ng...)
	binary.LittleEndian.PutUint32(bad[4:8], 0xfffffff0)
	require.ErrorContains(t, ReplayPcap(bytes.NewReader(bad)), "bounded reader limits")
}

func TestBinParserHTTPExpectContinue(t *testing.T) {
	header := "POST / HTTP/1.1\r\nHost: example.test\r\nContent-Length: 3\r\nExpect: 100-continue\r\n\r\n"
	interim := "HTTP/1.1 100 Continue\r\n\r\n"
	final := "HTTP/1.1 204 No Content\r\n\r\n"
	steps := []tcpStep{{seq: 0, syn: true}, {seq: 0, syn: true, reverse: true}, {seq: 1, data: header}, {seq: 1, data: interim, reverse: true}, {seq: 1 + uint32(len(header)), data: "abc"}, {seq: 1 + uint32(len(interim)), data: final, reverse: true}}
	events, stats, err := binReplay(t, binTestPcap(t, steps, 80, false, false), 1)
	require.NoError(t, err)
	require.Len(t, events, 3)
	require.Equal(t, uint64(3), stats.Decoded)
	require.Zero(t, stats.ContextRequired)
}

func TestBinParserSequenceGapsVisibleForEveryWorker(t *testing.T) {
	pcap := binTestPcap(t, []tcpStep{{seq: 0, syn: true}, {seq: 10, data: string(binMQTTConnect)}}, 1883, false, false)
	for _, workers := range []int{1, 2, 4} {
		var reassembly TCPReassemblyStats
		events, stats, err := binReplay(t, pcap, workers, WithTCPReassemblyStats(func(s TCPReassemblyStats) { reassembly = s }))
		require.ErrorContains(t, err, "unfilled sequence gap")
		require.Equal(t, uint64(len(binMQTTConnect)), reassembly.UnreassembledBytes)
		require.Len(t, events, 1)
		require.Equal(t, "incomplete", events[0].Status)
		require.Positive(t, events[0].FlowID)
		require.Zero(t, stats.Decoded)
	}
}

func TestBinParserReassemblyLimitIsAnError(t *testing.T) {
	pcap := binTestPcap(t, []tcpStep{{seq: 0, syn: true}, {seq: 10, data: string(binMQTTConnect)}}, 1883, false, false)
	_, _, err := binReplay(t, pcap, 1, WithTCPReassemblyOptions(TCPReassemblyOptions{MaxPendingBytes: 8}))
	require.ErrorContains(t, err, "resource limit")
}
