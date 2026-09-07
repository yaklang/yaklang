package bin_parser

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

func BenchmarkDICOMUserInformation(b *testing.B) {
	benchmarkDICOMUserInformation(b, false)
}

func BenchmarkDICOMUserInformationLegacy(b *testing.B) {
	benchmarkDICOMUserInformation(b, true)
}

// Match the existing resource-limit fixtures: seven required association,
// syntax and user items, plus count-7 empty extension items. Both paths build
// the complete field tree; never lower the 4096-item limit for a fast test.
func benchmarkDICOMUserInformation(b *testing.B, legacy bool) {
	for _, count := range []int{128, 512, 1024, 2048, 4096} {
		b.Run(fmt.Sprintf("items-%d", count), func(b *testing.B) {
			config := map[string]any{"dicomUserLegacy": legacy}
			user := append(dicomTestUser(), bytes.Repeat(dicomTestItem(0xee, nil), count-7)...)
			wire := dicomTestAssociation(1, dicomTestContext(0x20, 1, 0), user)
			warm := dicomTestAssociation(1, dicomTestContext(0x20, 1, 0), dicomTestUser())
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
				if got := len(protocolCorpusNodesNamed(node, "User Item")); got != count-5 {
					b.Fatalf("decoded %d user items, want %d", got, count-5)
				}
				if got := node.Ctx.GetItem("dicomItemCount"); got != count {
					b.Fatalf("total association item count %v, want %d", got, count)
				}
			}
		})
	}
}
