package ssatest

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

// TestScanProjectFiles_BootInfClasses_Java uses testfile/boot-inf-sample.jar (same pattern as GetJarFile).
func TestScanProjectFiles_BootInfClasses_Java(t *testing.T) {
	jarPath, err := GetBootInfSampleJarFile()
	require.NoError(t, err)

	const appEntry = "BOOT-INF/classes/com/acme/App.java"

	jarFs, err := javaclassparser.NewJarFSFromLocal(jarPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = jarFs.Close() })

	checkJava := func(path string) error {
		var b java2ssa.SSABuilder
		if b.FilterFile(path) {
			return nil
		}
		return fmt.Errorf("skip")
	}
	handlerHasApp := func(files []string) bool {
		e := filepath.ToSlash(appEntry)
		return slices.ContainsFunc(files, func(p string) bool {
			q := filepath.ToSlash(p)
			return q == e || strings.HasSuffix(q, "/"+e) || strings.HasSuffix(q, e)
		})
	}
	scan := func(exclude ssaapi.ExcludeFunc) []string {
		res, err := ssaapi.ScanProjectFiles(ssaapi.ScanConfig{
			ProgramName:   "p",
			ProgramPath:   ".",
			FileSystem:    jarFs,
			ExcludeFunc:   exclude,
			CheckLanguage: checkJava,
			Context:       context.Background(),
		})
		require.NoError(t, err)
		return res.HandlerFiles
	}

	t.Run("merged_default_exclude_files", func(t *testing.T) {
		files := scan(ssaapi.CompileExcludeFunc([]string{}, ""))
		require.True(t, handlerHasApp(files), "expected App.java, handler files: %v", files)
	})

	t.Run("empty_string_as_exclude_pattern", func(t *testing.T) {
		files := scan(ssaapi.CompileExcludeFunc([]string{""}, ""))
		require.True(t, handlerHasApp(files), "expected App.java, handler files: %v", files)
	})
}