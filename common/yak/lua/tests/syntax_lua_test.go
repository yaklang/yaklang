package tests

import (
	"embed"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/lua/lua2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

//go:embed all:code
var codeFS embed.FS

func TestLuaFrontendBasic(t *testing.T) {
	raw, err := codeFS.ReadFile("code/basic.lua")
	require.NoError(t, err)

	ast, err := lua2ssa.Frontend(string(raw))
	require.NoError(t, err)
	require.NotNil(t, ast)
}

func TestLuaFrontendVersionSamples(t *testing.T) {
	for _, name := range []string{"code/lua51.lua", "code/lua52.lua", "code/lua54.lua"} {
		t.Run(name, func(t *testing.T) {
			raw, err := codeFS.ReadFile(name)
			require.NoError(t, err)
			ast, err := lua2ssa.Frontend(string(raw))
			require.NoError(t, err)
			require.NotNil(t, ast)
		})
	}
}

func TestLuaProjectCompileBasic(t *testing.T) {
	raw, err := codeFS.ReadFile("code/basic.lua")
	require.NoError(t, err)

	vf := filesys.NewVirtualFs()
	vf.AddFile("main.lua", string(raw))

	progs, err := ssaapi.ParseProjectWithFS(
		vf,
		ssaapi.WithLanguage(ssaconfig.LUA),
		ssaapi.WithMemory(),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)

	prog := progs[0]
	prog.Show()
	require.NotEmpty(t, prog.Ref("add"))
	require.NotEmpty(t, prog.Ref("main"))
	require.NotEmpty(t, prog.Ref("x"))
}
