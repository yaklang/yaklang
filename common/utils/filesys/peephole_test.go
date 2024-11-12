package filesys

import (
	"testing"

	"github.com/stretchr/testify/assert"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

func TestPeephole(t *testing.T) {
	vfs := NewVirtualFs()
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

	count := 0
	Peephole(vfs, WithPeepholeSize(3), WithPeepholeCallback(func(system fi.FileSystem) {
		count++
		TreeView(system)
	}))
	assert.Equal(t, count, 6)
}
