package ssaapi

import (
	"testing"

	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// BenchmarkSFCheck_O3_EmptyCreateCheck measures the allocation of creating an
// EMPTY sfCheck (no sub-rules) — the hot path that eagerly built an
// originalSnapshot before O3. After O3, empty checks skip the snapshot build
// entirely (the 43M-call / 42GB hadoop hotspot).
func BenchmarkSFCheck_O3_EmptyCreateCheck(b *testing.B) {
	cfg := sf.NewConfig()
	ctx := sf.NewSFResult(nil, cfg)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CreateCheck(ctx, cfg)
	}
}

// BenchmarkSFCheck_O3_NonEmptyCreateCheck measures the allocation of creating a
// check WITH a sub-rule item (which builds the snapshot once). This documents
// the retained cost for checks that actually consume the snapshot.
func BenchmarkSFCheck_O3_NonEmptyCreateCheck(b *testing.B) {
	cfg := sf.NewConfig()
	ctx := sf.NewSFResult(nil, cfg)
	prog := NewTmpProgram("o3-bench")
	inst := ssa.NewConst(int64(1))
	inst.SetId(1)
	v, err := prog.NewValue(inst)
	if err != nil {
		b.Fatal(err)
	}
	ctx.SymbolTable.Set("src", sf.Values{v})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		check := CreateCheck(ctx, cfg)
		check.AppendItems(&sf.RecursiveConfigItem{
			Key:            string(sf.RecursiveConfig_Include),
			Value:          "* & $src as $__next__",
			SyntaxFlowRule: true,
		})
	}
}
