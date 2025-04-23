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
			if fi.Name() == ".DS_Store" {
				return nil
			}
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
			if fi.Name() == ".DS_Store" {
				return nil
			}
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
				if fi.Name() == ".DS_Store" {
					return nil
				}
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
			if fi.Name() == ".DS_Store" {
				return nil
			}
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

func TestFS_SkipAll(t *testing.T) {
	// 创建一个临时目录用于测试
	tempDir, err := os.MkdirTemp("", "test-skipall")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// 创建测试文件结构
	// 创建3个目录，每个目录包含2个文件
	for i := 1; i <= 3; i++ {
		dirPath := filepath.Join(tempDir, fmt.Sprintf("dir%d", i))
		err := os.Mkdir(dirPath, 0755)
		if err != nil {
			t.Fatal(err)
		}

		for j := 1; j <= 2; j++ {
			filePath := filepath.Join(dirPath, fmt.Sprintf("file%d.txt", j))
			f, err := os.Create(filePath)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
		}
	}

	// 测试文件数量限制触发 SkipAll
	t.Run("file limit skip all", func(t *testing.T) {
		processedFiles := 0
		err := Recursive(
			tempDir,
			WithFileSystem(NewLocalFs()),
			WithFileLimit(2), // 限制最多处理2个文件
			WithFileStat(func(s string, fi fs.FileInfo) error {
				processedFiles++
				return nil
			}),
		)
		if err != nil {
			t.Fatal(err)
		}
		if processedFiles != 2 {
			t.Fatalf("expected to process exactly 2 files, got %d", processedFiles)
		}
	})

	// 测试目录数量限制触发 SkipAll
	t.Run("dir limit skip all", func(t *testing.T) {
		processedDirs := 0
		err := Recursive(
			tempDir,
			WithFileSystem(NewLocalFs()),
			WithDirLimit(1), // 限制最多处理1个目录
			WithDirStat(func(s string, fi fs.FileInfo) error {
				processedDirs++
				return nil
			}),
		)
		if err != nil {
			t.Fatal(err)
		}
		if processedDirs != 1 {
			t.Fatalf("expected to process exactly 1 directory, got %d", processedDirs)
		}
	})

	// 测试总数量限制触发 SkipAll
	t.Run("total limit skip all", func(t *testing.T) {
		processedTotal := 0
		err := Recursive(
			tempDir,
			WithFileSystem(NewLocalFs()),
			WithTotalLimit(3), // 限制最多处理3个条目
			WithStat(func(isDir bool, pathname string, info fs.FileInfo) error {
				processedTotal++
				return nil
			}),
		)
		if err != nil {
			t.Fatal(err)
		}
		if processedTotal != 3 {
			t.Fatalf("expected to process exactly 3 items, got %d", processedTotal)
		}
	})

	// 测试 SkipAll 是否正确停止所有处理
	t.Run("skip all stops all processing", func(t *testing.T) {
		processedCount := 0
		shouldSkip := false
		err := Recursive(
			tempDir,
			WithFileSystem(NewLocalFs()),
			WithStat(func(isDir bool, pathname string, info fs.FileInfo) error {
				processedCount++
				if processedCount == 3 {
					shouldSkip = true
					return SkipAll
				}
				return nil
			}),
		)
		if err != nil {
			t.Fatal(err)
		}
		if processedCount != 3 {
			t.Fatalf("expected to process exactly 3 items before SkipAll, got %d", processedCount)
		}
		if !shouldSkip {
			t.Fatal("SkipAll was not triggered")
		}
	})

	// 测试 SkipAll 在递归目录中的行为
	t.Run("skip all in recursive directories", func(t *testing.T) {
		processedCount := 0
		err := Recursive(
			tempDir,
			WithFileSystem(NewLocalFs()),
			WithTotalLimit(4), // 限制总处理数量
			WithStat(func(isDir bool, pathname string, info fs.FileInfo) error {
				processedCount++
				return nil
			}),
		)
		if err != nil {
			t.Fatal(err)
		}
		if processedCount != 4 {
			t.Fatalf("expected to process exactly 4 items, got %d", processedCount)
		}
	})

	// 测试多个限制同时存在时的行为
	t.Run("multiple limits", func(t *testing.T) {
		processedCount := 0
		err := Recursive(
			tempDir,
			WithFileSystem(NewLocalFs()),
			WithFileLimit(1),  // 文件限制
			WithDirLimit(1),   // 目录限制
			WithTotalLimit(2), // 总数量限制
			WithStat(func(isDir bool, pathname string, info fs.FileInfo) error {
				processedCount++
				return nil
			}),
		)
		if err != nil {
			t.Fatal(err)
		}
		if processedCount > 2 {
			t.Fatalf("expected to process at most 2 items, got %d", processedCount)
		}
	})

	// 测试 SkipAll 在错误处理中的行为
	t.Run("skip all with errors", func(t *testing.T) {
		processedCount := 0
		errorCount := 0
		err := Recursive(
			tempDir,
			WithFileSystem(NewLocalFs()),
			WithStat(func(isDir bool, pathname string, info fs.FileInfo) error {
				processedCount++
				if processedCount == 3 {
					return SkipAll
				}
				if processedCount%2 == 0 {
					errorCount++
					return fmt.Errorf("test error")
				}
				return nil
			}),
		)
		if err != nil {
			t.Fatal(err)
		}
		if processedCount != 3 {
			t.Fatalf("expected to process exactly 3 items, got %d", processedCount)
		}
		if errorCount == 0 {
			t.Fatal("expected to encounter some errors")
		}
	})
}
