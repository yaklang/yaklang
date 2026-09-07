package bin_parser

import (
	"encoding/json"
	"testing"

	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// Cost decomposition only. Unlike the end-to-end benchmark these subtests
// reuse prepared trees/projections, and must never be reported as its speedup.
func BenchmarkCurrentCorpusStages(b *testing.B) {
	works := currentCorpusWorks(b, true)
	nodes := make([]*base.Node, len(works))
	values := make([]any, len(works))
	for i, w := range works {
		var err error
		nodes[i], err = parser.ParseBinary(newProtocolCorpusBoundedReader(w.wire), w.rule, w.entry)
		if err != nil {
			b.Fatal(err)
		}
		values[i] = map[string]any{"fields": NodeToMap(nodes[i]), "metadata": nodes[i].Cfg.GetItem("additionInfo")}
	}
	b.Run("Projection", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for _, n := range nodes {
				if NodeToMap(n) == nil {
					b.Fatal("empty projection")
				}
			}
		}
	})
	b.Run("Serialization", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for _, v := range values {
				if _, err := json.Marshal(v); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}

// This diagnostic isolates each public parse, without the batch scheduler. The
// original complete batch remains the performance acceptance workload.
func BenchmarkCurrentCorpusMessageCosts(b *testing.B) {
	for _, w := range currentCorpusWorks(b, true) {
		b.Run(w.id+"/"+w.entry, func(b *testing.B) {
			if _, err := currentCorpusParse(w, false); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := currentCorpusParse(w, false); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
