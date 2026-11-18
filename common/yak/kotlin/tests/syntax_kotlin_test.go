package tests

import (
	"embed"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/kotlin/kotlin2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

//go:embed all:code
var codeFS embed.FS

func TestKotlinFrontendBasic(t *testing.T) {
	raw, err := codeFS.ReadFile("code/basic.kt")
	require.NoError(t, err)

	ast, err := kotlin2ssa.Frontend(string(raw))
	require.NoError(t, err)
	require.NotNil(t, ast)
}

func TestKotlinFrontendVersionSamples(t *testing.T) {
	for _, name := range []string{"code/classic1.kt", "code/modern2.kt"} {
		t.Run(name, func(t *testing.T) {
			raw, err := codeFS.ReadFile(name)
			require.NoError(t, err)
			ast, err := kotlin2ssa.Frontend(string(raw))
			require.NoError(t, err)
			require.NotNil(t, ast)
		})
	}
}

func TestKotlinProjectCompileBasic(t *testing.T) {
	raw, err := codeFS.ReadFile("code/basic.kt")
	require.NoError(t, err)

	vf := filesys.NewVirtualFs()
	vf.AddFile("main.kt", string(raw))

	progs, err := ssaapi.ParseProjectWithFS(
		vf,
		ssaapi.WithLanguage(ssaconfig.KOTLIN),
		ssaapi.WithMemory(),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)

	prog := progs[0]
	prog.Show()
	require.NotEmpty(t, prog.Ref("add"))
	require.NotEmpty(t, prog.Ref("main"))
}
