package ssa

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRefParameterConcurrentLookupAndRecord(t *testing.T) {
	builder := NewBuilder(nil, NewFunctionWithType("method", nil), nil)
	const name = "member"
	builder.ReferenceParameter(name, 1, PointerSideEffect)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			if ref, ok := builder.lookupRefParameter(name); ok {
				require.Contains(t, []int{1, 2}, ref.Index)
				require.Equal(t, PointerSideEffect, ref.Kind)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			builder.ReferenceParameter(name, 2, PointerSideEffect)
		}
	}()
	wg.Wait()

	ref, ok := builder.lookupRefParameter(name)
	require.True(t, ok)
	require.Equal(t, 2, ref.Index)
	require.Equal(t, PointerSideEffect, ref.Kind)

	builder.ReferenceParameter("moved", 3, PointerSideEffect)
	moved, ok := builder.TakeRefParameter("moved")
	require.True(t, ok)
	require.Equal(t, 3, moved.Index)
	_, ok = builder.lookupRefParameter("moved")
	require.False(t, ok)
}
