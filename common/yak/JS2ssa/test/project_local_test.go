package test

import (
	"fmt"
	"io/fs"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/typescript/ts2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type jsASTMetric struct {
	Path     string
	Duration time.Duration
}

// TestJSASTParseLocalProject parses every js file of a local
// project (YAK_JS_PROJECT_TARGET) through the frontend only,
// asserting zero syntax errors and reporting files that exceed the per-file
// parse budget.
//
// Metrics profiling: setting YAK_JS_PROJECT_METRICS=1 additionally
// logs one structured AST_METRIC line per file (parse duration, heap alloc/
// inuse delta, ANTLR DFA/prediction-context cache growth, error), plus the
// SLL-first counters summary at the end. Setting YAK_START_PPROF=1 starts a
// pprof endpoint on :18080 for live CPU/heap profiling.
//
// File selection: YAK_JS_PROJECT_FILE restricts the run to one
// file (relative to the project root) and YAK_JS_PROJECT_LIMIT
// caps the number of files.
//
// Local-only: skipped on GitHub Actions and unless
// YAK_RUN_JS_AST_PROJECT_LOCAL_TEST=1.
func TestJSASTParseLocalProject(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("local-only project AST parse test")
	}
	if os.Getenv("YAK_RUN_JS_AST_PROJECT_LOCAL_TEST") == "" {
		t.Skip("set YAK_RUN_JS_AST_PROJECT_LOCAL_TEST=1 to run local project AST parse checks")
	}

	projectFS, fileList := mustLoadLocalJSProjectFS(t, jsLocalProjectTarget(t))
	fileList = selectJSMetricFiles(t, fileList)
	require.NotEmpty(t, fileList)

	metricsEnabled := os.Getenv("YAK_JS_PROJECT_METRICS") != ""
	if metricsEnabled {
		antlr4util.ResetSLLFirstCounters()
		if os.Getenv("YAK_START_PPROF") != "" {
			go func() {
				_ = http.ListenAndServe(":18080", nil)
			}()
		}
	}

	builder, ok := ts2ssa.CreateBuilder().(*ts2ssa.SSABuilder)
	require.True(t, ok)
	defer builder.Clearup()

	cache := builder.GetAntlrCache()
	resetEveryFiles := jsLocalASTResetEveryFiles()
	budget := jsLocalParseBudget()

	var slowFiles []jsASTMetric
	var parseErrors []string

	for index, path := range fileList {
		content, err := projectFS.ReadFile(path)
		require.NoError(t, err)

		var memBefore, memAfter runtime.MemStats
		var cacheBefore, cacheAfter jsAntlrCacheStats
		if metricsEnabled {
			runtime.ReadMemStats(&memBefore)
			cacheBefore = readJSAntlrCacheStats(cache)
		}
		start := time.Now()
		_, err = builder.ParseAST(utils.UnsafeBytesToString(content), cache)
		parseDur := time.Since(start)
		if metricsEnabled {
			runtime.ReadMemStats(&memAfter)
			cacheAfter = readJSAntlrCacheStats(cache)
		}

		if metricsEnabled {
			log.Infof(
				"AST_METRIC\tlang=js\tindex=%d/%d\tfile=%s\tsize=%s\tparse=%s\theap_alloc=%s->%s (%s)\theap_inuse=%s->%s (%s)\tcache_lexer_dfa=%d->%d (%+d)\tcache_parser_dfa=%d->%d (%+d)\tcache_lexer_ctx=%d->%d (%+d)\tcache_parser_ctx=%d->%d (%+d)\terr=%v",
				index+1, len(fileList), path,
				formatJSMetricBytes(uint64(len(content))), parseDur,
				formatJSMetricBytes(memBefore.HeapAlloc), formatJSMetricBytes(memAfter.HeapAlloc), formatJSMetricBytesDelta(int64(memAfter.HeapAlloc)-int64(memBefore.HeapAlloc)),
				formatJSMetricBytes(memBefore.HeapInuse), formatJSMetricBytes(memAfter.HeapInuse), formatJSMetricBytesDelta(int64(memAfter.HeapInuse)-int64(memBefore.HeapInuse)),
				cacheBefore.LexerDFAStates, cacheAfter.LexerDFAStates, cacheAfter.LexerDFAStates-cacheBefore.LexerDFAStates,
				cacheBefore.ParserDFAStates, cacheAfter.ParserDFAStates, cacheAfter.ParserDFAStates-cacheBefore.ParserDFAStates,
				cacheBefore.LexerPredCtx, cacheAfter.LexerPredCtx, cacheAfter.LexerPredCtx-cacheBefore.LexerPredCtx,
				cacheBefore.ParserPredCtx, cacheAfter.ParserPredCtx, cacheAfter.ParserPredCtx-cacheBefore.ParserPredCtx,
				err,
			)
		}

		if err != nil {
			parseErrors = append(parseErrors, path+": "+err.Error())
			continue
		}
		if budget > 0 && parseDur > budget {
			slowFiles = append(slowFiles, jsASTMetric{Path: path, Duration: parseDur})
		}
		if cache != nil && resetEveryFiles > 0 && (index+1)%resetEveryFiles == 0 {
			cache.ResetRuntimeCaches()
		}
	}

	if metricsEnabled {
		stats := antlr4util.SLLFirstCountersSnapshot()
		log.Infof(
			"[antlr-sll-first] lang=js ll_only=%d sll_attempts=%d fallbacks=%d cancelled=%d error=%d",
			stats.LLOnly, stats.SLLAttempts, stats.Fallbacks, stats.FallbackCancelled, stats.FallbackError,
		)
	}

	sort.Slice(slowFiles, func(i, j int) bool {
		return slowFiles[i].Duration > slowFiles[j].Duration
	})
	for _, metric := range slowFiles {
		t.Logf("slow AST parse: %s (%s)", metric.Path, metric.Duration)
	}

	require.Empty(t, parseErrors, "js AST parse errors:\n%s", strings.Join(parseErrors, "\n"))
	require.Empty(t, slowFiles, "js AST parse exceeded budget=%s", budget)
}

