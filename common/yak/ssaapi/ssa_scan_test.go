package ssaapi

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestScanProjectFiles(t *testing.T) {
	// Create a virtual filesystem for testing
	fs := filesys.NewVirtualFs()
	fs.AddFile("src/main.go", "package main")
	fs.AddFile("src/utils.go", "package main")
	fs.AddFile("src/vendor/lib.go", "package lib")
	fs.AddFile("src/test/test.go", "package test")
	fs.AddFile("src/testdata/issue47704.go", "package main")
	fs.AddFile("src/.git/config", "git config")
	fs.AddFile("src/.git/objects/pack/pack.go", "package ignored")
	fs.AddFile("src/target/generated.go", "package ignored")
	fs.AddFile("src/node_modules/pkg/index.go", "package ignored")
	fs.AddFile("src/ignored.txt", "ignored")

	// Define language checker (simulate handling Go files)
	checkLanguage := func(path string) error {
		if strings.HasSuffix(path, ".go") {
			return nil
		}
		return errors.New("not serve lang")
	}

	t.Run("Basic Scan", func(t *testing.T) {
		result, err := ScanProjectFiles(ScanConfig{
			ProgramName:     "test_prog",
			ProgramPath:     "src",
			FileSystem:      fs,
			ExcludeFunc:     nil,
			CheckLanguage:   checkLanguage,
			CheckPreHandler: nil,
			Context:         context.Background(),
		})
		require.NoError(t, err)
		require.NotNil(t, result)

		// Default excludes skip dependency/build/VCS roots, but keep test inputs.
		expectedFiles := []string{"src/main.go", "src/utils.go", "src/test/test.go", "src/testdata/issue47704.go"}
		require.ElementsMatch(t, expectedFiles, result.HandlerFiles)
		require.Equal(t, 4, result.HandlerTotal)
		require.GreaterOrEqual(t, len(result.Folders), 1)
	})

	t.Run("With Exclude", func(t *testing.T) {
		excludeFunc := newExcludeFunc([]string{"src/vendor/"}, "")

		result, err := ScanProjectFiles(ScanConfig{
			ProgramName:     "test_prog",
			ProgramPath:     "src",
			FileSystem:      fs,
			ExcludeFunc:     excludeFunc,
			CheckLanguage:   checkLanguage,
			CheckPreHandler: nil,
			Context:         context.Background(),
		})
		require.NoError(t, err)

		// Expected files: default excludes still skip generated roots; explicit exclude skips vendor.
		expectedFiles := []string{"src/main.go", "src/utils.go", "src/test/test.go", "src/testdata/issue47704.go"}
		require.ElementsMatch(t, expectedFiles, result.HandlerFiles)
		require.Equal(t, 4, result.HandlerTotal)
	})

	t.Run("Check PreHandler", func(t *testing.T) {
		checkPreHandler := func(path string) error {
			if path == "src/utils.go" {
				return nil
			}
			return errors.New("not serve lang")
		}

		result, err := ScanProjectFiles(ScanConfig{
			ProgramName:     "test_prog",
			ProgramPath:     "src",
			FileSystem:      fs,
			ExcludeFunc:     nil,
			CheckLanguage:   checkLanguage,
			CheckPreHandler: checkPreHandler,
			Context:         context.Background(),
		})
		require.NoError(t, err)

		require.Contains(t, result.PreHandlerFiles, "src/utils.go")
		require.NotContains(t, result.PreHandlerFiles, "src/main.go")
		require.Equal(t, 1, result.PreHandlerTotal)
		_, ok := result.HandlerFilesMap["src/utils.go"]
		require.True(t, ok)
	})

	t.Run("Keep testdata directory by default", func(t *testing.T) {
		result, err := ScanProjectFiles(ScanConfig{
			ProgramName:     "test_prog",
			ProgramPath:     "src",
			FileSystem:      fs,
			ExcludeFunc:     nil,
			CheckLanguage:   checkLanguage,
			CheckPreHandler: nil,
			Context:         context.Background(),
		})
		require.NoError(t, err)
		require.Contains(t, result.HandlerFiles, "src/testdata/issue47704.go")
	})

	t.Run("Skip testdata directory with explicit exclude", func(t *testing.T) {
		excludeFunc := newExcludeFunc([]string{"src/testdata/"}, "")

		result, err := ScanProjectFiles(ScanConfig{
			ProgramName:     "test_prog",
			ProgramPath:     "src",
			FileSystem:      fs,
			ExcludeFunc:     excludeFunc,
			CheckLanguage:   checkLanguage,
			CheckPreHandler: nil,
			Context:         context.Background(),
		})
		require.NoError(t, err)
		require.NotContains(t, result.HandlerFiles, "src/testdata/issue47704.go")
	})

	t.Run("Skip default excluded roots", func(t *testing.T) {
		result, err := ScanProjectFiles(ScanConfig{
			ProgramName:     "test_prog",
			ProgramPath:     "src",
			FileSystem:      fs,
			ExcludeFunc:     nil,
			CheckLanguage:   checkLanguage,
			CheckPreHandler: nil,
			Context:         context.Background(),
		})
		require.NoError(t, err)
		require.NotContains(t, result.HandlerFiles, "src/.git/objects/pack/pack.go")
		require.NotContains(t, result.HandlerFiles, "src/target/generated.go")
		require.NotContains(t, result.HandlerFiles, "src/node_modules/pkg/index.go")
	})

	t.Run("Skip root default excluded directories", func(t *testing.T) {
		rootFS := filesys.NewVirtualFs()
		rootFS.AddFile("main.go", "package main")
		rootFS.AddFile(".git/objects/pack/pack.go", "package ignored")
		rootFS.AddFile("target/generated.go", "package ignored")
		rootFS.AddFile("node_modules/pkg/index.go", "package ignored")

		result, err := ScanProjectFiles(ScanConfig{
			ProgramName:     "test_prog",
			ProgramPath:     ".",
			FileSystem:      rootFS,
			ExcludeFunc:     nil,
			CheckLanguage:   checkLanguage,
			CheckPreHandler: nil,
			Context:         context.Background(),
		})
		require.NoError(t, err)
		require.Contains(t, result.HandlerFiles, "main.go")
		require.NotContains(t, result.HandlerFiles, ".git/objects/pack/pack.go")
		require.NotContains(t, result.HandlerFiles, "target/generated.go")
		require.NotContains(t, result.HandlerFiles, "node_modules/pkg/index.go")
	})
}
