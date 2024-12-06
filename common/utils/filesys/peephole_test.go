package filesys_test

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

func TestPeephole(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("a1", "a")
	vfs.AddFile("a2", "a")
	vfs.AddFile("a3", "a")
	vfs.AddFile("a4", "a")
	vfs.AddFile("b/b1", "a")
	vfs.AddFile("b/b2", "a")
	vfs.AddFile("b/b3", "a")
	vfs.AddFile("b/b4", "a")
	vfs.AddFile("c/d/e1", "a")
	vfs.AddFile("c/d/e2", "a")
	vfs.AddFile("c/d/e3", "a")
	vfs.AddFile("c/d/e4", "a")
	vfs.AddFile("c/d/e6", "a")

	checkFilesystem := func(t *testing.T, system filesys_interface.FileSystem) {
		filesys.TreeView(system)
		fileCount := 0
		filesys.Recursive(".",
			filesys.WithFileSystem(system),
			filesys.WithFileStat(func(s string, fi fs.FileInfo) error {
				log.Infof("file: %s", s)
				data, err := system.ReadFile(s)
				require.NoError(t, err)
				log.Infof("data: %s\n", string(data))
				require.Greater(t, len(data), 0)
				fileCount++
				return nil
			}),
		)
		require.Greater(t, fileCount, 0)
	}

	t.Run("test size", func(t *testing.T) {
		count := 0
		filesys.Peephole(vfs,
			filesys.WithPeepholeSize(3),
			filesys.WithPeepholeCallback(func(system fi.FileSystem) {
				count++
				checkFilesystem(t, system)
			}),
		)
		require.Equal(t, count, 6)
	})

}
