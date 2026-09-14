package pcaputil

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/shirou/gopsutil/v4/process"
)

// A reproducible synthetic wire mix, not a claim about the real Internet:
// 24 flows, MQTT/HTTP/TLS, 512-byte application bodies, 128 rounds, with SYNs,
// automatic detection and full structured outputs. No rule/entry hints.
func binBenchmarkCapture(b testing.TB, mix bool) ([]byte, int64, uint64) {
	b.Helper()
	var out bytes.Buffer
	w := pcapgo.NewWriter(&out)
	if err := w.WriteFileHeader(65535, layers.LinkTypeEthernet); err != nil {
		b.Fatal(err)
	}
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.IP{192, 0, 2, 1}, DstIP: net.IP{192, 0, 2, 2}}
	tcp := &layers.TCP{DstPort: 1883}
	if err := tcp.SetNetworkLayerForChecksum(ip); err != nil {
		b.Fatal(err)
	}
	buf := gopacket.NewSerializeBuffer()
	publish := binMQTTPublish(512)
	http := []byte("POST /events HTTP/1.1\r\nHost: example.test\r\nContent-Length: 512\r\n\r\n" + string(bytes.Repeat([]byte{'x'}, 512)))
	tls := append([]byte{23, 3, 3, 2, 0}, bytes.Repeat([]byte{'x'}, 512)...)
	var seq [24]uint32
	var total int64
	var messages uint64
	for round := -1; round < 128; round++ {
		for flow := 0; flow < 24; flow++ {
			tcp.SrcPort = layers.TCPPort(10000 + flow)
			tcp.SYN = round < 0
			tcp.ACK = round >= 0
			tcp.Seq = seq[flow]
			var payload []byte
			if round >= 0 {
				payload = publish
				if mix && flow%3 == 1 {
					payload = http
				} else if mix && flow%3 == 2 {
					payload = tls
				} else if round == 0 {
					payload = append(append([]byte{}, binMQTTConnect...), publish...)
					messages++
				}
				messages++
				seq[flow] += uint32(len(payload))
				total += int64(len(payload))
			} else {
				seq[flow]++
			}
			buf.Clear()
			if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp, gopacket.Payload(payload)); err != nil {
				b.Fatal(err)
			}
			raw := buf.Bytes()
			if err := w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, int64((round+1)*24+flow)*1000), CaptureLength: len(raw), Length: len(raw)}, raw); err != nil {
				b.Fatal(err)
			}
		}
	}
	return out.Bytes(), total, messages
}

func BenchmarkPcapBinParser(b *testing.B) {
	for _, mix := range []bool{false, true} {
		pcap, payload, want := binBenchmarkCapture(b, mix)
		for _, mode := range []string{"reassembly", "full", "deferred-view"} {
			for _, workers := range []int{1, 2, 4} {
				b.Run(fmt.Sprintf("mixed=%v/%s/workers=%d", mix, mode, workers), func(b *testing.B) {
					old := runtime.GOMAXPROCS(workers)
					defer runtime.GOMAXPROCS(old)
					b.SetBytes(payload)
					b.ReportAllocs()
					proc, err := process.NewProcess(int32(os.Getpid()))
					if err != nil {
						b.Fatal(err)
					}
					cpu, err := proc.Times()
					if err != nil {
						b.Fatal(err)
					}
					b.ResetTimer()
					for n := 0; n < b.N; n++ {
						var count atomic.Uint64
						var stats BinParserStats
						opts := []CaptureOption{WithTCPReassemblyWorkers(workers)}
						if mode == "reassembly" {
							opts = append(opts, WithTCPReassemblyStream(64<<10), WithOnTrafficFlowOnDataFrameArrived(func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { count.Add(uint64(len(f.Payload))) }))
						} else {
							callback := func(e *BinParserEvent) {
								if e.Status != "decoded" || e.Structured == nil {
									panic(fmt.Sprintf("unexpected %s: %s %s", e.Status, e.Summary, e.Error))
								}
								count.Add(1)
							}
							if mode == "deferred-view" {
								view, _ := NewBinParserInspector(2048, 2<<20)
								callback = view.OnEvent
								opts = append(opts, WithBinParserDeferred(true))
							}
							opts = append(opts, WithBinParser(callback), WithBinParserStats(func(s BinParserStats) { stats = s }))
						}
						if err := ReplayPcap(bytes.NewReader(pcap), opts...); err != nil {
							b.Fatal(err)
						}
						if mode == "reassembly" {
							if count.Load() != uint64(payload) {
								b.Fatal("missing reassembled bytes")
							}
						} else if stats.Messages != want || stats.Malformed != 0 || stats.Unknown != 0 || stats.Incomplete != 0 {
							b.Fatalf("incomplete analysis: %+v want=%d", stats, want)
						}
					}
					b.StopTimer()
					end, err := proc.Times()
					if err != nil {
						b.Fatal(err)
					}
					b.ReportMetric((end.User+end.System-cpu.User-cpu.System)*1e3/float64(b.N), "cpu-ms/batch")
				})
			}
		}
	}
}
