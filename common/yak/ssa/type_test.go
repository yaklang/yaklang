package ssa_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/ssa_option"
)

func Test_Type_ContainSelf(t *testing.T) {
	t.Run("function return self", func(t *testing.T) {
		fType := ssa.NewFunctionType("", nil, nil, false)
		fType.ReturnType = fType
		log.Infof("fType: %s", fType.String())
	})

	t.Run("object type contain self", func(t *testing.T) {
		objType := ssa.NewMapType(ssa.CreateStringType(), ssa.CreateAnyType())
		objType.FieldType = objType
		objType.Finish()
		log.Infof("objType: %s", objType.String())
	})
}

func Test_Type_CheckOrType(t *testing.T) {
	// {
	// 	str1 := ssa.CreateStringType()
	// 	str2 := ssa.CreateStringType()
	// 	targetType := ssa.NewOrType(str1, str2)
	// 	require.Equal(t, ssa.StringTypeKind, targetType.GetTypeKind())
	// }

	{
		str1 := ssa.CreateStringType()
		num1 := ssa.CreateNumberType()
		targetType := ssa.NewOrType(str1, num1)
		require.Equal(t, ssa.OrTypeKind, targetType.GetTypeKind())
		ssa.ExternMethodBuilder = &ssa_option.Builder{}

		method := ssa.GetMethod(str1, "Contains")
		require.NotNil(t, method)

		method2 := ssa.GetMethod(num1, "Contains")
		require.Nil(t, method2)

		method3 := ssa.GetMethod(targetType, "Contains")
		require.NotNil(t, method3)
	}
}

func TestFunctionTypeString_CachesRawStringResult(t *testing.T) {
	// Build a FunctionType with a large SideEffects list, which makes
	// RawString() expensive (string concat per side effect).
	ft := ssa.NewFunctionType("", nil, nil, false)
	for i := 0; i < 1000; i++ {
		ft.SideEffects = append(ft.SideEffects, &ssa.FunctionSideEffect{
			Name:        "se",
			VerboseName: "se",
		})
	}

	// First call computes RawString and caches it in Name.
	first := ft.String()
	require.NotEmpty(t, first)

	// Second call must return the cached result, not recompute RawString.
	// With the atomic stringCache, String() returns the cached signature
	// immediately.
	second := ft.String()
	require.Equal(t, first, second)

	// Also verify RawString itself is bounded (no infinite recursion).
	raw := ft.RawString()
	require.NotEmpty(t, raw)
}

// Test_FunctionType_StringCacheInvalidatedByMutation verifies String() output
// changes when the string-affecting fields mutate through the setters, and
// that self-referential types still terminate (review A5).
func Test_FunctionType_StringCacheInvalidatedByMutation(t *testing.T) {
	ft := ssa.NewFunctionType("", []ssa.Type{ssa.CreateStringType()}, ssa.CreateNumberType(), false)
	first := ft.String()
	require.Contains(t, first, "string")
	require.Contains(t, first, "-> number")

	// Same object mutates: cached String() must not stay stale.
	ft.SetReturnType(ssa.CreateBooleanType())
	second := ft.String()
	require.Contains(t, second, "-> boolean", "return type mutation must invalidate stringCache")

	ft.SetParameter([]ssa.Type{ssa.CreateNumberType()})
	third := ft.String()
	require.Contains(t, third, "(number)")
	require.NotEqual(t, second, third, "parameter mutation must invalidate stringCache")

	// Self-referential type must not recurse forever.
	ft.SetParameter([]ssa.Type{ft})
	require.NotPanics(t, func() { _ = ft.String() })
}

// Test_FunctionType_StringConcurrentSafe verifies concurrent String() calls
// on one FunctionType neither race on the cache/guard state nor hang: each
// call returns either the full signature or the "..." placeholder while
// another goroutine is computing, and the cache settles to the full
// signature afterwards (review A5).
func Test_FunctionType_StringConcurrentSafe(t *testing.T) {
	ft := ssa.NewFunctionType("", []ssa.Type{ssa.CreateStringType()}, ssa.CreateNumberType(), false)
	for i := 0; i < 200; i++ {
		ft.SideEffects = append(ft.SideEffects, &ssa.FunctionSideEffect{Name: "se", VerboseName: "se"})
	}

	const workers = 64
	results := make([]string, workers)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = ft.String()
		}(i)
	}
	wg.Wait()

	for _, r := range results {
		require.NotEmpty(t, r, "String() must never return an empty result")
		require.True(t, r == "..." || strings.Contains(r, "-> number"),
			"result must be the placeholder or a full signature, got %q", r)
	}
	require.Contains(t, ft.String(), "-> number", "cache must settle to the full signature")
}
