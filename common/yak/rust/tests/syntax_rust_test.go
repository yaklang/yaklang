package tests

import (
	"embed"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/rust/rust2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

//go:embed all:code
var codeFS embed.FS

func TestRustFrontendBasic(t *testing.T) {
	raw, err := codeFS.ReadFile("code/basic.rs")
	require.NoError(t, err)

	ast, err := rust2ssa.Frontend(string(raw))
	require.NoError(t, err)
	require.NotNil(t, ast)
}

func TestRustFrontendEditionSamples(t *testing.T) {
	for _, name := range []string{"code/edition2015.rs", "code/edition2021.rs"} {
		t.Run(name, func(t *testing.T) {
			raw, err := codeFS.ReadFile(name)
			require.NoError(t, err)
			ast, err := rust2ssa.Frontend(string(raw))
			require.NoError(t, err)
			require.NotNil(t, ast)
		})
	}
}

func TestRustProjectCompileBasic(t *testing.T) {
	raw, err := codeFS.ReadFile("code/basic.rs")
	require.NoError(t, err)

	vf := filesys.NewVirtualFs()
	vf.AddFile("main.rs", string(raw))

	progs, err := ssaapi.ParseProjectWithFS(
		vf,
		ssaapi.WithLanguage(ssaconfig.RUST),
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
