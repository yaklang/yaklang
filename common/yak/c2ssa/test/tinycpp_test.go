package test

import (
	"io/fs"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func loadTinycppTestFS(t *testing.T) *filesys.VirtualFS {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Join(filepath.Dir(file), "..", "preprocess", "testdata", "tinycpp")
	local := filesys.NewRelLocalFs(root)
	vf := filesys.NewVirtualFs()
	err := filesys.Recursive(".", filesys.WithFileSystem(local), filesys.WithStat(func(isDir bool, pathname string, info fs.FileInfo) error {
		if isDir {
			return nil
		}
		// Match real tinycpp project layout (3 translation units).
		if pathname == "preproc_snippet.c" {
			return nil
		}
		data, err := local.ReadFile(pathname)
		if err != nil {
			return err
		}
		vf.AddFile(pathname, string(data))
		return nil
	}))
	require.NoError(t, err)
	return vf
}

func assertNoLibulzMacroErrors(t *testing.T, msg string) {
	t.Helper()
	require.NotContains(t, msg, "tglist(char")
	require.NotContains(t, msg, "hbmap(char")
	require.NotContains(t, msg, "hbmap_foreach")
	require.NotContains(t, msg, "tglist_foreach")
	require.NotContains(t, msg, "tglist _impl")
}

func TestTinycpp_FullSSA(t *testing.T) {
	vf := loadTinycppTestFS(t)

	progs, err := ssaapi.ParseProjectWithFS(vf,
		ssaapi.WithLanguage(ssaconfig.C),
		ssaapi.WithStrictMode(true),
	)
	require.NoError(t, err, "SSA project compile should succeed without AST errors")
	require.NotEmpty(t, progs)

	for _, prog := range progs {
		for _, e := range prog.GetErrors() {
			if e.Kind != ssa.Error {
				continue
			}
			assertNoLibulzMacroErrors(t, e.Message)
			require.NotContains(t, e.Message, "syntax errors found", "unexpected syntax error: %s", e.Message)
		}

		fn := prog.Ref("free_macros")
		require.NotEmpty(t, fn, "free_macros should be present in SSA program")
	}
}
