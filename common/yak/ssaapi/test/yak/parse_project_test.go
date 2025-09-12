package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestParseInclude(t *testing.T) {
	fs := filesys.NewVirtualFs()
	fs.AddFile("a/a.yak", `include "b/b.yak"; dump(b)`)
	fs.AddFile("b/b.yak", `b = 3`)
	ssatest.CheckSyntaxFlowWithFS(t, fs, `dump(* as $sink)`, map[string][]string{
		"sink": {"3"},
	}, false, ssaapi.WithLanguage(ssaapi.Yak))
}

func TestParseInclude2(t *testing.T) {
	fs := filesys.NewVirtualFs()
	fs.AddFile("a/a.yak", `include "b/b.yak"; dump(b)`)
	fs.AddFile("b/b.yak", `b = 3`)
	fs.AddFile("a/c.yak", `include "b/b.yak"; dump(b)`)
	programs, err := ssaapi.ParseProjectWithFS(fs, ssaapi.WithLanguage(ssaapi.Yak))
	require.NoError(t, err)
	valB := programs[0].Ref("b")
	valB.Show()
}

func TestParseProject(t *testing.T) {
	// for i := 0; i < 100; i++ {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("a/b", "c")
	vfs.AddFile("a/a.yak", `include "b/b.yak"; dump(b)`)
	vfs.AddFile("b/b.yak", `dump("in b.yak"); b = 3`)
	vfs.AddFile("c/c.yak", `include "b/b.yak"; dump(b + 1)`)

	t.Run("parse project with entry", func(t *testing.T) {
		progs, err := ssaapi.ParseProjectWithFS(
			vfs,
			ssaapi.WithFileSystemEntry("a/a.yak"),
			ssaapi.WithLanguage(ssaapi.Yak),
			// ssaapi.WithDatabaseProgramName("test"),
		)
		progs.Show()
		require.NoError(t, err, "parse project failed")

		require.Len(t, progs, 1, "progs should be 1")
		prog := progs[0]

		valuesB := prog.Ref("b")
		valuesB.Show()
		require.Len(t, valuesB, 1, "valuesB should be 1")

		valueB := valuesB[0]
		require.Contains(t, valueB.String(), "3", "valueB should be 3")
	})
	// }
}
