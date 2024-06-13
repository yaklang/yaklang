package filesys

import (
	"context"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
	_, err := WatchPath(context.Background(), "test", func(eventSet EventSet) {

		createEvents := eventSet.CreateEvents
		deleteEvents := eventSet.DeleteEvents

		createEventsMap := make(map[string][]Event)
		for _, event := range createEvents {
			dir, _ := filepath.Split(event.Path)
			findPath := false
			for path, events := range createEventsMap {
				if isSubDir(path, dir) {
					createEventsMap[path] = append(events, event)
					findPath = true
					break
				} else if isSubDir(dir, path) {
					createEventsMap[dir] = append(events, event)
					delete(createEventsMap, path)
					findPath = true
					break
				}
			}
			if !findPath {
				createEventsMap[dir] = []Event{event}
			}
		}

		deleteEventsMap := make(map[string][]Event)
		for _, event := range deleteEvents {
			if event.IsDir {
				deleteEventsMap[event.Path] = []Event{event}
			}
			dir, _ := filepath.Split(event.Path)
			for path, events := range deleteEventsMap {
				if isSubDir(path, dir) {
					deleteEventsMap[path] = append(events, event)
				} else if isSubDir(dir, path) {
					deleteEventsMap[dir] = append(events, event)
					delete(deleteEventsMap, path)
				}
			}

		}

		//for path, event := range deleteEvents {
		//	if event.IsDir {
		//		dir, _ := filepath.Split(event.Path)
		//		if _, ok := createEventsMap[dir]; ok {
		//
		//		}
		//	}
		//}

	})
	if err != nil {
		return
	}
	select {}
}

func isSubDir(basePath, targetPath string) bool {
	// Clean the paths to remove any unnecessary components
	basePath = filepath.Clean(basePath)
	targetPath = filepath.Clean(targetPath)

	// Get the relative path from basePath to targetPath
	rel, err := filepath.Rel(basePath, targetPath)
	if err != nil {
		return false
	}

	// If the relative path starts with ".." or is ".", targetPath is not a subdir of basePath
	return !filepath.IsAbs(rel) && rel != "." && !strings.HasPrefix(rel, "../")
}
