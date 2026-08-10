package sfvm

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/utils/omap"
)

type benchmarkIDValue struct {
	stubValueOperator
	id int64
}

func (v *benchmarkIDValue) GetId() int64 { return v.id }

// BenchmarkTakeSymbolSnapshot measures allocation of the per-check symbol
// snapshot, the #1 flat allocator in the Hadoop scan window (run14: ~32.8GB).
func BenchmarkTakeSymbolSnapshot(b *testing.B) {
	table := omap.NewEmptyOrderedMap[string, Values]()
	for i := 0; i < 1000; i++ {
		table.Set(fmt.Sprintf("key-%d", i), Values{
			&benchmarkIDValue{id: int64(i)},
			&benchmarkIDValue{id: int64(i + 100000)},
		})
	}

	var sink *SymbolSnapshot
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = TakeSymbolSnapshot(table)
	}
	_ = sink
}