// TestJSCompileLocalProject compiles a local project
// (YAK_JS_PROJECT_TARGET) in-memory through the full SSA pipeline
// (parse + IR build) and records the SSA errors each program reported.
// Compile errors are logged per file, not asserted: real-world projects
// legitimately produce semantic SSA errors (missing cross-file types,
// closure capture hints) that must not fail the run.
//
// Local-only: skipped on GitHub Actions and unless
// YAK_RUN_JS_PROJECT_COMPILE_LOCAL_TEST=1.
func TestJSCompileLocalProject(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("local-only project compile test")
	}
	if os.Getenv("YAK_RUN_JS_PROJECT_COMPILE_LOCAL_TEST") == "" {
		t.Skip("set YAK_RUN_JS_PROJECT_COMPILE_LOCAL_TEST=1 to run local project compile checks")
	}

	projectFS, fileList := mustLoadLocalJSProjectFS(t, jsLocalProjectTarget(t))
	require.NotEmpty(t, fileList)

	progs, err := ssaapi.ParseProjectWithFS(
		projectFS,
		ssaapi.WithLanguage(ssaconfig.JS),
		ssaapi.WithMemory(true),
		ssaapi.WithFilePerformanceLog(os.Getenv("YAK_JS_PROJECT_FILE_PERF") != ""),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)

	jsLogProjectErrors("project compile", progs)
}

// TestJSCompileLocalProjectWithDatabase compiles a local project
// (YAK_JS_PROJECT_TARGET) with IR persistence into a temporary SSA
// SQLite database, exercising the SaveToDatabase path (instruction marshal,
// batched writes) that pure in-memory compiles skip. Compile errors are
// logged per file, not asserted (see TestJSCompileLocalProject).
//
// Local-only: skipped on GitHub Actions and unless
// YAK_RUN_JS_PROJECT_DB_LOCAL_TEST=1.
func TestJSCompileLocalProjectWithDatabase(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("local-only project compile-with-database test")
	}
	if os.Getenv("YAK_RUN_JS_PROJECT_DB_LOCAL_TEST") == "" {
		t.Skip("set YAK_RUN_JS_PROJECT_DB_LOCAL_TEST=1 to run local project compile + database persistence checks")
	}

	projectFS, fileList := mustLoadLocalJSProjectFS(t, jsLocalProjectTarget(t))
	require.NotEmpty(t, fileList)

	db, err := consts.GetTempSSADataBase()
	require.NoError(t, err)
	old := consts.GetGormSSAProjectDataBase()
	consts.SetGormSSAProjectDatabase(db)
	defer consts.SetGormSSAProjectDatabase(old)

	progs, err := ssaapi.ParseProjectWithFS(
		projectFS,
		ssaapi.WithLanguage(ssaconfig.JS),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)

	jsLogProjectErrors("project compile (with database)", progs)
}

func jsLocalProjectTarget(t *testing.T) string {
	t.Helper()

	target := strings.TrimSpace(os.Getenv("YAK_JS_PROJECT_TARGET"))
	if target == "" {
		t.Skip("set YAK_JS_PROJECT_TARGET to a local js project path")
	}
	if _, err := os.Stat(target); err != nil {
		t.Skipf("target path not found: %s (%v)", target, err)
	}
	return target
}

