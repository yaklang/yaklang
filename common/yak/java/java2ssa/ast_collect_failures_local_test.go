package java2ssa_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
)

func TestCollectASTParseFailures_DecompiledCodeTarget(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("local-only AST fixture collector")
	}
	if os.Getenv("YAK_RUN_AST_FAILURE_COLLECTOR_TEST") == "" {
		t.Skip("set YAK_RUN_AST_FAILURE_COLLECTOR_TEST=1 to collect AST failure fixtures")
	}

	target := strings.TrimSpace(os.Getenv("YAK_AST_FAILURE_TARGET"))
	if target == "" {
		target = "/home/wlz/Target/decompiled-code-target"
	}

	wd, err := os.Getwd()
	require.NoError(t, err)

	fixtureRoot := filepath.Join(wd, "test", "syntax", "collected_decompiled_code_target")
	require.NoError(t, os.RemoveAll(fixtureRoot))
	require.NoError(t, os.MkdirAll(fixtureRoot, 0o755))

	refFS := filesys.NewRelLocalFs(target)
	if _, err := refFS.Stat("."); err != nil {
		t.Skipf("target path not found: %s (%v)", target, err)
	}

	fileList := collectASTFailureFiles(t, refFS)
	require.NotEmpty(t, fileList)

	builder, ok := java2ssa.CreateBuilder().(*java2ssa.SSABuilder)
	require.True(t, ok)
	defer builder.Clearup()

	cache := builder.GetAntlrCache()

	manifestLines := make([]string, 0, len(fileList))
	failureCount := 0
	for _, fileName := range fileList {
		content, err := refFS.ReadFile(fileName)
		require.NoError(t, err)

		_, err = builder.ParseAST(utils.UnsafeBytesToString(content), cache)
		if err == nil {
			continue
		}

		outPath := filepath.Join(fixtureRoot, filepath.FromSlash(fileName))
		require.NoError(t, os.MkdirAll(filepath.Dir(outPath), 0o755))
		require.NoError(t, os.WriteFile(outPath, content, 0o644))

		manifestLines = append(manifestLines, fileName+"\t"+err.Error())
		failureCount++
	}

	sort.Strings(manifestLines)
	if len(manifestLines) > 0 {
		manifestPath := filepath.Join(fixtureRoot, "_manifest.txt")
		require.NoError(t, os.WriteFile(manifestPath, []byte(strings.Join(manifestLines, "\n")+"\n"), 0o644))
	}

	t.Logf("collected %d AST failure fixture(s) into %s", failureCount, fixtureRoot)
}

func collectASTFailureFiles(t *testing.T, refFS *filesys.RelLocalFs) []string {
	t.Helper()

	limit := 0
	if raw := strings.TrimSpace(os.Getenv("YAK_AST_FAILURE_LIMIT")); raw != "" {
		value, err := strconv.Atoi(raw)
		require.NoError(t, err)
		if value > 0 {
			limit = value
		}
	}

	fileList := make([]string, 0)
	err := filesys.Recursive(".",
		filesys.WithFileSystem(refFS),
		filesys.WithDirStat(func(fullPath string, fi fs.FileInfo) error {
			_, folderName := refFS.PathSplit(fullPath)
			if folderName == "test" || folderName == ".git" {
				return filesys.SkipDir
			}
			return nil
		}),
		filesys.WithFileStat(func(fileName string, fi os.FileInfo) error {
			if fi.IsDir() || refFS.Ext(fileName) != ".java" {
				return nil
			}
			fileList = append(fileList, fileName)
			return nil
		}),
	)
	require.NoError(t, err)

	sort.Strings(fileList)
	if limit > 0 && len(fileList) > limit {
		return fileList[:limit]
	}
	return fileList
}
