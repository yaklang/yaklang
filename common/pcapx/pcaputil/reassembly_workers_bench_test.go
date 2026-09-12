package pcaputil

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"hash"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
)

func workerBenchmarkFile(b *testing.B, flows, packets, size int) string {
	b.Helper()
	name := filepath.Join(b.TempDir(), "workers.pcap")
	f, err := os.Create(name)
	if err != nil {
		b.Fatal(err)
	}
	buffered := bufio.NewWriterSize(f, 1<<20)
	w := pcapgo.NewWriter(buffered)
	if err := w.WriteFileHeader(65535, layers.LinkTypeEthernet); err != nil {
		b.Fatal(err)
	}
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.IP{192, 0, 2, 1}, DstIP: net.IP{192, 0, 2, 2}}
	tcp := &layers.TCP{DstPort: 80}
	if err := tcp.SetNetworkLayerForChecksum(ip); err != nil {
		b.Fatal(err)
	}
	payload := make([]byte, size)
	buf := gopacket.NewSerializeBuffer()
	for part := -1; part < packets; part++ {
		for flow := 0; flow < flows; flow++ {
			tcp.SrcPort = layers.TCPPort(10000 + flow)
			tcp.SYN = part < 0
			tcp.ACK = part >= 0
			tcp.Seq = 0
			var data []byte
			if part >= 0 {
				tcp.Seq = 1 + uint32(part*size)
				data = payload
			}
			buf.Clear()
			if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp, gopacket.Payload(data)); err != nil {
				b.Fatal(err)
			}
			raw := buf.Bytes()
			if err := w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, int64((part+1)*flows+flow)*1000), CaptureLength: len(raw), Length: len(raw)}, raw); err != nil {
				b.Fatal(err)
			}
		}
	}
	if err := buffered.Flush(); err != nil {
		b.Fatal(err)
	}
	if err := f.Close(); err != nil {
		b.Fatal(err)
	}
	return name
}

func BenchmarkTCPWorkers(b *testing.B) {
	benchmarkTCPWorkers(b, 64)
}

func BenchmarkTCPWorkersSingleFlow(b *testing.B) { benchmarkTCPWorkers(b, 1) }

func benchmarkTCPWorkers(b *testing.B, flows int) {
	const size = 1460
	packets := 16384 / flows
	name := workerBenchmarkFile(b, flows, packets, size)
	wantHash := sha256.New()
	for i := 0; i < packets; i++ {
		wantHash.Write(make([]byte, size))
	}
	want := wantHash.Sum(nil)
	for _, digest := range []bool{false, true} {
		for _, workers := range []int{1, 2, 4, 8} {
			b.Run(fmt.Sprintf("sha256=%v/workers=%d", digest, workers), func(b *testing.B) {
				b.SetBytes(int64(flows * packets * size))
				b.ReportAllocs()
				for n := 0; n < b.N; n++ {
					var received atomic.Uint64
					hashes := make([]hash.Hash, flows)
					if digest {
						for i := range hashes {
							hashes[i] = sha256.New()
						}
					}
					err := OpenPcapFile(name, WithTCPReassemblyWorkers(workers), WithTCPReassemblyStream(64<<10), WithOnTrafficFlowOnDataFrameReassembled(func(_ *TrafficFlow, c *TrafficConnection, f *TrafficFrame) {
						if digest {
							hashes[c.LocalPort()-10000].Write(f.Payload)
						}
						received.Add(uint64(len(f.Payload)))
					}))
					if err != nil {
						b.Fatal(err)
					}
					if received.Load() != uint64(flows*packets*size) {
						b.Fatal("incomplete reassembly", received.Load())
					}
					if digest {
						for _, h := range hashes {
							if h == nil {
								b.Fatal("missing hash")
							}
							if !bytes.Equal(want, h.Sum(nil)) {
								b.Fatal("reassembled hash mismatch")
							}
						}
					}
				}
			})
		}
	}
}