func mustLoadLocalJSProjectFS(t *testing.T, root string) (*filesys.VirtualFS, []string) {
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
			case ".git", ".hg", ".svn", "node_modules", "vendor":
				return filesys.SkipDir
			default:
				return nil
			}
		}),
		filesys.WithFileStat(func(filePath string, info os.FileInfo) error {
			if info.IsDir() {
				return nil
			}
			if ext := strings.ToLower(refFS.Ext(filePath)); ext != ".js" {
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

// selectJSMetricFiles narrows the file list for the AST layer:
// YAK_JS_PROJECT_FILE picks a single file (relative to the
// project root), YAK_JS_PROJECT_LIMIT caps the total count.
func selectJSMetricFiles(t *testing.T, fileList []string) []string {
	t.Helper()

	if singleFile := strings.TrimSpace(os.Getenv("YAK_JS_PROJECT_FILE")); singleFile != "" {
		singleFile = filepath.ToSlash(singleFile)
		for _, path := range fileList {
			if filepath.ToSlash(path) == singleFile {
				return []string{path}
			}
		}
		t.Fatalf("YAK_JS_PROJECT_FILE not found in project: %s", singleFile)
	}

	limit := 0
	if raw := strings.TrimSpace(os.Getenv("YAK_JS_PROJECT_LIMIT")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			t.Fatalf("invalid YAK_JS_PROJECT_LIMIT: %s", raw)
		}
		limit = value
	}
	if limit > 0 && len(fileList) > limit {
		return fileList[:limit]
	}
	return fileList
}

func jsLocalParseBudget() time.Duration {
	raw := strings.TrimSpace(os.Getenv("YAK_JS_PARSE_BUDGET_SEC"))
	if raw == "" {
		return 30 * time.Second
	}
	sec, err := strconv.Atoi(raw)
	if err != nil || sec <= 0 {
		return 0
	}
	return time.Duration(sec) * time.Second
}

func jsLocalASTResetEveryFiles() int {
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

type jsAntlrCacheStats struct {
	LexerDFAStates  int
	ParserDFAStates int
	LexerPredCtx    int
	ParserPredCtx   int
}

// readJSAntlrCacheStats reads the ANTLR runtime cache sizes for
// per-file growth metrics. It uses reflection over unexported fields because
// the antlr fork does not expose them.
func readJSAntlrCacheStats(cache *ssa.AntlrCache) jsAntlrCacheStats {
	if cache == nil {
		return jsAntlrCacheStats{}
	}

	stats := jsAntlrCacheStats{}
	for _, dfa := range cache.LexerDfaCache {
		stats.LexerDFAStates += countJSMetricDFAStates(dfa)
	}
	for _, dfa := range cache.ParserDfaCache {
		stats.ParserDFAStates += countJSMetricDFAStates(dfa)
	}
	stats.LexerPredCtx = countJSMetricPredictionContexts(cache.LexerPredictionContextCache)
	stats.ParserPredCtx = countJSMetricPredictionContexts(cache.ParserPredictionContextCache)
	return stats
}

func countJSMetricPredictionContexts(cache *antlr.PredictionContextCache) int {
	if cache == nil {
		return 0
	}
	value := reflect.ValueOf(cache)
	if value.Kind() != reflect.Ptr || value.IsNil() {
		return 0
	}
	value = value.Elem()
	field := value.FieldByName("cache")
	if !field.IsValid() || field.Kind() != reflect.Map {
		return 0
	}
	return field.Len()
}

func countJSMetricDFAStates(dfa *antlr.DFA) int {
	if dfa == nil {
		return 0
	}
	value := reflect.ValueOf(dfa)
	if value.Kind() != reflect.Ptr || value.IsNil() {
		return 0
	}
	value = value.Elem()
	field := value.FieldByName("states")
	if !field.IsValid() || field.Kind() != reflect.Ptr || field.IsNil() {
		return 0
	}
	field = field.Elem()
	lengthField := field.FieldByName("len")
	if !lengthField.IsValid() || lengthField.Kind() != reflect.Int {
		return 0
	}
	return int(lengthField.Int())
}

func formatJSMetricBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return strconv.FormatUint(bytes, 10) + "B"
	}

	div := uint64(unit)
	suffixes := []string{"KB", "MB", "GB", "TB", "PB"}
	index := 0
	for value := bytes / unit; value >= unit && index < len(suffixes)-1; value /= unit {
		div *= unit
		index++
	}
	return strconv.FormatFloat(float64(bytes)/float64(div), 'f', 2, 64) + suffixes[index]
}

func formatJSMetricBytesDelta(delta int64) string {
	if delta == 0 {
		return "0B"
	}
	sign := ""
	if delta > 0 {
		sign = "+"
	} else {
		delta = -delta
		sign = "-"
	}
	return sign + formatJSMetricBytes(uint64(delta))
}

// jsLogProjectErrors records, without failing the test, the SSA errors
// each program reported. Real projects legitimately produce semantic SSA
// errors (missing cross-file types, closure capture hints) - the goal of
// these local runs is honest data, not a zero-error guarantee.
func jsLogProjectErrors(phase string, progs []*ssaapi.Program) {
	total := 0
	files := 0
	for _, prog := range progs {
		errs := prog.GetErrors()
		if len(errs) == 0 {
			continue
		}
		total += len(errs)
		files++
		log.Infof("[js-local-project] %s: %d SSA errors:\n%s", phase, len(errs), formatJSProjectErrorsByFile(errs))
	}
	if total > 0 {
		log.Infof("[js-local-project] %s summary: %d SSA errors across %d/%d programs (recorded, not asserted)", phase, total, files, len(progs))
	}
}

func formatJSProjectErrorsByFile(errs ssa.SSAErrors) string {
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
