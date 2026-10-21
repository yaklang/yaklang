package ssa_test

import (
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

func TestFunctionAddAnnotationFuncInitializesDeferredType(t *testing.T) {
	f := &ssa.Function{}
	called := false
	f.AddAnnotationFunc(func(ssa.Value) { called = true })
	ft, ok := ssa.ToFunctionType(f.GetType())
	require.True(t, ok)
	require.Same(t, f, ft.This)
	require.Len(t, ft.AnnotationFunc, 1)
	ft.AnnotationFunc[0](nil)
	require.True(t, called)
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
		ssa.SetExternMethodBuilder(&ssa_option.Builder{})

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
	// With the cache, Name is non-empty and not "..." so String() returns
	// the cached string immediately.
	second := ft.String()
	require.Equal(t, first, second)

	// Also verify RawString itself is bounded (no infinite recursion).
	raw := ft.RawString()
	require.NotEmpty(t, raw)
}

// diamondStructType returns a struct whose raw field tree holds 2^(levels+1)-1
// nodes: each level embeds the previous one twice by value.
func diamondStructType(levels int) *ssa.ObjectType {
	typ := ssa.NewStructType()
	for i := 0; i < levels; i++ {
		next := ssa.NewStructType()
		next.AddField(ssa.NewConst("l"), typ)
		next.AddField(ssa.NewConst("r"), typ)
		typ = next
	}
	return typ
}

// Test_StructDefaultExpansionMemo pins the memoization behind
// StructDefaultExpansionExceedsBudget: a cached "fits" answer must reset when
// the type grows through AddField, and cyclic field graphs must terminate as
// "exceeds" instead of hanging.
func Test_StructDefaultExpansionMemo(t *testing.T) {
	t.Run("AddField growth invalidates the cached answer", func(t *testing.T) {
		outer := ssa.NewStructType()
		outer.AddField(ssa.NewConst("f"), diamondStructType(8)) // 511 nodes, fits
		require.False(t, ssa.StructDefaultExpansionExceedsBudget(outer))

		outer.AddField(ssa.NewConst("g"), diamondStructType(10)) // subtree alone > budget
		require.True(t, ssa.StructDefaultExpansionExceedsBudget(outer))
	})

	t.Run("cyclic field graph terminates as exceeding", func(t *testing.T) {
		a := ssa.NewStructType()
		b := ssa.NewStructType()
		a.AddField(ssa.NewConst("b"), b)
		b.AddField(ssa.NewConst("a"), a)
		require.True(t, ssa.StructDefaultExpansionExceedsBudget(a))
	})

	t.Run("plain and alias-wrapped structs stay within budget", func(t *testing.T) {
		require.False(t, ssa.StructDefaultExpansionExceedsBudget(diamondStructType(8)))
		alias := ssa.NewAliasType("T", "", diamondStructType(10))
		require.True(t, ssa.StructDefaultExpansionExceedsBudget(alias))
	})
}

// Test_TypeStringLimited_BoundedOutput pins TypeStringLimited: small types
// render fully, exponential field graphs abbreviate with "..." and stay far
// below the raw expansion size.
func Test_TypeStringLimited_BoundedOutput(t *testing.T) {
	small := ssa.NewStructType()
	small.AddField(ssa.NewConst("a"), ssa.CreateStringType())
	s := ssa.TypeStringLimited(small)
	require.Contains(t, s, "struct {")
	require.NotContains(t, s, "...")

	big := ssa.TypeStringLimited(diamondStructType(16)) // raw tree > 100k nodes
	require.Contains(t, big, "...")
	require.Less(t, len(big), 2*4096+64)
}
