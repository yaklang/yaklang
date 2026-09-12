package bin_parser

import (
	"encoding/binary"
	"os"
	"testing"

	"github.com/gopacket/gopacket/pcapgo"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

// Keep per-record test scheduling and assertions outside the timed region; the
// separate corpus test continues to check all 16,827 records and every field.
func BenchmarkProtocolCorpusIPX(b *testing.B) {
	file, err := os.Open("testdata/protocol-corpus/captures/google-samples/google-ipx-session.pcapng")
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()
	capture, err := pcapgo.NewNgReader(file, pcapgo.DefaultNgReaderOptions)
	if err != nil {
		b.Fatal(err)
	}
	frame, _, err := capture.ReadPacketData()
	if err != nil {
		b.Fatal(err)
	}
	if len(frame) < 47 {
		b.Fatal("short IPX reference frame")
	}
	length := int(binary.BigEndian.Uint16(frame[19:]))
	if length < 30 || 17+length > len(frame) {
		b.Fatal("invalid IPX reference boundary")
	}
	for _, test := range []struct {
		name, rule, entry string
		wire              []byte
	}{
		{"direct", "application-layer.extended_protocols", "IPX", frame[17 : 17+length]},
		{"standalone", "ipx", "IPX", frame[17 : 17+length]},
		{"ethernet", "ethernet", "Ethernet", frame},
	} {
		b.Run(test.name, func(b *testing.B) {
			parse := func() {
				reader := newProtocolCorpusBoundedReader(test.wire)
				n, err := parser.ParseBinary(reader, test.rule, test.entry)
				if err != nil {
					b.Fatal(err)
				}
				if reader.Len() != 0 || protocolCorpusFindNode(n, "Source Socket") == nil {
					b.Fatal("incomplete IPX field tree or input consumption")
				}
			}
			parse() // warm immutable rule and operator caches
			b.ReportAllocs()
			b.SetBytes(int64(len(test.wire)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				parse()
			}
		})
	}
}
