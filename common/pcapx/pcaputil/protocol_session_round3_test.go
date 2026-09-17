package pcaputil

import (
	"bytes"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func TestRound3SMTPPipeline(t *testing.T) {
	for _, chunk := range []int{0, 1, 7} {
		t.Run(fmt.Sprint(chunk), func(t *testing.T) {
			steps := []sessionStep{
				{1, mailCR("220 test ESMTP ready")},
				{0, mailCR("MAIL FROM:<a@b>\r\nRCPT TO:<c@d>\r\nDATA")},
				{1, mailCR("250 sender\r\n250 recipient\r\n354 send body")},
				{0, mailCR("Subject: test\r\n\r\nbody\r\n.\r\nQUIT")},
				{1, mailCR("250 queued\r\n221 closing")},
			}
			events, _ := sessionTestFlow(t, "smtp", steps, chunk, false)
			var replies []string
			for _, e := range events {
				require.Empty(t, e.Error)
				if e.Session["Role"] == "reply" && e.Session["In Reply To"] != nil {
					replies = append(replies, e.Session["In Reply To"].(string))
				}
			}
			require.Equal(t, []string{"MAIL", "RCPT", "DATA", "DATA", "QUIT"}, replies)
		})
	}
}

func TestRound3FTPMultilineAndPreliminary(t *testing.T) {
	for _, chunk := range []int{0, 1, 7} {
		steps := []sessionStep{
			{1, mailCR("220 test FTP ready")}, {0, mailCR("FEAT")},
			{1, mailCR("211-Features:\r\n UTF8\r\n SIZE\r\n211 End")},
			{0, mailCR("RETR readme")}, {1, mailCR("150 opening\r\n226 transfer complete")},
		}
		events, _ := sessionTestFlow(t, "ftp", steps, chunk, false)
		require.Len(t, events, 6)
		for _, e := range events {
			require.Empty(t, e.Error)
		}
		require.Equal(t, "FEAT", events[2].Session["In Reply To"])
		require.Equal(t, "RETR", events[4].Session["In Reply To"])
		require.Equal(t, "RETR", events[5].Session["In Reply To"])
	}
}

func TestRound3RadiusRejectPartialAttributes(t *testing.T) {
	for _, attrs := range [][]byte{{1}, {1, 1}, {1, 5, 'x'}} {
		_, err := (&binRADIUS{}).consume(radiusPkt(1, 1, attrs), 8)
		require.Error(t, err)
	}
	_, err := (&binRADIUS{}).consume(radiusPkt(1, 1, append(radiusAttr(1, []byte("a")), radiusAttr(32, []byte("b"))...)), 1)
	require.ErrorContains(t, err, "budget")
}

func TestRound3CoAPSeparateResponse(t *testing.T) {
	s := &binCoAP{}
	_, err := s.consume(coapMsg(0, 1, 1, 7, []byte{0xab}, nil, nil))
	require.NoError(t, err)
	ack, err := s.consume(coapMsg(2, 0, 0, 7, nil, nil, nil))
	require.NoError(t, err)
	require.Equal(t, "GET", ack["In Reply To"])
	wrong, err := s.consume(coapMsg(0, 1, 69, 90, []byte{0xcd}, nil, []byte("wrong")))
	require.NoError(t, err)
	require.Equal(t, "missing-request", wrong["Association"])
	resp, err := s.consume(coapMsg(0, 1, 69, 91, []byte{0xab}, nil, []byte("ok")))
	require.NoError(t, err)
	require.Equal(t, "response", resp["Role"])
	require.Equal(t, "GET", resp["In Reply To"])
	require.Empty(t, s.pending)
	for _, bad := range [][]byte{
		coapMsg(3, 0, 69, 1, nil, nil, nil), coapMsg(2, 1, 0, 1, []byte{1}, nil, nil), coapMsg(2, 0, 69, 1, nil, []byte{0xff}, nil), coapMsg(0, 0, 1, 1, nil, []byte{0xf0}, nil),
	} {
		_, err := (&binCoAP{}).consume(bad)
		require.Error(t, err)
	}
	// An extended option value of 15 is valid; only nibble 15 is reserved.
	_, err = (&binCoAP{}).consume(coapMsg(0, 0, 1, 1, nil, append([]byte{0x0d, 2}, bytes.Repeat([]byte{'x'}, 15)...), nil))
	require.NoError(t, err)
}

func sessionDatagramPCAP(t testing.TB, steps []sessionStep, port layers.UDPPort) []byte {
	t.Helper()
	var out bytes.Buffer
	w := pcapgo.NewWriterNanos(&out)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
	for i, st := range steps {
		src, dst := net.IP{192, 0, 2, 1}, net.IP{192, 0, 2, 2}
		sport, dport := layers.UDPPort(40000), port
		if st.dir == 1 {
			src, dst, sport, dport = dst, src, dport, sport
		}
		eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
		ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: src, DstIP: dst}
		udp := &layers.UDP{SrcPort: sport, DstPort: dport}
		require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
		b := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, gopacket.Payload(st.wire)))
		wire := b.Bytes()
		require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, int64(i)*1234), CaptureLength: len(wire), Length: len(wire)}, wire))
	}
	return out.Bytes()
}

