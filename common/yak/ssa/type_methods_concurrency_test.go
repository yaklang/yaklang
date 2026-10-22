package ssa

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
)

func TestTypeMethodsPublishStableSnapshots(t *testing.T) {
	typ := NewBaseType()
	const count = 32
	fn := NewFunctionWithType("method", nil)
	old := typ.GetMethod()
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			typ.AddMethod(fmt.Sprintf("m%d", i), fn)
			for range typ.GetMethod() {
			}
		}(i)
	}
	wg.Wait()
	require.Empty(t, old)
	require.Len(t, typ.GetMethod(), count)
	typ.RangeMethod(func(name string, _ *Function) { typ.AddMethod(name, fn) })
	require.Len(t, typ.GetMethod(), count)
	source := map[string]*Function{"only": fn}
	typ.SetMethod(source)
	delete(source, "only")
	require.Len(t, typ.GetMethod(), 1)
}

func TestTypeMethodBatchesPreserveConcurrentUpdates(t *testing.T) {
	typ := NewBaseType()
	fn := NewFunctionWithType("method", nil)
	typ.AddMethod("original", fn)
	old := typ.GetMethod()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			typ.AddMethods(map[string]*Function{fmt.Sprintf("m%d", i): fn})
			for range typ.GetMethod() {
			}
		}(i)
	}
	wg.Wait()
	require.Len(t, old, 1)
	require.Len(t, typ.GetMethod(), 33)
	replacement := NewFunctionWithType("replacement", nil)
	input := map[string]*Function{"original": replacement}
	typ.AddMethods(input)
	delete(input, "original")
	require.Same(t, fn, old["original"])
	require.Same(t, replacement, typ.GetMethod()["original"])
	allocs := testing.AllocsPerRun(100, func() { typ.AddMethods(typ.GetMethod()) })
	require.Zero(t, allocs, "unchanged method batches should allocate nothing")
}
