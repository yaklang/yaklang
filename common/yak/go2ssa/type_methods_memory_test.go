package go2ssa

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestCoverTypeMethodMergeAllocation(t *testing.T) {
	source := ssa.NewObjectType()
	source.SetTypeKind(ssa.StructTypeKind)
	methods := make(map[string]*ssa.Function)
	for i := 0; i < 64; i++ {
		methods[fmt.Sprintf("method%d", i)] = ssa.NewFunctionWithType("method", nil)
	}
	source.SetMethod(methods)
	target := ssa.NewObjectType()
	target.SetTypeKind(ssa.StructTypeKind)
	keep := ssa.NewFunctionWithType("keep", nil)
	target.AddMethod("keep", keep)
	old := target.GetMethod()
	coverType(target, source)
	require.Len(t, target.GetMethod(), 65)
	require.Equal(t, map[string]*ssa.Function{"keep": keep}, old)
	for name, fn := range methods {
		require.Same(t, fn, target.GetMethod()[name])
	}
	allocs := testing.AllocsPerRun(20, func() {
		dst := ssa.NewObjectType()
		coverType(dst, source)
	})
	t.Logf("cover 64 methods: %.0f allocations", allocs)
	require.Less(t, allocs, float64(30), "merging a method set must copy once, not once per method")
}
