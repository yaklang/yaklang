package test

import (
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/go2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type goASTMetric struct {
	Path     string
	Duration time.Duration
}

func TestGoASTParseLocalProject(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("local-only project AST parse test")
	}
	if os.Getenv("YAK_RUN_GO_AST_PROJECT_LOCAL_TEST") == "" {
		t.Skip("set YAK_RUN_GO_AST_PROJECT_LOCAL_TEST=1 to run local project AST parse checks")
	}

	projectFS, fileList := mustLoadLocalGoProjectFS(t, goLocalProjectTarget(t))
	require.NotEmpty(t, fileList)

	builder, ok := go2ssa.CreateBuilder().(*go2ssa.SSABuilder)
	require.True(t, ok)
	defer builder.Clearup()

	cache := builder.GetAntlrCache()
	resetEveryFiles := goLocalASTResetEveryFiles()
	budget := goFixtureParseBudget()

	var slowFiles []goASTMetric
	var parseErrors []string

	for index, path := range fileList {
		content, err := projectFS.ReadFile(path)
		require.NoError(t, err)

		start := time.Now()
		_, err = builder.ParseAST(utils.UnsafeBytesToString(content), cache)
		elapsed := time.Since(start)
		if err != nil {
			parseErrors = append(parseErrors, path+": "+err.Error())
			continue
		}
		if budget > 0 && elapsed > budget {
			slowFiles = append(slowFiles, goASTMetric{Path: path, Duration: elapsed})
		}
		if cache != nil && resetEveryFiles > 0 && (index+1)%resetEveryFiles == 0 {
			cache.ResetRuntimeCaches()
		}
	}

	sort.Slice(slowFiles, func(i, j int) bool {
		return slowFiles[i].Duration > slowFiles[j].Duration
	})
	for _, metric := range slowFiles {
		t.Logf("slow AST parse: %s (%s)", metric.Path, metric.Duration)
	}

	require.Empty(t, parseErrors, "go AST parse errors:\n%s", strings.Join(parseErrors, "\n"))
	require.Empty(t, slowFiles, "go AST parse exceeded budget=%s", budget)
}

func TestGoCompileLocalProject(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("local-only project compile test")
	}
	if os.Getenv("YAK_RUN_GO_PROJECT_COMPILE_LOCAL_TEST") == "" {
		t.Skip("set YAK_RUN_GO_PROJECT_COMPILE_LOCAL_TEST=1 to run local project compile checks")
	}

	projectFS, fileList := mustLoadLocalGoProjectFS(t, goLocalProjectTarget(t))
	require.NotEmpty(t, fileList)

	progs, err := ssaapi.ParseProjectWithFS(
		projectFS,
		ssaapi.WithLanguage(ssaconfig.GO),
		ssaapi.WithMemory(true),
		ssaapi.WithFilePerformanceLog(os.Getenv("YAK_GO_PROJECT_FILE_PERF") != ""),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)

	for _, prog := range progs {
		require.Len(t, prog.GetErrors(), 0, "project compile reported SSA errors:\n%s", formatGoProjectErrorsByFile(prog.GetErrors()))
	}
}

func goLocalProjectTarget(t *testing.T) string {
	t.Helper()

	target := strings.TrimSpace(os.Getenv("YAK_GO_PROJECT_TARGET"))
	if target == "" {
		t.Skip("set YAK_GO_PROJECT_TARGET to a local go project path")
	}
	if _, err := os.Stat(target); err != nil {
		t.Skipf("target path not found: %s (%v)", target, err)
	}
	return target
}

func mustLoadLocalGoProjectFS(t *testing.T, root string) (*filesys.VirtualFS, []string) {
	t.Helper()

	refFS := filesys.NewRelLocalFs(root)
	vfs := filesys.NewVirtualFs()
	fileList := make([]string, 0)

	err := filesys.Recursive(
		".",
		filesys.WithFileSystem(refFS),
		filesys.WithDirStat(func(fullPath string, info fs.FileInfo) error {
			_, folderName := refFS.PathSplit(fullPath)
			switch folderName {
			case ".git", ".hg", ".svn", "vendor", "node_modules", "testdata":
				return filesys.SkipDir
			default:
				return nil
			}
		}),
		filesys.WithFileStat(func(filePath string, info os.FileInfo) error {
			if info.IsDir() {
				return nil
			}

			// go.mod / go.work are needed for compile; mirror python keep-extras pattern.
			baseName := refFS.Base(filePath)
			switch baseName {
			case "go.mod", "go.sum", "go.work", "go.work.sum":
				raw, err := refFS.ReadFile(filePath)
				if err != nil {
					return err
				}
				vfs.AddFile(filePath, string(raw))
				return nil
			}

			if refFS.Ext(filePath) != ".go" {
				return nil
			}
			raw, err := refFS.ReadFile(filePath)
			if err != nil {
				return err
			}
			vfs.AddFile(filePath, string(raw))
			fileList = append(fileList, filePath)
			return nil
		}),
	)
	require.NoError(t, err)
	sort.Strings(fileList)
	return vfs, fileList
}

func goLocalASTResetEveryFiles() int {
	raw := strings.TrimSpace(os.Getenv("YAK_ANTLR_CACHE_RESET_FILES"))
	if raw == "" {
		return 100
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func formatGoProjectErrorsByFile(errs ssa.SSAErrors) string {
	if len(errs) == 0 {
		return ""
	}

	grouped := make(map[string][]string)
	order := make([]string, 0)
	for _, err := range errs {
		if err == nil {
			continue
		}
		path := "<unknown>"
		if err.Pos != nil {
			if editor := err.Pos.GetEditor(); editor != nil {
				path = strings.TrimPrefix(editor.GetFilePath(), "/")
				if path == "" {
					path = editor.GetFilename()
				}
			}
		}
		if _, ok := grouped[path]; !ok {
			order = append(order, path)
		}
		grouped[path] = append(grouped[path], err.String())
	}

	sort.Strings(order)

	var builder strings.Builder
	for _, path := range order {
		messages := grouped[path]
		fmt.Fprintf(&builder, "%s (%d)\n", path, len(messages))
		for _, message := range messages {
			builder.WriteString("  ")
			builder.WriteString(message)
			builder.WriteByte('\n')
		}
	}
	return builder.String()
}
