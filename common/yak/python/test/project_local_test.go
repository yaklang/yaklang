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
	"github.com/yaklang/yaklang/common/yak/python/python2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type pythonASTMetric struct {
	Path     string
	Duration time.Duration
}

func TestPythonASTParseLocalProject(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("local-only project AST parse test")
	}
	if os.Getenv("YAK_RUN_PYTHON_AST_PROJECT_LOCAL_TEST") == "" {
		t.Skip("set YAK_RUN_PYTHON_AST_PROJECT_LOCAL_TEST=1 to run local project AST parse checks")
	}

	projectFS, fileList := mustLoadLocalPythonProjectFS(t, pythonLocalProjectTarget(t))
	require.NotEmpty(t, fileList)

	builder, ok := python2ssa.CreateBuilder().(*python2ssa.SSABuilder)
	require.True(t, ok)
	defer builder.Clearup()

	cache := builder.GetAntlrCache()
	resetEveryFiles := pythonLocalASTResetEveryFiles()
	budget := pythonFixtureParseBudget()

	var slowFiles []pythonASTMetric
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
			slowFiles = append(slowFiles, pythonASTMetric{Path: path, Duration: elapsed})
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

	require.Empty(t, parseErrors, "python AST parse errors:\n%s", strings.Join(parseErrors, "\n"))
	require.Empty(t, slowFiles, "python AST parse exceeded budget=%s", budget)
}

func TestPythonCompileLocalProject(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("local-only project compile test")
	}
	if os.Getenv("YAK_RUN_PYTHON_PROJECT_COMPILE_LOCAL_TEST") == "" {
		t.Skip("set YAK_RUN_PYTHON_PROJECT_COMPILE_LOCAL_TEST=1 to run local project compile checks")
	}

	projectFS, fileList := mustLoadLocalPythonProjectFS(t, pythonLocalProjectTarget(t))
	require.NotEmpty(t, fileList)

	progs, err := ssaapi.ParseProjectWithFS(
		projectFS,
		ssaapi.WithLanguage(ssaconfig.PYTHON),
		ssaapi.WithMemory(true),
		ssaapi.WithFilePerformanceLog(os.Getenv("YAK_PYTHON_PROJECT_FILE_PERF") != ""),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)

	for _, prog := range progs {
		require.Len(t, prog.GetErrors(), 0, "project compile reported SSA errors:\n%s", formatPythonProjectErrorsByFile(prog.GetErrors()))
	}
}

func pythonLocalProjectTarget(t *testing.T) string {
	t.Helper()

	target := strings.TrimSpace(os.Getenv("YAK_PYTHON_PROJECT_TARGET"))
	if target == "" {
		t.Skip("set YAK_PYTHON_PROJECT_TARGET to a local python project path")
	}
	if _, err := os.Stat(target); err != nil {
		t.Skipf("target path not found: %s (%v)", target, err)
	}
	return target
}

func mustLoadLocalPythonProjectFS(t *testing.T, root string) (*filesys.VirtualFS, []string) {
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
			case ".git", ".hg", ".svn", "__pycache__", ".mypy_cache", ".pytest_cache", ".ruff_cache", ".venv", "venv", "node_modules":
				return filesys.SkipDir
			default:
				return nil
			}
		}),
		filesys.WithFileStat(func(filePath string, info os.FileInfo) error {
			if info.IsDir() || refFS.Ext(filePath) != ".py" {
				return nil
			}
			if strings.HasPrefix(filePath, "templates/") || strings.Contains(filePath, "/templates/") {
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

func pythonLocalASTResetEveryFiles() int {
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

func formatPythonProjectErrorsByFile(errs ssa.SSAErrors) string {
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
