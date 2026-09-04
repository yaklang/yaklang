package csharp2ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestCSharpValuePathSignatureIsStructural(t *testing.T) {
	dottedSegment := csharpValuePathSignature([]ssa.Value{
		ssa.NewConst("Root"),
		ssa.NewConst("a.b"),
		ssa.NewConst("Value"),
	})
	nestedSegments := csharpValuePathSignature([]ssa.Value{
		ssa.NewConst("Root"),
		ssa.NewConst("a"),
		ssa.NewConst("b"),
		ssa.NewConst("Value"),
	})
	require.NotEqual(t, dottedSegment, nestedSegments)
}
