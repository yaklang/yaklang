package tests

import (
	"embed"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/scala/scala2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

//go:embed all:code
var codeFS embed.FS

func TestScalaFrontendBasic(t *testing.T) {
	raw, err := codeFS.ReadFile("code/basic.scala")
	require.NoError(t, err)

	ast, err := scala2ssa.Frontend(string(raw))
	require.NoError(t, err)
	require.NotNil(t, ast)
}

func TestScalaFrontendVersionSamples(t *testing.T) {
	for _, name := range []string{"code/legacy2.scala", "code/modern3.scala"} {
		t.Run(name, func(t *testing.T) {
			raw, err := codeFS.ReadFile(name)
			require.NoError(t, err)
			ast, err := scala2ssa.Frontend(string(raw))
			require.NoError(t, err)
			require.NotNil(t, ast)
		})
	}
}

func TestScalaProjectCompileBasic(t *testing.T) {
	raw, err := codeFS.ReadFile("code/basic.scala")
	require.NoError(t, err)

	vf := filesys.NewVirtualFs()
	vf.AddFile("main.scala", string(raw))

	progs, err := ssaapi.ParseProjectWithFS(
		vf,
		ssaapi.WithLanguage(ssaconfig.SCALA),
		ssaapi.WithMemory(),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)

	prog := progs[0]
	prog.Show()
	require.NotEmpty(t, prog.Ref("add"))
	require.NotEmpty(t, prog.Ref("main"))
}
