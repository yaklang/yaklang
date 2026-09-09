package sfvm

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestRuleModeFromCompiledDesc(t *testing.T) {
	require.True(t, (&schema.SyntaxFlowRule{Mode: schema.SFR_MODE_STRUCT}).IsStructMode())
	require.False(t, (&schema.SyntaxFlowRule{Mode: schema.SFR_MODE_SSA}).IsStructMode())
	require.False(t, (&schema.SyntaxFlowRule{Mode: schema.SFR_MODE_SOURCE}).IsStructMode())
	require.False(t, (*schema.SyntaxFlowRule)(nil).IsStructMode())
	require.False(t, (&schema.SyntaxFlowRule{Tag: "security|struct"}).IsStructMode(),
		"empty Mode defaults to ssa; tag-only does not select struct until NormalizeMode")

	require.True(t, (&schema.SyntaxFlowRule{Mode: schema.SFR_MODE_SOURCE}).IsSourceMode())
	require.False(t, (&schema.SyntaxFlowRule{Mode: schema.SFR_MODE_SSA}).IsSourceMode())
	require.False(t, (*schema.SyntaxFlowRule)(nil).IsSourceMode())
}

func TestFrameIsStructModeFromDesc(t *testing.T) {
	frame, err := NewSyntaxFlowVirtualMachine().Compile(`
desc(mode: "struct", language: "java")
Runtime.getRuntime().exec(* as $cmd) as $call
alert $call
`)
	require.NoError(t, err)
	require.True(t, FrameIsStructMode(frame))
	require.False(t, FrameIsSourceMode(frame))
	require.True(t, frame.GetRule().IsStructMode())
	require.Equal(t, schema.SFR_MODE_STRUCT, frame.GetRule().Mode)

	ssaFrame, err := NewSyntaxFlowVirtualMachine().Compile(`
desc(mode: "ssa")
a as $hit
alert $hit
`)
	require.NoError(t, err)
	require.False(t, FrameIsStructMode(ssaFrame))
	require.Equal(t, schema.SFR_MODE_SSA, ssaFrame.GetRule().Mode)
}
