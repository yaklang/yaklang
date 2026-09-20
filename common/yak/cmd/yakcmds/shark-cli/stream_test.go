package sharkcli

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func tcpPacket(t *testing.T, number uint64, seq uint32, payload string, reverse, syn, fin, ipv6 bool) *capturedPacket {
	t.Helper()
	src, dst := net.ParseIP("192.0.2.1"), net.ParseIP("198.51.100.2")
	if ipv6 {
		src, dst = net.ParseIP("2001:db8::1"), net.ParseIP("2001:db8::2")
	}
	tcp := &layers.TCP{SrcPort: 45000, DstPort: 12011, Seq: seq, ACK: !syn || reverse, SYN: syn, FIN: fin, Window: 65535}
	if reverse {
		src, dst = dst, src
		tcp.SrcPort, tcp.DstPort = tcp.DstPort, tcp.SrcPort
	}
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
	var network gopacket.SerializableLayer
	if ipv6 {
		ip := &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: layers.IPProtocolTCP, SrcIP: src, DstIP: dst}
		require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
		network = ip
		eth.EthernetType = layers.EthernetTypeIPv6
	} else {
		ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: src, DstIP: dst}
		require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
		network = ip
	}
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, network, tcp, gopacket.Payload([]byte(payload))))
	return &capturedPacket{number: number, data: b.Bytes(), link: layers.LinkTypeEthernet, ci: gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, int64(number)*1000000), CaptureLength: len(b.Bytes()), Length: len(b.Bytes())}}
}
func directionData(v *streamSnapshot, d int) string {
	var b strings.Builder
	for _, c := range v.chunks {
		if c.direction == d {
			b.Write(c.data)
		}
	}
	return b.String()
}

