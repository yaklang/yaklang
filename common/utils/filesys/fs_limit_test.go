package filesys

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFSRecursiveLimit(t *testing.T) {
	vfs := NewVirtualFs()
	vfs.AddFile("1.txt", "123")
	vfs.AddFile("1/2.txt", "123")
	vfs.AddFile("1/2/3.txt", "123")
	vfs.AddFile("1/2/3/4.txt", "123")
	vfs.AddFile("1/2/3/4/5.txt", "123")
	vfs.AddFile("1/2/3/4/5/6.txt", "123")

	count := 0
	Recursive(".", WithFileSystem(vfs), WithFileLimit(1), WithFileStat(func(name string, info fs.FileInfo) error {
		count++
		t.Logf("name: %s, info: %v", name, info)
		return nil
	}))
	require.Equal(t, count, 1)
}

func TestFSRecursiveLimit2(t *testing.T) {
	vfs := NewVirtualFs()
	vfs.AddFile("1.txt", "123")
	vfs.AddFile("1/2.txt", "123")
	vfs.AddFile("1/2/3.txt", "123")
	vfs.AddFile("1/2/3/4.txt", "123")
	vfs.AddFile("1/2/3/4/5.txt", "123")
	vfs.AddFile("1/2/3/4/5/6.txt", "123")

	count := 0
	Recursive(".", WithFileSystem(vfs), WithFileLimit(100), WithFileStat(func(name string, info fs.FileInfo) error {
		count++
		t.Logf("name: %s, info: %v", name, info)
		return nil
	}))
	require.Equal(t, count, 6)
}

func TestFSRecursiveLimit3(t *testing.T) {
	vfs := NewVirtualFs()
	vfs.AddFile("1.txt", "123")
	vfs.AddFile("1/2.txt", "123")
	vfs.AddFile("1/2/3.txt", "123")
	vfs.AddFile("1/2/3/4.txt", "123")
	vfs.AddFile("1/2/3/4/5.txt", "123")
	vfs.AddFile("1/2/3/4/5/6.txt", "123")

	count := 0
	Recursive(".", WithFileSystem(vfs), WithFileLimit(6), WithFileStat(func(name string, info fs.FileInfo) error {
		count++
		t.Logf("name: %s, info: %v", name, info)
		return nil
	}))
	require.Equal(t, count, 6)
}
