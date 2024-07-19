package filesys

import (
	_ "embed"
	"fmt"
	"github.com/stretchr/testify/assert"
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
	Recursive(".", WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
		count++
		fmt.Println(pathname)
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
