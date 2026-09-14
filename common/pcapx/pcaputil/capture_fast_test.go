package pcaputil

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func makeTestCapture(t *testing.T, kind string) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), kind+".pcap")
	f, err := os.Create(name)
	require.NoError(t, err)
	defer f.Close()
	w := pcapgo.NewWriter(f)
	link := layers.LinkTypeEthernet
	if kind == "raw" {
		link = layers.LinkTypeRaw
	}
	require.NoError(t, w.WriteFileHeader(65535, link))
	for _, step := range []tcpStep{{seq: 99, syn: true}, {seq: 103, data: "def"}, {seq: 100, data: "abc"}, {seq: 106, data: "ghi", fin: true}} {
		eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
		ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.IPv4(192, 0, 2, 1), DstIP: net.IPv4(192, 0, 2, 2)}
		tcp := &layers.TCP{SrcPort: 12345, DstPort: 80, Seq: step.seq, SYN: step.syn, FIN: step.fin, ACK: !step.syn}
		tcp.SetNetworkLayerForChecksum(ip)
		var network gopacket.SerializableLayer = ip
		if kind == "ipv6" {
			eth.EthernetType = layers.EthernetTypeIPv6
			ip6 := &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: layers.IPProtocolTCP, SrcIP: net.ParseIP("2001:db8::1"), DstIP: net.ParseIP("2001:db8::2")}
			network = ip6
			tcp.SetNetworkLayerForChecksum(ip6)
		}
		serial := []gopacket.SerializableLayer{eth, network, tcp, gopacket.Payload(step.data)}
		if kind == "raw" {
			serial = serial[1:]
		}
		if kind == "vlan" {
			eth.EthernetType = layers.EthernetTypeDot1Q
			serial = []gopacket.SerializableLayer{eth, &layers.Dot1Q{VLANIdentifier: 42, Type: layers.EthernetTypeIPv4}, ip, tcp, gopacket.Payload(step.data)}
		}
		buf := gopacket.NewSerializeBuffer()
		require.NoError(t, gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, serial...))
		raw := buf.Bytes()
		require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(int64(step.seq), 0), CaptureLength: len(raw), Length: len(raw)}, raw))
	}
	return name
}

func TestOfflineDecoderParity(t *testing.T) {
	for _, kind := range []string{"ipv4", "ipv6", "vlan", "raw"} {
		t.Run(kind, func(t *testing.T) {
			name := makeTestCapture(t, kind)
			type frame struct {
				Payload   string
				Seq       uint32
				Timestamp time.Time
				Hash      string
			}
			var results [2][]frame
			for path := 0; path < 2; path++ {
				opts := []CaptureOption{WithOnTrafficFlowOnDataFrameArrived(func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) {
					results[path] = append(results[path], frame{string(f.Payload), f.Seq, f.Timestamp, f.ConnHash})
				})}
				var packets []gopacket.Packet
				if path == 1 {
					opts = append(opts, WithEveryPacket(func(p gopacket.Packet) { packets = append(packets, p) }))
				}
				require.NoError(t, OpenPcapFile(name, opts...))
				if path == 1 {
					require.Len(t, packets, 4)
					require.Equal(t, uint32(99), packets[0].TransportLayer().(*layers.TCP).Seq)
				}
			}
			require.Equal(t, results[1], results[0])
			require.Len(t, results[0], 3)
			require.Equal(t, "abc", results[0][0].Payload)
			require.Equal(t, "def", results[0][1].Payload)
			require.Equal(t, "ghi", results[0][2].Payload)
		})
	}
}

func TestOfflineDecoderTruncatedAndNonTCP(t *testing.T) {
	p := NewTrafficPool(context.Background())
	defer p.Close()
	conf := NewDefaultConfig()
	conf.trafficPool = p
	d := offlineDecoder{conf: conf, link: layers.LinkTypeEthernet}
	// Truncated Ethernet/IP/TCP headers must never reuse the previous TCP layer.
	for size := 0; size < 54; size++ {
		raw := make([]byte, size)
		if size >= 14 {
			binary.BigEndian.PutUint16(raw[12:14], uint16(layers.EthernetTypeIPv4))
		}
		if size >= 34 {
			raw[14], raw[23] = 0x45, 6
			binary.BigEndian.PutUint16(raw[16:18], 40)
		}
		d.feed(context.Background(), raw, gopacket.CaptureInfo{})
	}
	require.Zero(t, p.flowCache.Count())
}

