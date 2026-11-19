package test

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"testing"
)

func sourceProject(t *testing.T) *filesys.VirtualFS {
	t.Helper()
	fs := filesys.NewVirtualFs()
	for _, name := range []string{"main.sh", "lib/colors.sh", "lib/palette.sh"} {
		raw, err := codeFs.ReadFile("code/source/" + name)
		require.NoError(t, err)
		fs.AddFile(name, string(raw))
	}
	return fs
}

func TestBashSourceInCaller(t *testing.T) {
	ps, err := ssaapi.ParseProjectWithFS(sourceProject(t), ssaapi.WithLanguage(ssaconfig.BASH), ssaapi.WithMemory())
	require.NoError(t, err)
	require.NotEmpty(t, ps)
	var got []string
	for _, p := range ps {
		for _, call := range p.Ref("println").GetUsers() {
			if v := call.GetOperand(1); v != nil {
				got = append(got, v.String())
			}
		}
	}
	require.ElementsMatch(t, []string{`"supplied"`, `"caller"`, `"again"`}, got)
}
