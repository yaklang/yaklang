package bin_parser

import (
	"bytes"
	"testing"

	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

// Compare only the shared consumed-length traversal in the same binary. Keep
// AllJoyn's rule-local remaining-length implementation enabled in both modes;
// its independent legacy switch must not contaminate this comparison.
func BenchmarkParserConsumedLength(b *testing.B) {
	for _, fixture := range []struct {
		name string
		wire []byte
	}{
		{"ordinary-184B", alljoynTestFixtures(b)[2]},
		{"counts-1454B", alljoynTestHighCountWire()},
	} {
		for _, entry := range []string{"AllJoynNS", "AllJoynNSCarrier"} {
			for _, legacy := range []bool{false, true} {
				mode := "DirectLookup"
				if legacy {
					mode = "LegacyTraversal"
				}
				b.Run(fixture.name+"/"+entry+"/"+mode, func(b *testing.B) {
					config := map[string]any{
						"parseConsumedLengthLegacy": legacy,
						"alljoynNSLegacyLengths":    false,
					}
					parse := func() {
						reader := newProtocolCorpusBoundedReader(fixture.wire)
						node, err := parser.ParseBinaryWithConfig(reader, alljoynCorpusRule, config, entry)
						if err != nil {
							b.Fatal(err)
						}
						if _, err = node.Result(); err != nil {
							b.Fatal(err)
						}
						if !bytes.Equal(fixture.wire, NodeToBytes(node)) {
							b.Fatal("consumed-length mode changed AllJoyn bytes")
						}
						if reader.Len() != 0 {
							b.Fatalf("consumed-length mode left %d unread bytes", reader.Len())
						}
					}
					// Prewarm the complete public path, including Result output
					// expressions and byte-preservation checks, before measuring it.
					parse()
					b.ReportAllocs()
					b.SetBytes(int64(len(fixture.wire)))
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						parse()
					}
				})
			}
		}
	}
}