func TestCaptureOptionsAndErrors(t *testing.T) {
	require.Error(t, OpenPcapFile(filepath.Join(t.TempDir(), "missing.pcap")))
	require.Error(t, Start(WithTCPReassemblyStream(0)))
	require.Error(t, Start(WithTCPReassemblyOptions(TCPReassemblyOptions{MaxFlows: -1})))
	httpOpt := WithHTTPFlow(func(*TrafficFlow, *http.Request, *http.Response) {})
	require.ErrorContains(t, Start(WithTCPReassemblyStream(1024), httpOpt), "HTTP/TLS")
	require.ErrorContains(t, Start(httpOpt, WithTCPReassemblyStream(1024)), "HTTP/TLS")
	name := makeTestCapture(t, "ipv4")
	var chunks []*TrafficFrame
	require.NoError(t, OpenPcapFile(name, WithTCPReassemblyStream(2), WithOnTrafficFlowOnDataFrameReassembled(func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { chunks = append(chunks, f) })))
	require.Equal(t, "abcdefghi", frameData(chunks))
	for _, chunk := range chunks {
		require.LessOrEqual(t, len(chunk.Payload), 2)
		require.True(t, chunk.Done)
	}
	// A libpcap read error after a valid header must be reported to the caller.
	f, err := os.OpenFile(name, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = f.Write([]byte{1, 2, 3})
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.Error(t, OpenPcapFile(name))
}

func TestOfflineDecoderCallbackPanic(t *testing.T) {
	name := makeTestCapture(t, "ipv4")
	require.NoError(t, OpenPcapFile(name, WithOnTrafficFlowOnDataFrameArrived(func(*TrafficFlow, *TrafficConnection, *TrafficFrame) { panic("test callback") })))
}

func TestLargePcapFileStream(t *testing.T) {
	if os.Getenv("PCAPX_LARGE_TEST") != "1" {
		t.Skip("set PCAPX_LARGE_TEST=1 to generate and replay a 1 GiB pcap")
	}
	const total = uint64(1<<30) + 17
	workers := 1
	if value := os.Getenv("PCAPX_LARGE_WORKERS"); value != "" {
		var err error
		workers, err = strconv.Atoi(value)
		require.NoError(t, err)
	}
	name := filepath.Join(t.TempDir(), "large.pcap")
	f, err := os.Create(name)
	require.NoError(t, err)
	buffered := bufio.NewWriterSize(f, 1<<20)
	w := pcapgo.NewWriter(buffered)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.IPv4(192, 0, 2, 1), DstIP: net.IPv4(192, 0, 2, 2)}
	tcp := &layers.TCP{SrcPort: 12345, DstPort: 80, ACK: true}
	require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
	payload := make([]byte, 32<<10)
	buf := gopacket.NewSerializeBuffer()
	want, got := sha256.New(), sha256.New()
	for offset := uint64(0); offset < total; {
		n := uint64(len(payload))
		if n > total-offset {
			n = total - offset
		}
		binary.BigEndian.PutUint64(payload, offset)
		want.Write(payload[:n])
		tcp.Seq = uint32(offset)
		require.NoError(t, buf.Clear())
		require.NoError(t, gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp, gopacket.Payload(payload[:n])))
		raw := buf.Bytes()
		require.NoError(t, w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, int64(offset)), CaptureLength: len(raw), Length: len(raw)}, raw))
		offset += n
	}
	require.NoError(t, buffered.Flush())
	require.NoError(t, f.Close())
	var received, first, peak uint64
	started := time.Now()
	require.NoError(t, OpenPcapFile(name, WithTCPReassemblyWorkers(workers), WithTCPReassemblyStream(64<<10), WithOnTrafficFlowOnDataFrameReassembled(func(_ *TrafficFlow, _ *TrafficConnection, frame *TrafficFrame) {
		got.Write(frame.Payload)
		received += uint64(len(frame.Payload))
		if received%(256<<20) == 0 {
			runtime.GC()
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			if first == 0 {
				first = m.HeapAlloc
			}
			if m.HeapAlloc > peak {
				peak = m.HeapAlloc
			}
			t.Logf("pcap_payload=%d heap_live=%d", received, m.HeapAlloc)
		}
	})))
	require.Equal(t, total, received)
	require.Equal(t, want.Sum(nil), got.Sum(nil))
	require.Less(t, peak-first, uint64(16<<20))
	t.Logf("pcap_payload=%d replay=%s SHA256=%x live_heap_growth=%d", received, time.Since(started), got.Sum(nil), peak-first)
}
