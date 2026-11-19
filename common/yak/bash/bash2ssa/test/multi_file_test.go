package test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// multiFileProject builds the source tree used by the multi-file tests:
// main.sh sources lib/util.sh, which sources lib/core.sh in turn.
func multiFileProject(t *testing.T) *filesys.VirtualFS {
	t.Helper()
	fs := filesys.NewVirtualFs()
	for _, name := range []string{"main.sh", "lib/util.sh", "lib/core.sh"} {
		raw, err := codeFs.ReadFile("code/multi_file/" + name)
		require.NoError(t, err)
		fs.AddFile(name, string(raw))
	}
	return fs
}

func TestBashMultiFileSource(t *testing.T) {
	ps, err := ssaapi.ParseProjectWithFS(multiFileProject(t), ssaapi.WithLanguage(ssaconfig.BASH), ssaapi.WithMemory())
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
	require.Contains(t, got, `0`)
	require.Condition(t, func() bool {
		for _, v := range got {
			if strings.Contains(v, `add("wrapped:",`) {
				return true
			}
		}
		return false
	}, "println must receive the wrapped command substitution result: %v", got)
}

func TestBashMultiFileDataflow(t *testing.T) {
	ssatest.CheckSyntaxFlowWithFS(t, multiFileProject(t), `println(* #-> as $v)`, map[string][]string{
		"v": {`"value"`, `"wrapped:"`},
	}, true, ssaapi.WithLanguage(ssaconfig.BASH), ssaapi.WithMemory())
}
