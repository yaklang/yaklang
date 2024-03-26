package filesys

import (
	"embed"
	"github.com/yaklang/yaklang/common/log"
	"os"
	"testing"
)

//go:embed testdata/***
var testfs embed.FS

func TestFS_1(t *testing.T) {
	f := testfs
	_ = f
	count := 0
	err := Recursive(
		"testdata",
		WithEmbedFS(testfs),
		WithDirMatch("ta", WithFileStat(func(pathname string, info os.FileInfo) error {
			log.Infof("match: %v", pathname)
			count++
			return nil
		})))
	if err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatal("count != 4")
	}
}

func TestFS_Chains(t *testing.T) {
	count := 0
	err := Recursive(
		"testdata",
		WithDirMatches([]string{
			"cc", "dd",
		}, WithFileStat(func(pathname string, info os.FileInfo) error {
			count++
			log.Infof("match: %v", pathname)
			return nil
		})), WithEmbedFS(testfs),
	)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatal("count != 4")
	}
}
