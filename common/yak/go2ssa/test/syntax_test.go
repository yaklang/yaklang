package test

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/go2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

//go:embed code/***
var codeFs embed.FS

var goTestAntlrCache = func() *ssa.AntlrCache {
	return go2ssa.CreateBuilder().GetAntlrCache()
}()

func goFixtureParseBudget() time.Duration {
	raw := strings.TrimSpace(os.Getenv("YAK_GO_FIXTURE_PARSE_BUDGET_SEC"))
	if raw == "" {
		return 30 * time.Second
	}
	sec, err := strconv.Atoi(raw)
	if err != nil || sec <= 0 {
		return 0
	}
	return time.Duration(sec) * time.Second
}

func goProjectParseBudget() time.Duration {
	raw := strings.TrimSpace(os.Getenv("YAK_GO_PROJECT_AST_BUDGET_SEC"))
	if raw == "" {
		return 30 * time.Second
	}
	sec, err := strconv.Atoi(raw)
	if err != nil || sec <= 0 {
		return 0
	}
	return time.Duration(sec) * time.Second
}

func isGoSyntaxASTFixture(fixturePath string) bool {
	ext := strings.ToLower(filepath.Ext(fixturePath))
	return ext == "" || ext == ".go"
}

func goFixtureVirtualPath(filename string) string {
	trimmed := strings.TrimPrefix(filepath.ToSlash(filename), "code/")
	if trimmed == "" {
		trimmed = "fixture"
	}
	if filepath.Ext(trimmed) == "" {
		trimmed += ".go"
	}
	return path.Join("fixture", trimmed)
}

func captureStdoutAndLogs(t *testing.T, fn func()) (captured string) {
	t.Helper()

	oldStdout := os.Stdout
	oldStderr := os.Stderr

	reader, writer, err := os.Pipe()
	require.NoError(t, err)

	outputCh := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, reader)
		outputCh <- buf.String()
	}()

	restore := func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
		log.SetOutput(oldStdout)
	}

	os.Stdout = writer
	os.Stderr = writer
	log.SetOutput(writer)

	defer func() {
		restore()
		_ = writer.Close()
		captured = <-outputCh
	}()

	fn()
	return captured
}

func buildOutputHasRecoveredPanic(output string) bool {
	markers := []string{
		"Current goroutine call stack:",
		"compile panic:",
		"parse error with panic :",
		"panic({",
	}
	for _, marker := range markers {
		if strings.Contains(output, marker) {
			return true
		}
	}
	return false
}

func trimCapturedOutput(output string) string {
	const maxLen = 4000
	if len(output) <= maxLen {
		return output
	}
	return output[len(output)-maxLen:]
}

func validateBuildFromSource(t *testing.T, filename string, src string) {
	t.Helper()

	vf := filesys.NewVirtualFs()
	vf.AddFile("fixture/go.mod", "module fixture\n\ngo 1.20\n")
	vf.AddFile(goFixtureVirtualPath(filename), src)

	var err error
	output := captureStdoutAndLogs(t, func() {
		_, err = ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.GO))
	})
	require.NoError(t, err, "build from AST fixture failed: %s", filename)
	require.False(t, buildOutputHasRecoveredPanic(output), "build from AST fixture emitted recovered panic logs for %s:\n%s", filename, trimCapturedOutput(output))
}

func validateSource(t *testing.T, filename string, src string) {
	t.Run(fmt.Sprintf("syntax file: %v", filename), func(t *testing.T) {
		start := time.Now()
		_, err := go2ssa.Frontend(src, goTestAntlrCache)
		elapsed := time.Since(start)
		require.NoError(t, err, "parse AST FrontEnd error: %v", err)
		if budget := goFixtureParseBudget(); budget > 0 && elapsed > budget {
			t.Fatalf("parse AST exceeded budget for %s: elapsed=%s budget=%s", filename, elapsed, budget)
		}
		validateBuildFromSource(t, filename, src)
	})
}

func TestAllSyntaxForGo_G4(t *testing.T) {
	err := fs.WalkDir(codeFs, "code", func(codePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isGoSyntaxASTFixture(codePath) {
			return nil
		}
		raw, err := codeFs.ReadFile(codePath)
		if err != nil {
			return fmt.Errorf("cannot found syntax fs %s: %w", codePath, err)
		}
		validateSource(t, codePath, string(raw))
		return nil
	})
	require.NoError(t, err, "walk go syntax fixtures")
}

type ParseError struct {
	Duration time.Duration
	Message  string
}

type BuildFailure struct {
	Error  string
	Output string
}

func goProjectASTRoot(t *testing.T) string {
	t.Helper()

	if root := os.Getenv("YAK_GO_PROJECT_AST_TARGET"); root != "" {
		return root
	}

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	return filepath.Join(home, "Target", "gocms", "GoBlog")
}

