package filesys

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
		WithFileStat(func(s string, fi fs.FileInfo) error {
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
		WithFileStat(func(s string, fi fs.FileInfo) error {
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
			WithFileStat(func(s string, fi fs.FileInfo) error {
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
		opts = append(opts, WithFileStat(func(s string, fi fs.FileInfo) error {
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
			WithFileSystem(NewLocalFs()),
		)
	})

	t.Run("local fs with absolute path", func(t *testing.T) {
		currentDir, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		_ = currentDir
		check(7,
			WithFileSystem(NewLocalFs()),
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

func TestYakFileMonitor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tempDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	err = createTestFileStructure(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	var currentEvent = make(chan *EventSet)
	_, err = WatchPath(ctx, tempDir, func(eventSet *EventSet) {
		if len(eventSet.ChangeEvents) == 0 && len(eventSet.CreateEvents) == 0 && len(eventSet.DeleteEvents) == 0 {
			return
		}
		currentEvent <- eventSet
	})
	if err != nil {
		t.Fatalf("watch path err: %v", err)
	}

	// rename
	pervPath := filepath.Join(tempDir, "dir1")
	currentPath := filepath.Join(tempDir, "dir3")
	err = os.Rename(pervPath, currentPath)
	if err != nil {
		t.Fatalf("rename err: %v", err)
	}
	select {
	case eventSet := <-currentEvent:
		require.Equal(t, 1, len(eventSet.DeleteEvents), "delete event count")
		require.Equal(t, 1, len(eventSet.CreateEvents), "create event count")
		require.Equal(t, pervPath, eventSet.DeleteEvents[0].Path, "delete event path")
		require.Equal(t, currentPath, eventSet.CreateEvents[0].Path, "create event path")
	case <-ctx.Done():
		t.Fatal("timeout")
	}

	// delete
	deletePath := filepath.Join(tempDir, "dir2")
	err = os.RemoveAll(deletePath)
	if err != nil {
		t.Fatalf("remove err: %v", err)
	}
	select {
	case eventSet := <-currentEvent:
		require.Equal(t, 1, len(eventSet.DeleteEvents), "delete event count")
		require.Equal(t, 0, len(eventSet.CreateEvents), "create event count")
		require.Equal(t, deletePath, eventSet.DeleteEvents[0].Path, "delete event path")
	case <-ctx.Done():
		t.Fatal("timeout")
	}

	// create
	createPath := filepath.Join(tempDir, "dir5")
	err = os.MkdirAll(createPath, 0755)
	if err != nil {
		t.Fatalf("create err: %v", err)
	}
	select {
	case eventSet := <-currentEvent:
		require.Equal(t, 0, len(eventSet.DeleteEvents), "delete event count")
		require.Equal(t, 1, len(eventSet.CreateEvents), "create event count")
		require.Equal(t, createPath, eventSet.CreateEvents[0].Path, "create event path")
	case <-ctx.Done():
		t.Fatal("timeout")
	}

}

func createTestFileStructure(basePath string) error {
	dirs := []string{
		"dir1",
		"dir1/subdir1",
		"dir2",
	}
	files := []string{
		"dir1/file1.txt",
		"dir1/subdir1/file2.txt",
		"dir2/file3.txt",
	}

	for _, dir := range dirs {
		path := filepath.Join(basePath, dir)
		err := os.MkdirAll(path, 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory %s: %v", path, err)
		}
	}

	for _, file := range files {
		path := filepath.Join(basePath, file)
		f, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %v", path, err)
		}
		f.Close()
	}

	return nil
}
