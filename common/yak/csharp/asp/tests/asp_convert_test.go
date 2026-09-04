package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/csharp/asp"
	"github.com/yaklang/yaklang/common/yak/csharp/csharp2ssa"
)

func TestASPConvertToCSharpNamedIsland_Frontend(t *testing.T) {
	raw, err := aspFs.ReadFile("code/mixed/island.aspx")
	require.NoError(t, err)
	cs, err := asp.ConvertToCSharp(string(raw), "island.aspx")
	require.NoError(t, err)
	require.Contains(t, cs, "IslandValue")
	require.Contains(t, cs, "keptScriptlet")
	require.Contains(t, cs, "class Generated_island")
	ast, err := csharp2ssa.Frontend(cs)
	require.NoError(t, err)
	require.NotNil(t, ast)
	require.Contains(t, ast.GetText(), "IslandValue")
}

func TestASPConvertHelloParsesAsCSharp(t *testing.T) {
	raw, err := aspFs.ReadFile("code/hello.aspx")
	require.NoError(t, err)
	cs, err := asp.ConvertToCSharp(string(raw), "hello.aspx")
	require.NoError(t, err)
	ast, err := csharp2ssa.Frontend(cs)
	require.NoError(t, err)
	require.NotNil(t, ast)
	require.Contains(t, ast.GetText(), "Page_Load")
}