func TestStreamReordersDeduplicatesAndIdentifiesWholeConnection(t *testing.T) {
	for _, ipv6 := range []bool{false, true} {
		t.Run(fmt.Sprint(ipv6), func(t *testing.T) {
			s := newStreamStore(16, 65536)
			packets := []*capturedPacket{
				tcpPacket(t, 1, 100, "", false, true, false, ipv6), tcpPacket(t, 2, 500, "", true, true, false, ipv6),
				tcpPacket(t, 3, 103, "T /hello HTTP/1.1\r\nHost: example.com\r\n\r\n", false, false, false, ipv6),
				tcpPacket(t, 4, 101, "GE", false, false, false, ipv6),
				tcpPacket(t, 5, 101, "GET /hello HTTP/1.1\r\nHost: example.com\r\n\r\n", false, false, false, ipv6),
				tcpPacket(t, 6, 501, "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nHELLO", true, false, false, ipv6),
			}
			for _, p := range packets {
				s.add(p)
			}
			v := s.snapshot(packets[0].streamID)
			require.NotNil(t, v)
			require.Equal(t, "GET /hello HTTP/1.1\r\nHost: example.com\r\n\r\n", directionData(v, 0))
			require.Contains(t, directionData(v, 1), "HELLO")
			require.Equal(t, uint64(1), v.outOfOrder)
			require.Equal(t, uint64(1), v.retransmits)
			require.Zero(t, v.gaps)
			require.Equal(t, "HTTP", v.protocol)
			u := &tui{streams: s}
			row := u.packetRow(&packetEntry{packet: packets[0]})
			require.Equal(t, "TCP/HTTP", row.Protocol)
			require.Equal(t, v.id, row.StreamID)
			// Reads are detached: capture cannot mutate a browsed snapshot.
			old := directionData(v, 1)
			s.add(tcpPacket(t, 7, 501+uint32(len(old)), "NEXT", true, false, false, ipv6))
			require.Equal(t, old, directionData(v, 1))
		})
	}
}
func TestStreamMidCaptureGapsWrapAndTupleReuse(t *testing.T) {
	t.Run("midstream reorder", func(t *testing.T) {
		s := newStreamStore(8, 65536)
		p := tcpPacket(t, 1, 105, "world", false, false, false, false)
		s.add(p)
		s.add(tcpPacket(t, 2, 100, "hello", false, false, false, false))
		s.finish()
		v := s.snapshot(p.streamID)
		require.Equal(t, "helloworld", directionData(v, 0))
		require.Equal(t, -1, v.chunks[0].gap)
	})
	t.Run("missing bytes", func(t *testing.T) {
		s := newStreamStore(8, 65536)
		p := tcpPacket(t, 1, 100, "", false, true, false, false)
		s.add(p)
		s.add(tcpPacket(t, 2, 101, "GE", false, false, false, false))
		s.add(tcpPacket(t, 3, 109, "T /gap", false, false, false, false))
		s.finish()
		v := s.snapshot(p.streamID)
		require.Equal(t, uint64(6), v.missing)
		require.Empty(t, v.protocol)
		require.Equal(t, 6, v.chunks[1].gap)
		require.Equal(t, "GE", string(v.prefix[0]), "classification must not concatenate across gaps")
	})
	t.Run("sequence rollover", func(t *testing.T) {
		s := newStreamStore(8, 65536)
		p := tcpPacket(t, 1, 0xfffffffc, "", false, true, false, false)
		s.add(p)
		s.add(tcpPacket(t, 2, 0xfffffffd, "abc", false, false, false, false))
		s.add(tcpPacket(t, 3, 0, "def", false, false, false, false))
		require.Equal(t, "abcdef", directionData(s.snapshot(p.streamID), 0))
	})
	t.Run("new SYN creates new stream", func(t *testing.T) {
		s := newStreamStore(8, 65536)
		p := tcpPacket(t, 1, 100, "", false, true, false, false)
		s.add(p)
		s.add(tcpPacket(t, 2, 101, "old", false, false, false, false))
		next := tcpPacket(t, 3, 900, "", false, true, false, false)
		s.add(next)
		s.add(tcpPacket(t, 4, 901, "new", false, false, false, false))
		require.NotEqual(t, p.streamID, next.streamID)
		require.Equal(t, "new", directionData(s.snapshot(next.streamID), 0))
		require.Equal(t, "old", directionData(s.snapshot(p.streamID), 0))
	})
}
func TestStreamLimitsAndHistoryEviction(t *testing.T) {
	s := newStreamStore(2, 64)
	p := tcpPacket(t, 1, 100, "", false, true, false, false)
	s.add(p)
	for i := 0; i < 20; i++ {
		s.add(tcpPacket(t, uint64(i+2), uint32(101+i*16), strings.Repeat("x", 16), false, false, false, false))
	}
	v := s.snapshot(p.streamID)
	require.Greater(t, v.omitted, 0)
	require.LessOrEqual(t, len(directionData(v, 0)), 64)
	for i := 0; i < 5; i++ {
		s.add(tcpPacket(t, uint64(100+i), uint32(1000+i*100), "", false, true, false, false))
	}
	require.LessOrEqual(t, len(s.records), 2)
	require.Nil(t, s.snapshot(p.streamID))
	require.LessOrEqual(t, s.retained, 128)
}
func TestStreamRunsBeforeLossyDisplayQueue(t *testing.T) {
	var packets [][]byte
	packets = append(packets, tcpPacket(t, 1, 100, "", false, true, false, false).data)
	for i, payload := range []string{"GE", "T ", "/queue ", "HTTP/1.1\r\n", "\r\n"} {
		seq := 101
		for j := 1; j < len(packets); j++ {
			decoded := gopacket.NewPacket(packets[j], layers.LinkTypeEthernet, gopacket.Default)
			seq += len(decoded.TransportLayer().LayerPayload())
		}
		packets = append(packets, tcpPacket(t, uint64(i+2), uint32(seq), payload, false, false, false, false).data)
	}
	src, err := openOffline(fixture(t, "queue.pcap", packets...), "")
	require.NoError(t, err)
	defer src.Close()
	src.offline = false // Exercise the live queue policy with a deterministic reader.
	s := &captureSession{packets: make(chan *capturedPacket, 1), streams: newStreamStore(8, 65536)}
	require.NoError(t, s.capture(context.Background(), src, captureConfig{}, true))
	last := <-s.packets
	require.Positive(t, s.skipped.Load())
	require.Equal(t, "GET /queue HTTP/1.1\r\n\r\n", directionData(s.streams.snapshot(last.streamID), 0))
}
func TestStreamApplicationSignaturesAndYAMLDecode(t *testing.T) {
	for _, tc := range []struct{ data, name string }{
		{"GET / HTTP/1.1\r\n\r\n", "HTTP"}, {"\x17\x03\x03\x00\x05abcde", "TLS"}, {"SSH-2.0-OpenSSH\r\n", "SSH"},
		{"*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n", "Redis"}, {"\x10\x10\x00\x04MQTT\x04\x02\x00\x3c", "MQTT"}, {"AMQP\x00\x00\x09\x01", "AMQP"}, {"RFB 003.008\n", "VNC"},
		{"\xfeSMB0123456789", "SMB2"}, {string(bytes.Repeat([]byte{0xa5}, 80)), ""},
	} {
		require.Equal(t, tc.name, sniffStreamPayload([]byte(tc.data), 12011, 45000), fmt.Sprintf("%q", tc.data))
	}
	d := decodeStream(streamDecodeRequest{ID: 9, Ports: [2]uint16{45000, 12011}, Prefix: [2][]byte{[]byte("GET /assembled HTTP/1.1\r\nHost: example.com\r\n\r\n"), []byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nHELLO")}})
	require.Equal(t, "HTTP", d.protocol)
	var text strings.Builder
	for _, f := range d.fields {
		text.WriteString(f.text)
		text.WriteByte('\n')
		require.Zero(t, f.depth)
	}
	require.Contains(t, text.String(), "/assembled")
	require.Contains(t, text.String(), "HELLO")
	require.NotContains(t, text.String(), "Sequence Number")
}
func TestStreamPaneSelectionScrollingAndFlatFields(t *testing.T) {
	u := browsingFixture(t)
	u.fields = detail(u.inspect)
	for _, f := range u.fields.fields {
		require.Zero(t, f.depth)
		require.NotEqual(t, "root", f.text)
	}
	frame := u.view(&captureSession{}, &captureSource{}, captureConfig{})
	require.NotContains(t, frame, "▾")
	require.NotContains(t, frame, "▸")
	require.Contains(t, frame, "[4 Stream]")
	u.streams = newStreamStore(8, 65536)
	p := tcpPacket(t, 101, 100, "", false, true, false, false)
	u.streams.add(p)
	u.streams.add(tcpPacket(t, 102, 101, "GET /stream HTTP/1.1\r\nHost: test\r\n\r\n"+strings.Repeat("body\n", 100), false, false, false, false))
	u.ring.add(p)
	u.follow = true
	u.refreshVisible()
	u.syncInspection(time.Now())
	l := layoutDashboard(u.width-1, u.height)
	u.mouse(mouseEvent{action: mousePress, button: 0, x: 38, y: l.tabs})
	require.Equal(t, 3, u.pane)
	require.NotNil(t, u.stream)
	r := u.viewport(3)
	u.mouse(mouseEvent{action: mouseWheel, x: r.x + 4, y: r.y + 2, delta: 6})
	require.Equal(t, 6, u.streamTop)
	frame = u.view(&captureSession{}, &captureSource{}, captureConfig{})
	require.NotContains(t, frame, "PROTOCOL FIELDS")
	require.Contains(t, frame, "4 STREAM")
	u.key("d", &captureSource{})
	require.Equal(t, 2, u.streamMode)
	u.key("x", &captureSource{})
	require.Equal(t, 1, u.streamMode)
	u.key("v", &captureSource{})
	require.Equal(t, 1, u.streamDirection)
	u.key("\t", &captureSource{})
	require.Equal(t, 0, u.pane)
}

func TestStreamAutoRefreshStopsWhileBrowsing(t *testing.T) {
	u := browsingFixture(t)
	u.streams = newStreamStore(8, 65536)
	p := tcpPacket(t, 200, 100, "", false, true, false, false)
	u.streams.add(p)
	u.ring.add(p)
	u.follow = true
	u.refreshVisible()
	u.syncInspection(time.Now())
	u.focusPane(3)
	version := u.stream.version
	u.streams.add(tcpPacket(t, 201, 101, "GET / HTTP/1.1\r\n\r\n", false, false, false, false))
	u.streamRefresh = time.Time{}
	u.syncStream()
	require.Greater(t, u.stream.version, version)
	u.scroll(3, 3)
	version = u.stream.version
	u.streams.add(tcpPacket(t, 202, 120, "NEXT", false, false, false, false))
	u.streamRefresh = time.Time{}
	u.syncStream()
	require.Equal(t, version, u.stream.version)
	u.key("r", &captureSource{})
	require.False(t, u.streamPinned)
	require.Greater(t, u.stream.version, version)
}

func TestStreamFINClosesBothDirections(t *testing.T) {
	s := newStreamStore(8, 65536)
	p := tcpPacket(t, 1, 100, "", false, true, false, false)
	s.add(p)
	s.add(tcpPacket(t, 2, 500, "", true, true, false, false))
	s.add(tcpPacket(t, 3, 101, "abc", false, false, false, false))
	s.add(tcpPacket(t, 4, 501, "xy", true, false, false, false))
	s.add(tcpPacket(t, 5, 104, "", false, false, true, false))
	s.add(tcpPacket(t, 6, 503, "", true, false, true, false))
	v := s.snapshot(p.streamID)
	require.Equal(t, "FIN", v.closed)
	require.Equal(t, "abc", directionData(v, 0))
	require.Equal(t, "xy", directionData(v, 1))
	require.Empty(t, s.active)
}
