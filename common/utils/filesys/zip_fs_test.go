package filesys

import (
	_ "embed"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"os"
	"testing"
)

//go:embed testdata.zip
var jarFS string

func TestCFRZip(t *testing.T) {
	z, err := NewZipFSFromString(jarFS)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	Recursive(".", WithFileSystem(z), WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
		fmt.Println(pathname)
		count++
		return nil
	}))
	if count == 0 {
		t.Fatal("no file found")
	}
	entry, _ := z.ReadDir("zip_fs.go")
	assert.Equal(t, len(entry), 0)
	raw, _ := z.ReadFile("zip_fs.go")
	assert.Greater(t, len(raw), 100)
	entry, _ = z.ReadDir(".")
	assert.Greater(t, len(entry), 10)
	entry, _ = z.ReadDir("/")
	assert.Greater(t, len(entry), 10)
}

//go:embed fs.zip
var badFs string

func TestCode(t *testing.T) {
	zipfs, err := NewZipFSFromString(badFs)
	require.NoError(t, err)
	var count int
	err = Recursive(".", WithFileSystem(zipfs), WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
		log.Infof("pathname: %s", pathname)
		count++
		return nil
	}))
	fmt.Println(count)
	require.Greater(t, count, 0)
}
