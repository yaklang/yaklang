package pcaputil

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/require"
)

// A constructed stress mix, not original packet order or an Internet traffic
// distribution: 1,024 new TLS flows (real ClientHello plus application data),
// eight HTTP and eight MQTT flows, and 1,024 real DNS datagrams. Preparation is
// outside timing. No protocol/entry hints are supplied to the measured replay.
func binHandshakeCapture(t testing.TB) ([]byte, int64, uint64) {
	t.Helper()
	var hello []byte
	err := ReplayPcap(bytes.NewReader(binCorpusBytes(t, "ndpi/ndpi-http-connect.pcap")), WithBinParserDeferred(true), WithBinParser(func(e *BinParserEvent) {
		if hello == nil && e.Protocol == "tls" && len(e.Raw) > 9 && e.Raw[0] == 22 && e.Raw[5] == 1 {
			hello = bytes.Clone(e.Raw)
		}
	}))
	require.True(t, err == nil || strings.Contains(err.Error(), "TCP SYN changes an established initial sequence number"), "%v", err)
	require.Len(t, hello, 517)
	var dns [][]byte
	require.NoError(t, ReplayPcap(bytes.NewReader(binCorpusBytes(t, "ndpi/ndpi-dns.pcap")), WithBinParserDeferred(true), WithBinParser(func(e *BinParserEvent) {
		if e.Protocol == "dns" {
			dns = append(dns, bytes.Clone(e.Raw))
		}
	})))
	require.NotEmpty(t, dns)
	var out bytes.Buffer
	w := pcapgo.NewWriter(&out)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, SrcIP: net.IP{192, 0, 2, 1}, DstIP: net.IP{192, 0, 2, 2}}
	buf := gopacket.NewSerializeBuffer()
	var packets, messages uint64
	var payload int64
	write := func(layer gopacket.SerializableLayer, data []byte) {
		buf.Clear()
		require.NoError(t, gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, layer, gopacket.Payload(data)))
		raw := buf.Bytes()
		require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, int64(packets)*1000), CaptureLength: len(raw), Length: len(raw)}, raw))
		packets++
		payload += int64(len(data))
	}
	tcp := &layers.TCP{DstPort: 1883} // Deliberately share the port across protocols.
	require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
	seq := make(map[layers.TCPPort]uint32)
	writeTCP := func(port layers.TCPPort, data []byte, pdus uint64) {
		ip.Protocol = layers.IPProtocolTCP
		tcp.SrcPort = port
		if _, exists := seq[port]; !exists {
			tcp.SYN, tcp.ACK, tcp.Seq = true, false, 0
			write(tcp, nil)
			seq[port] = 1
		}
		tcp.SYN, tcp.ACK, tcp.Seq = false, true, seq[port]
		write(tcp, data)
		seq[port] += uint32(len(data))
		messages += pdus
	}
	udp := &layers.UDP{SrcPort: 30000, DstPort: 53}
	require.NoError(t, udp.SetNetworkLayerForChecksum(ip))
	http := []byte("POST /events HTTP/1.1\r\nHost: example.test\r\nContent-Length: 512\r\n\r\n" + strings.Repeat("x", 512))
	app := append([]byte{23, 3, 3, 2, 0}, bytes.Repeat([]byte{'x'}, 512)...)
	publish := binMQTTPublish(512)
	for round := 0; round < 128; round++ {
		for slot := 0; slot < 8; slot++ {
			port := layers.TCPPort(20000 + round*8 + slot)
			writeTCP(port, hello, 1)
			writeTCP(port, app, 1)
			writeTCP(layers.TCPPort(10000+slot), http, 1)
			mqtt, count := publish, uint64(1)
			if round == 0 {
				mqtt = append(bytes.Clone(binMQTTConnect), publish...)
				count++
			}
			writeTCP(layers.TCPPort(11000+slot), mqtt, count)
			ip.Protocol = layers.IPProtocolUDP
			write(udp, dns[(round*8+slot)%len(dns)])
			messages++
		}
	}
	return out.Bytes(), payload, messages
}

func BenchmarkPcapBinParserHandshakeMix(b *testing.B) {
	wire, payload, want := binHandshakeCapture(b)
	for _, workers := range []int{1, 2, 4} {
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			old := runtime.GOMAXPROCS(workers)
			defer runtime.GOMAXPROCS(old)
			proc, err := process.NewProcess(int32(os.Getpid()))
			require.NoError(b, err)
			cpu, err := proc.Times()
			require.NoError(b, err)
			b.SetBytes(payload)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var count atomic.Uint64
				var stats BinParserStats
				err := ReplayPcap(bytes.NewReader(wire), WithTCPReassemblyWorkers(workers), WithBinParser(func(e *BinParserEvent) {
					if e.Status != "decoded" || e.Structured == nil {
						panic(fmt.Sprintf("unexpected %s: %s", e.Status, e.Error))
					}
					count.Add(1)
				}), WithBinParserStats(func(s BinParserStats) { stats = s }))
				require.NoError(b, err)
				require.Equal(b, want, count.Load())
				require.EqualValues(b, payload, stats.MessageBytes)
				require.EqualValues(b, payload, stats.InputBytes)
				require.Zero(b, stats.Unknown+stats.Malformed+stats.Incomplete)
				require.Zero(b, stats.BufferedBytes)
			}
			b.StopTimer()
			end, err := proc.Times()
			require.NoError(b, err)
			b.ReportMetric((end.User+end.System-cpu.User-cpu.System)*1e3/float64(b.N), "cpu-ms/batch")
			b.ReportMetric(float64(want), "messages/batch")
		})
	}
}

func TestBinParserPacedHandshakeReplay(t *testing.T) {
	if os.Getenv("PCAPX_BIN_RATE") != "1" {
		t.Skip("set PCAPX_BIN_RATE=1 for a 20-second 150 Mbps replay with new TLS flows and DNS")
	}
	wire, payload, want := binHandshakeCapture(t)
	binPacedReplay(t, wire, payload, want)
}