func TestRound3UDPReplay(t *testing.T) {
	for _, sample := range m1SessionSamples(t) {
		switch sample.protocol {
		case "radius", "dhcp", "ntp", "coap":
		default:
			continue
		}
		for _, deferred := range []bool{false, true} {
			for _, workers := range []int{1, 2} {
				t.Run(fmt.Sprintf("%s/deferred=%v/workers=%d", sample.protocol, deferred, workers), func(t *testing.T) {
					var events []*BinParserEvent
					var stats BinParserStats
					err := ReplayPcap(bytes.NewReader(sessionDatagramPCAP(t, sample.steps, layers.UDPPort(sample.port))), WithTCPReassemblyWorkers(workers), WithBinParserStats(func(s BinParserStats) { stats = s }), WithBinParserConfig(BinParserConfig{Deferred: deferred, OnEvent: func(e *BinParserEvent) { events = append(events, e) }}))
					require.NoError(t, err)
					require.Len(t, events, len(sample.steps))
					require.Zero(t, stats.Malformed)
					for _, e := range events {
						require.Equal(t, "udp", e.Transport)
						require.Equal(t, sample.protocol, e.Protocol)
						require.Empty(t, e.Error)
						require.NotNil(t, e.Session)
						_, err = e.Decode()
						require.NoError(t, err)
					}
				})
			}
		}
	}
}

func TestRound3ModbusDirectionAndValidation(t *testing.T) {
	s := &binModbus{}
	request := mbap(1, 1, 3, []byte{0, 0, 0, 2})
	for i := 0; i < 2; i++ {
		info, err := s.consume(0, request)
		require.NoError(t, err)
		require.Equal(t, "request", info["Role"])
	}
	info, err := s.consume(1, mbap(1, 1, 3, []byte{4, 0, 1, 0, 2}))
	require.NoError(t, err)
	require.Equal(t, "response", info["Role"])
	for _, wire := range [][]byte{mbap(2, 1, 3, nil), mbap(2, 1, 0x83, nil), mbap(2, 1, 3, []byte{0, 0, 0, 0}), mbap(2, 1, 16, []byte{0, 0, 0, 2, 2, 0, 1})} {
		_, err := (&binModbus{}).consume(0, wire)
		require.Error(t, err)
	}
	_, err = s.consume(0, request)
	require.NoError(t, err)
	_, err = s.consume(1, mbap(1, 2, 3, []byte{4, 0, 1, 0, 2}))
	require.ErrorContains(t, err, "unit/function")
}

