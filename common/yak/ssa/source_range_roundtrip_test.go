package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestSyntheticInstructionRangeRoundTrip(t *testing.T) {
	// Constants constructed outside an AST visitor have no source range.
	constant := NewConst("synthetic member key", true)
	ir := &ssadb.IrCode{OpcodeName: "ConstInst"}
	fitRange(ir, constant.GetRange(), constant)
	require.Empty(t, ir.SourceCodeHash)
	editor, codeRange, err := getIRCodeRange(nil, ir)
	require.NoError(t, err)
	require.Nil(t, editor)
	require.Nil(t, codeRange)
}

func TestIncompleteInstructionSourceRangeIsAnError(t *testing.T) {
	for _, ir := range []*ssadb.IrCode{
		{SourceCodeStartOffset: 1},
		{SourceCodeEndOffset: 12},
		{SourceCodeHash: "nonexistent-source-range-regression"},
	} {
		_, _, err := getIRCodeRange(nil, ir)
		require.Error(t, err, "partial or broken source metadata must stay visible")
	}
}