func collectGoProjectFiles(t *testing.T, root string) ([]string, map[string]struct{}, *filesys.RelLocalFs) {
	t.Helper()

	fileList := make([]string, 0, 128)
	fileMap := make(map[string]struct{})
	refFs := filesys.NewRelLocalFs(root)

	err := filesys.Recursive(".",
		filesys.WithFileSystem(refFs),
		filesys.WithDirStat(func(dirPath string, fi fs.FileInfo) error {
			switch filepath.Base(dirPath) {
			case ".git", "vendor":
				return fs.SkipDir
			}
			return nil
		}),
		filesys.WithFileStat(func(filePath string, fi fs.FileInfo) error {
			if filepath.Ext(filePath) != ".go" {
				return nil
			}
			fileList = append(fileList, filePath)
			fileMap[filePath] = struct{}{}
			return nil
		}),
	)
	require.NoError(t, err)
	sort.Strings(fileList)
	return fileList, fileMap, refFs
}

func cloneProjectFileAsVirtualFS(t *testing.T, refFs *filesys.RelLocalFs, filePath string) *filesys.VirtualFS {
	t.Helper()

	vf := filesys.NewVirtualFs()
	if goMod, err := refFs.ReadFile("go.mod"); err == nil {
		vf.AddFile("go.mod", string(goMod))
	}

	raw, err := refFs.ReadFile(filePath)
	require.NoError(t, err)
	vf.AddFile(filePath, string(raw))
	return vf
}

func cloneProjectPathsAsVirtualFS(t *testing.T, refFs *filesys.RelLocalFs, filePaths []string) *filesys.VirtualFS {
	t.Helper()

	vf := filesys.NewVirtualFs()
	if goMod, err := refFs.ReadFile("go.mod"); err == nil {
		vf.AddFile("go.mod", string(goMod))
	}
	for _, filePath := range filePaths {
		raw, err := refFs.ReadFile(filePath)
		require.NoError(t, err)
		vf.AddFile(filePath, string(raw))
	}
	return vf
}

func TestProjectAst(t *testing.T) {
	if os.Getenv("YAK_GO_RUN_PROJECT_AST") == "" {
		t.Skip("set YAK_GO_RUN_PROJECT_AST=1 to run local gocms project AST integration")
	}

	root := goProjectASTRoot(t)
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("project ast target unavailable: %s: %v", root, err)
	}

	fileList, fileMap, refFs := collectGoProjectFiles(t, root)
	errorFiles := make(map[string]ParseError)
	slowFiles := make(map[string]time.Duration)

	config, err := ssaapi.DefaultConfig(
		ssaapi.WithFileSystem(refFs),
		ssaapi.WithProgramPath("."),
		ssaapi.WithLanguage(ssaconfig.GO),
	)
	require.NoError(t, err)
	require.NotNil(t, config)

	start := time.Now()
	ch := config.GetFileHandler(refFs, fileList, fileMap)
	for fileContent := range ch {
		log.Infof("go project ast: %s size[%s] time[%s]", fileContent.Path, ssaapi.Size(len(fileContent.Content)), fileContent.Duration)
		if budget := goProjectParseBudget(); budget > 0 && fileContent.Duration > budget {
			slowFiles[fileContent.Path] = fileContent.Duration
		}
		if fileContent.Err != nil {
			errorFiles[fileContent.Path] = ParseError{
				Duration: fileContent.Duration,
				Message:  fileContent.Err.Error(),
			}
		}
	}
	log.Infof("go project AST parsed %d files under %s in %s", len(fileList), root, time.Since(start))

	failedFiles := make([]string, 0, len(errorFiles))
	for path := range errorFiles {
		failedFiles = append(failedFiles, path)
	}
	sort.Strings(failedFiles)
	for _, path := range failedFiles {
		parseErr := errorFiles[path]
		log.Errorf("go project AST parse failed: %s duration=%s err=%s", path, parseErr.Duration, parseErr.Message)
	}

	slowFileList := make([]string, 0, len(slowFiles))
	for path := range slowFiles {
		slowFileList = append(slowFileList, path)
	}
	sort.Strings(slowFileList)
	for _, path := range slowFileList {
		log.Warnf("go project AST slow file: %s duration=%s", path, slowFiles[path])
	}

	require.Empty(t, failedFiles, "project AST parse failed for %d files under %s: %v", len(failedFiles), root, failedFiles)
	require.Empty(t, slowFileList, "project AST exceeded budget for %d files under %s: %v", len(slowFileList), root, slowFileList)
}

