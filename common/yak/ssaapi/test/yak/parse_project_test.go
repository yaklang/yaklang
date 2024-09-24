package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestParseProject(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("a/b", "c")
	vfs.AddFile("a/a.yak", `include "b/b.yak"; dump(b)`)
	vfs.AddFile("b/b.yak", `dump("in b.yak"); b = 3`)
	vfs.AddFile("c/c.yak", `include "b/b.yak"; dump(b + 1)`)

	t.Run("parse project with entry", func(t *testing.T) {
		progs, err := ssaapi.ParseProject(
			vfs,
			ssaapi.WithFileSystemEntry("a/a.yak"),
			// ssaapi.WithDatabaseProgramName("test"),
		)

		require.NoError(t, err, "parse project failed")

		prog := progs

		valuesB := prog.Ref("b")
		valuesB.Show()
		require.Len(t, valuesB, 2, "valuesB should be 1")

		valueB := valuesB[0]
		require.Equal(t, "3", valueB.String(), "valueB should be 3")
	})

}