func TestRound3DHCPAndTNSBounds(t *testing.T) {
	wire := dhcpMsg(1, 1, 7, []byte{1, 2, 3, 4, 5, 6})
	n, err := dhcpFrameLength(wire[:240])
	require.NoError(t, err)
	require.Zero(t, n)
	n, err = dhcpFrameLength(wire)
	require.NoError(t, err)
	require.Equal(t, len(wire), n)
	bad := tnsConnect("abc")
	bad[26], bad[27] = 0xff, 0xff
	_, err = (&binTNS{}).consume(bad)
	require.Error(t, err)
}

func TestRound3UpstreamCaptureReplay(t *testing.T) {
	// Original bytes, source URLs, licenses and hashes are retained in the
	// shared protocol-corpus manifest; these are not generated session PCAPs.
	for _, tc := range []struct {
		protocol, capture string
		messages          int
		bytes             uint64
	}{
		{"smtp", "ndpi/ndpi-smtp.pcap", 73, 17955}, {"imap", "ndpi/ndpi-imap.pcap", 27, 1580},
		{"ftp", "ndpi/ndpi-ftp.pcap", 35, 1063}, {"modbus", "ndpi/ndpi-modbus.pcap", 102, 1173},
		{"radius", "tcpdump/tcpdump-radius.pcap", 4, 519}, {"ntp", "ndpi/ndpi-ntpv4.pcap", 1, 48},
		{"dhcp", "wireshark-tests/wireshark-dhcp.pcap", 4, 1144},
		{"coap", "ndpi/ndpi-coap.pcap", 819, 47512},
	} {
		for _, workers := range []int{1, 2} {
			t.Run(fmt.Sprintf("%s/workers=%d", tc.protocol, workers), func(t *testing.T) {
				events, stats, err := binReplay(t, binCorpusBytes(t, tc.capture), workers)
				require.NoError(t, err)
				count := 0
				var messageBytes uint64
				for _, event := range events {
					if event.Protocol == tc.protocol && event.Status == "decoded" {
						count++
						messageBytes += uint64(event.Length)
						require.NotNil(t, event.Session)
						require.NotEmpty(t, event.Raw)
					}
					if event.Status == "malformed" {
						t.Errorf("%s: %s", event.Protocol, event.Error)
					}
				}
				require.Equal(t, tc.messages, count)
				require.Equal(t, tc.bytes, messageBytes)
				require.Zero(t, stats.BufferedBytes)
				t.Logf("decoded %s=%d, bytes=%d", tc.protocol, count, messageBytes)
			})
		}
	}
}

func TestRound3SMTPGreetingBeyondProbeWindow(t *testing.T) {
	for _, chunk := range []int{0, 1, 7} {
		banner := append([]byte("220 mail.example ESMTP "), bytes.Repeat([]byte("x"), 70)...)
		banner = append(banner, '\r', '\n')
		events, _ := sessionTestFlow(t, "smtp", []sessionStep{{1, banner}, {0, mailCR("EHLO test")}, {1, mailCR("250 ready")}}, chunk, false)
		require.Len(t, events, 3)
		for _, e := range events {
			require.Empty(t, e.Error)
		}
		require.Equal(t, "EHLO", events[2].Session["In Reply To"])
	}
}

func TestRound3IMAPLiteralTrailer(t *testing.T) {
	for _, chunk := range []int{0, 1, 7} {
		steps := []sessionStep{{1, mailCR("* OK IMAP4rev1 ready")}, {0, mailCR("a1 FETCH 1 BODY[]")}, {1, []byte("* 1 FETCH (BODY[] {5}\r\nhello)\r\na1 OK fetched\r\n")}}
		events, _ := sessionTestFlow(t, "imap", steps, chunk, false)
		require.Len(t, events, 6)
		for _, e := range events {
			require.Empty(t, e.Error)
		}
		require.Equal(t, "Literal End", events[4].Session["Packet Name"])
		require.Equal(t, "FETCH", events[5].Session["In Reply To"])
	}
}
