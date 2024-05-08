package filesys

import (
	"embed"
	"io/fs"
	"os"
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

func TestFS_LocalFS(t *testing.T) {
	check := func(want int, opts ...Option) {
		count := 0
		opts = append(opts, WithFileStat(func(s string, f fs.File, fi fs.FileInfo) error {
			count++
			log.Infof("match: %v", s)
			return nil
		}))
		err := Recursive(
			"testdata",
			opts...,
		)
		if err != nil {
			t.Fatal(err)
		}
		if count != want {
			t.Fatalf("count[%d] != %d", count, want)
		}
	}

	t.Run("empty local fs", func(t *testing.T) {
		check(7,
			WithFileSystem(NewLocalFs()),
		)
	})

	t.Run("local fs with current dir .", func(t *testing.T) {
		check(7,
			WithFileSystem(NewLocalFsWithPath(".")),
		)
	})

	t.Run("local fs with absolute path", func(t *testing.T) {
		currentDir, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		check(7,
			WithFileSystem(NewLocalFsWithPath(currentDir)),
		)
	})

	t.Run("use state skip all directory ", func(t *testing.T) {
		check(1,
			WithFileSystem(NewLocalFs()),
			WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
				if isDir {
					return SkipDir
				}
				return nil
			}),
		)
	})
}
