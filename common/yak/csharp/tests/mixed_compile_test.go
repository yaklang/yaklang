package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/csharp/asp"
	"github.com/yaklang/yaklang/common/yak/csharp/csharp2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestCSharpASPXMixedCompile_ParseProjectWithFS(t *testing.T) {
	aspxPath := filepath.Join("..", "asp", "tests", "code", "mixed", "island.aspx")
	aspxRaw, err := os.ReadFile(aspxPath)
	require.NoError(t, err)
	csRaw, err := os.ReadFile(filepath.Join("mixed", "CodeBehind.cs"))
	require.NoError(t, err)

	generated, err := asp.ConvertToCSharp(string(aspxRaw), "island.aspx")
	require.NoError(t, err)
	require.Contains(t, generated, "IslandValue", "ASPX island must lower to named C#")
	require.Contains(t, generated, "keptScriptlet")

	ast, err := csharp2ssa.Frontend(generated)
	require.NoError(t, err)
	require.NotNil(t, ast)
	require.Contains(t, ast.GetText(), "IslandValue")
	require.Contains(t, ast.GetText(), "Generated_island")

	vf := filesys.NewVirtualFs()
	vf.AddFile("Web/island.aspx", string(aspxRaw))
	vf.AddFile("Web/CodeBehind.cs", string(csRaw))
	progs, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.CSHARP), ssaapi.WithMemory())
	require.NoError(t, err)
	require.NotEmpty(t, progs)
	prog := progs[0]
	require.NotNil(t, prog)
	require.True(t,
		len(prog.Ref("IslandValue")) > 0 || len(prog.Ref("Generated_island")) > 0 || len(prog.Ref("Render")) > 0,
		"expected IslandValue/Generated_island from ASPX island in mixed project SSA")
	require.True(t,
		len(prog.Ref("CodeBehind")) > 0 || len(prog.Ref("Partner")) > 0,
		"expected code-behind type in mixed project")
}
