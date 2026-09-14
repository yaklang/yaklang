package pcaputil

import (
	"bufio"
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

// Each case uses the complete file -> decoder -> TCP -> owned chunk callback
// path. Packet size materially changes the payload bandwidth per packet/s.
func BenchmarkOpenPcapPacketSize(b *testing.B) {
	for _, size := range []int{64, 1460, 32768} {
		b.Run(fmt.Sprintf("payload=%d", size), func(b *testing.B) {
			packets := (16 << 20) / size
			name := filepath.Join(b.TempDir(), "sized.pcap")
			file, err := os.Create(name)
			if err != nil {
				b.Fatal(err)
			}
			buffered := bufio.NewWriterSize(file, 256<<10)
			w := pcapgo.NewWriter(buffered)
			if err := w.WriteFileHeader(65535, layers.LinkTypeEthernet); err != nil {
				b.Fatal(err)
			}
			eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{0, 1, 2, 3, 4, 5}, DstMAC: net.HardwareAddr{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4}
			ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.IPv4(192, 0, 2, 1), DstIP: net.IPv4(192, 0, 2, 2)}
			tcp := &layers.TCP{SrcPort: 12345, DstPort: 80, SYN: true}
			if err := tcp.SetNetworkLayerForChecksum(ip); err != nil {
				b.Fatal(err)
			}
			payload := make([]byte, size)
			buf := gopacket.NewSerializeBuffer()
			for i := -1; i < packets; i++ {
				var data []byte
				if i >= 0 {
					tcp.SYN, tcp.ACK, tcp.Seq, data = false, true, 1+uint32(i*size), payload
				}
				buf.Clear()
				if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, tcp, gopacket.Payload(data)); err != nil {
					b.Fatal(err)
				}
				raw := buf.Bytes()
				if err := w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Unix(1700000000, int64(i+1)*1000), CaptureLength: len(raw), Length: len(raw)}, raw); err != nil {
					b.Fatal(err)
				}
			}
			if err := buffered.Flush(); err != nil {
				b.Fatal(err)
			}
			if err := file.Close(); err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(packets * size))
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				received := 0
				err := OpenPcapFile(name, WithTCPReassemblyStream(64<<10), WithOnTrafficFlowOnDataFrameReassembled(func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { received += len(f.Payload) }))
				if err != nil {
					b.Fatal(err)
				}
				if received != packets*size {
					b.Fatalf("received %d, want %d", received, packets*size)
				}
			}
		})
	}
}
