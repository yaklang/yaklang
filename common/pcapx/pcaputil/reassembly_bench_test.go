package pcaputil

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
)

// These benchmarks deliberately use the public Feed and OpenPcapFile paths.
// Keep their inputs unchanged when comparing against the parent revision.
func BenchmarkTrafficPool(b *testing.B) {
	for _, reverse := range []bool{false, true} {
		b.Run(fmt.Sprintf("reverse=%v", reverse), func(b *testing.B) {
			const packets = 4096
			payload := make([]byte, 1024)
			ip := &layers.IPv4{SrcIP: net.IPv4(10, 0, 0, 1), DstIP: net.IPv4(10, 0, 0, 2)}
			ts := time.Unix(1700000000, 0)
			b.SetBytes(packets * int64(len(payload)))
			b.ReportAllocs()
			for n := 0; n < b.N; n++ {
				p := NewTrafficPool(context.Background())
				var received int
				p.onFlowFrameDataFrameArrived = append(p.onFlowFrameDataFrameArrived, func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { received += len(f.Payload) })
				tcp := &layers.TCP{SrcPort: 12345, DstPort: 80, SYN: true, Seq: 0}
				p.Feed(nil, ip, tcp, ts)
				tcp.SYN, tcp.ACK, tcp.Seq = false, true, 1
				p.Feed(nil, ip, tcp, ts)
				tcp.Payload = payload
				// Establish the expected sequence before the out-of-order burst.
				p.Feed(nil, ip, tcp, ts)
				for i := 1; i < packets; i++ {
					index := i
					if reverse {
						index = packets - i
					}
					tcp.Seq = 1 + uint32(index*len(payload))
					p.Feed(nil, ip, tcp, ts)
				}
				if received != packets*len(payload) {
					b.Fatalf("received %d bytes", received)
				}
				p.flowCache.ForEach(func(_ string, f *TrafficFlow) { f.Close() })
				p.flowCache.Close()
			}
		})
	}
}

func benchmarkCaptureFile(b *testing.B, packets int) string {
	b.Helper()
	name := filepath.Join(b.TempDir(), "tcp.pcap")
	f, err := os.Create(name)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	w := pcapgo.NewWriter(f)
	if err := w.WriteFileHeader(65535, layers.LinkTypeEthernet); err != nil {
		b.Fatal(err)
	}
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.IPv4(10, 0, 0, 1), DstIP: net.IPv4(10, 0, 0, 2)}
	tcp := &layers.TCP{SrcPort: 12345, DstPort: 80, SYN: true, Seq: 0, Window: 65535}
	if err := tcp.SetNetworkLayerForChecksum(ip); err != nil {
		b.Fatal(err)
	}
	payload := make([]byte, 1024)
	for i := -2; i < packets; i++ {
		var data []byte
		if i >= -1 {
			tcp.SYN, tcp.ACK, tcp.Seq = false, true, 1
		}
		if i >= 0 {
			tcp.Seq = 1 + uint32(i*len(payload))
			data = payload
		}
		buf := gopacket.NewSerializeBuffer()
		if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp, gopacket.Payload(data)); err != nil {
			b.Fatal(err)
		}
		raw := buf.Bytes()
		if err := w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, int64(i+2)*1000), CaptureLength: len(raw), Length: len(raw)}, raw); err != nil {
			b.Fatal(err)
		}
	}
	return name
}

func BenchmarkOpenPcapFile(b *testing.B) {
	const packets = 4096
	name := benchmarkCaptureFile(b, packets)
	b.SetBytes(packets * 1024)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		var received int
		err := OpenPcapFile(name, WithOnTrafficFlowOnDataFrameArrived(func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { received += len(f.Payload) }))
		if err != nil {
			b.Fatal(err)
		}
		if received != packets*1024 {
			b.Fatalf("received %d bytes", received)
		}
	}
}

func BenchmarkTrafficPoolManyFlows(b *testing.B) {
	const flows, packets = 1024, 4
	ip := &layers.IPv4{SrcIP: net.IPv4(10, 0, 0, 1), DstIP: net.IPv4(10, 0, 0, 2)}
	payload := make([]byte, 1024)
	ts := time.Unix(1700000000, 0)
	b.SetBytes(flows * packets * 1024)
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		p := NewTrafficPool(context.Background())
		var received int
		p.onFlowFrameDataFrameArrived = append(p.onFlowFrameDataFrameArrived, func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { received += len(f.Payload) })
		for i := 0; i < flows; i++ {
			p.Feed(nil, ip, &layers.TCP{SrcPort: layers.TCPPort(10000 + i), DstPort: 80, SYN: true}, ts)
		}
		for packet := 0; packet < packets; packet++ {
			for i := 0; i < flows; i++ {
				tcp := &layers.TCP{SrcPort: layers.TCPPort(10000 + i), DstPort: 80, Seq: 1 + uint32(packet*len(payload)), ACK: true}
				tcp.Payload = payload
				p.Feed(nil, ip, tcp, ts)
			}
		}
		if received != flows*packets*len(payload) {
			b.Fatalf("received %d bytes", received)
		}
		p.flowCache.ForEach(func(_ string, f *TrafficFlow) { f.Close() })
		p.flowCache.Close()
	}
}
