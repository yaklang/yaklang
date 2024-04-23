package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
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
		for index, prog := range progs {
			log.Infof("prog[%d]:", index)
			prog.Show()
		}

		require.NoError(t, err, "parse project failed")

		require.Len(t, progs, 2, "progs should be 2")
		prog := progs[0]

		valuesB := prog.Ref("b")
		require.Len(t, valuesB, 1, "valuesB should be 1")

		valueB := valuesB[0]
		require.Equal(t, "3", valueB.String(), "valueB should be 3")
	})

}