func TestProjectBuild(t *testing.T) {
	if os.Getenv("YAK_GO_RUN_PROJECT_AST") == "" {
		t.Skip("set YAK_GO_RUN_PROJECT_AST=1 to run local gocms project build integration")
	}

	root := goProjectASTRoot(t)
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("project build target unavailable: %s: %v", root, err)
	}

	refFs := filesys.NewRelLocalFs(root)
	var err error
	output := captureStdoutAndLogs(t, func() {
		_, err = ssaapi.ParseProjectWithFS(
			refFs,
			ssaapi.WithProgramPath("."),
			ssaapi.WithLanguage(ssaconfig.GO),
		)
	})
	require.NoError(t, err, "project build failed for %s", root)
	require.False(t, buildOutputHasRecoveredPanic(output), "project build emitted recovered panic logs for %s:\n%s", root, trimCapturedOutput(output))
}

func TestProjectSingleFileBuild(t *testing.T) {
	if os.Getenv("YAK_GO_RUN_PROJECT_SINGLE_FILE_BUILD") == "" {
		t.Skip("set YAK_GO_RUN_PROJECT_SINGLE_FILE_BUILD=1 to run per-file local gocms build integration")
	}

	root := goProjectASTRoot(t)
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("project single-file build target unavailable: %s: %v", root, err)
	}

	fileList, _, refFs := collectGoProjectFiles(t, root)
	failures := make(map[string]BuildFailure)

	for _, filePath := range fileList {
		vf := cloneProjectFileAsVirtualFS(t, refFs, filePath)

		var err error
		output := captureStdoutAndLogs(t, func() {
			_, err = ssaapi.ParseProjectWithFS(
				vf,
				ssaapi.WithProgramPath("."),
				ssaapi.WithLanguage(ssaconfig.GO),
			)
		})
		if err == nil && !buildOutputHasRecoveredPanic(output) {
			continue
		}
		failures[filePath] = BuildFailure{
			Output: trimCapturedOutput(output),
		}
		if err != nil {
			failures[filePath] = BuildFailure{
				Error:  err.Error(),
				Output: trimCapturedOutput(output),
			}
		}
		log.Errorf("go single-file build failed: %s err=%v", filePath, err)
	}

	if len(failures) == 0 {
		return
	}

	failedFiles := make([]string, 0, len(failures))
	for path := range failures {
		failedFiles = append(failedFiles, path)
	}
	sort.Strings(failedFiles)
	for _, path := range failedFiles {
		failure := failures[path]
		log.Errorf("go single-file build failure: %s err=%s output=%s", path, failure.Error, failure.Output)
	}

	require.Empty(t, failedFiles, "single-file build failures under %s: %v", root, failedFiles)
}

func TestProjectPackageBuild(t *testing.T) {
	if os.Getenv("YAK_GO_RUN_PROJECT_PACKAGE_BUILD") == "" {
		t.Skip("set YAK_GO_RUN_PROJECT_PACKAGE_BUILD=1 to run per-package local gocms build integration")
	}

	root := goProjectASTRoot(t)
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("project package build target unavailable: %s: %v", root, err)
	}

	fileList, _, refFs := collectGoProjectFiles(t, root)
	packages := make(map[string][]string)
	for _, filePath := range fileList {
		dir := filepath.Dir(filePath)
		packages[dir] = append(packages[dir], filePath)
	}

	failures := make(map[string]BuildFailure)
	packageDirs := make([]string, 0, len(packages))
	for dir := range packages {
		packageDirs = append(packageDirs, dir)
	}
	sort.Strings(packageDirs)

	for _, dir := range packageDirs {
		files := packages[dir]
		sort.Strings(files)
		vf := cloneProjectPathsAsVirtualFS(t, refFs, files)

		var err error
		output := captureStdoutAndLogs(t, func() {
			_, err = ssaapi.ParseProjectWithFS(
				vf,
				ssaapi.WithProgramPath("."),
				ssaapi.WithLanguage(ssaconfig.GO),
			)
		})
		if err == nil && !buildOutputHasRecoveredPanic(output) {
			continue
		}
		failures[dir] = BuildFailure{
			Output: trimCapturedOutput(output),
		}
		if err != nil {
			failures[dir] = BuildFailure{
				Error:  err.Error(),
				Output: trimCapturedOutput(output),
			}
		}
		log.Errorf("go package build failed: %s err=%v", dir, err)
	}

	if len(failures) == 0 {
		return
	}

	failedPackages := make([]string, 0, len(failures))
	for dir := range failures {
		failedPackages = append(failedPackages, dir)
	}
	sort.Strings(failedPackages)
	for _, dir := range failedPackages {
		failure := failures[dir]
		log.Errorf("go package build failure: %s err=%s output=%s", dir, failure.Error, failure.Output)
	}

	require.Empty(t, failedPackages, "package build failures under %s: %v", root, failedPackages)
}
