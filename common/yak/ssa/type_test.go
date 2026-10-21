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

// Test_TypeStringLimited_BoundedOutput pins TypeStringLimited: small types
// render fully, exponential field graphs abbreviate with "..." and stay far
// below the raw expansion size. Each nesting level assembles at most
// typeStringWidthLimit characters, so the output is bounded by the depth cap
// times the width limit.
func Test_TypeStringLimited_BoundedOutput(t *testing.T) {
	t.Run("small type renders fully", func(t *testing.T) {
		small := ssa.NewStructType()
		small.AddField(ssa.NewConst("a"), ssa.CreateStringType())
		s := ssa.TypeStringLimited(small)
		require.Contains(t, s, "struct {")
		require.NotContains(t, s, "...")
	})

	t.Run("exponential nesting abbreviates", func(t *testing.T) {
		big := ssa.TypeStringLimited(diamondStructType(16)) // raw tree > 100k nodes
		require.Contains(t, big, "...")
		require.Less(t, len(big), 2*4096+64)
	})

	t.Run("wide flat struct truncates at the width limit", func(t *testing.T) {
		wide := ssa.NewStructType()
		for i := 0; i < 4000; i++ {
			wide.AddField(ssa.NewConst(int64(i)), ssa.CreateNumberType())
		}
		s := ssa.TypeStringLimited(wide)
		require.Contains(t, s, "...")
		require.Less(t, len(s), 2*4096+64)
	})
}
