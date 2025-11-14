package java

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCompile_Jar(t *testing.T) {
	t.Run("test should compile jar file", func(t *testing.T) {
		jarPath, err := ssatest.GetJarFile()
		require.NoError(t, err)

		info := map[string]any{
			"kind":       "jar",
			"local_file": jarPath,
		}

		prog, err := ssaapi.ParseProject(ssaapi.WithLanguage(ssaconfig.JAVA), ssaapi.WithConfigInfo(info))
		require.NoError(t, err)
		require.NotNil(t, prog)
		prog.Show()

		vals, err := prog.SyntaxFlowWithError(`System.out.println(,* as $a)`)
		require.NoError(t, err)
		res := vals.GetValues("a")
		require.Contains(t, res.String(), "Hello world")
	})

	t.Run("test should not compile jar file", func(t *testing.T) {
		jarRaw, err := ssatest.GetJarContent()
		require.NoError(t, err)

		vf := filesys.NewVirtualFs()
		vf.AddFile("B.class", string(jarRaw)) // B.class will skip
		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
			programs.Show()
			vals, err := programs.SyntaxFlowWithError(`System.out.println(* as $a)`)
			require.NoError(t, err)
			res := vals.GetValues("a")
			require.Equal(t, 0, res.Len())
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}
