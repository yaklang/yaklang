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
	fs.AddFile("src/.git/config", "git config")
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

		// Expected files: src/main.go, src/utils.go, src/vendor/lib.go
		expectedFiles := []string{"src/main.go", "src/utils.go", "src/vendor/lib.go"}
		require.ElementsMatch(t, expectedFiles, result.HandlerFiles)
		require.Equal(t, 3, result.HandlerTotal)
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

		// Expected files: src/main.go, src/utils.go
		expectedFiles := []string{"src/main.go", "src/utils.go"}
		require.ElementsMatch(t, expectedFiles, result.HandlerFiles)
		require.Equal(t, 2, result.HandlerTotal)
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
}
