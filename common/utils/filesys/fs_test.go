package filesys

import (
	"embed"
	"io/fs"
	"testing"

	"github.com/yaklang/yaklang/common/log"
)

//go:embed testdata/***
var testfs embed.FS

func TestFS_EmbedFS_SingleLevel(t *testing.T) {
	count := 0

	err := Recursive(
		"testdata",
		WithEmbedFS(testfs),
		WithRecursiveDirectory(false),
		WithFileStat(func(s string, f fs.File, fi fs.FileInfo) error {
			log.Infof("read file: %s", fi.Name())
			count++
			return nil
		}),
	)

	if err != nil {
		t.Fatal(err)
	}

	if count != 1 {
		t.Fatal("count != 1")
	}
}

func TestFS_EmbedFS_NotSingleLevel(t *testing.T) {
	count := 0

	err := Recursive(
		"testdata",
		WithEmbedFS(testfs),
		WithFileStat(func(s string, f fs.File, fi fs.FileInfo) error {
			log.Infof("read file: %s", fi.Name())
			count++
			return nil
		}),
	)

	if err != nil {
		t.Fatal(err)
	}

	if count != 7 {
		t.Fatal("count != 7")
	}
}

func TestFS_Chains(t *testing.T) {
	count := 0
	err := Recursive(
		"testdata",
		WithEmbedFS(testfs),
		WithDir(
			"ta",
			WithFileStat(func(s string, f fs.File, fi fs.FileInfo) error {
				count++
				log.Infof("match: %v", s)
				return nil
			}),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("count[%d] != 4", count)
	}
}
