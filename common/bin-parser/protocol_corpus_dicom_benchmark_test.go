package bin_parser

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

// Keep the original 4,096-element resource boundary in the benchmark. Parsing
// always includes every PDV and its length/context/control fields; the count
// check also prevents a fast implementation from silently dropping elements.
func BenchmarkDICOMPDVList(b *testing.B) {
	benchmarkDICOMPDVList(b, false)
}

// Retain a same-binary oracle for reproducible end-to-end comparisons. The
// parser, result-window code, input and field-count checks are otherwise equal.
func BenchmarkDICOMPDVListLegacy(b *testing.B) {
	benchmarkDICOMPDVList(b, true)
}

func benchmarkDICOMPDVList(b *testing.B, legacy bool) {
	for _, count := range []int{128, 512, 1024, 2048, 4096} {
		b.Run(fmt.Sprintf("items-%d", count), func(b *testing.B) {
			config := map[string]any{"dicomPDVLegacy": legacy}
			wire := dicomTestPDU(4, bytes.Repeat(dicomTestPDV(1, 0, nil), count))
			warm := dicomTestPDU(4, dicomTestPDV(1, 0, nil))
			if _, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(warm), dicomRule, config, "DICOM"); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(wire)))
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				node, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire), dicomRule, config, "DICOM")
				if err != nil {
					b.Fatal(err)
				}
				if got := len(protocolCorpusNodesNamed(node, "PDV")); got != count {
					b.Fatalf("decoded %d PDVs, want %d", got, count)
				}
			}
		})
	}
}
