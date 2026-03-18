package tests

import (
	"io/fs"
	"net/http"
	_ "net/http/pprof"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssalog"
)

func TestASTParseFileMetrics_DecompiledCodeTarget(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("local-only profiling test")
	}
	if os.Getenv("YAK_RUN_AST_PARSE_FILE_METRICS_TEST") == "" {
		t.Skip("set YAK_RUN_AST_PARSE_FILE_METRICS_TEST=1 to run local AST parse profiling")
	}

	antlr4util.ResetSLLFirstCounters()
	ssalog.Log.SetLevel("info")

	if os.Getenv("YAK_START_PPROF") != "" {
		go func() {
			_ = http.ListenAndServe(":18080", nil)
		}()
	}

	path := os.Getenv("YAK_AST_PARSE_FILE_METRICS_TARGET")
	if path == "" {
		path = "/tmp/decompiled-code-target"
	}
	refFS := filesys.NewRelLocalFs(path)
	if _, err := refFS.Stat("."); err != nil {
		t.Skipf("target path not found: %s (%v)", path, err)
	}

	fileList := collectASTMetricFiles(t, refFS)
	require.NotEmpty(t, fileList)

	builder, ok := java2ssa.CreateBuilder().(*java2ssa.SSABuilder)
	require.True(t, ok)
	defer builder.Clearup()

	cache := builder.GetAntlrCache()
	resetEveryFiles := astMetricResetEveryFiles()

	for index, path := range fileList {
		content, err := refFS.ReadFile(path)
		require.NoError(t, err)

		memBefore := readASTMetricMemSnapshot()
		cacheBefore := readASTMetricCacheStats(cache)
		start := time.Now()
		_, err = builder.ParseAST(utils.UnsafeBytesToString(content), cache)
		parseDur := time.Since(start)
		memAfter := readASTMetricMemSnapshot()
		cacheAfter := readASTMetricCacheStats(cache)

		log.Infof(
			"AST_METRIC\tindex=%d/%d\tfile=%s\tsize=%s\tparse=%s\theap_alloc=%s->%s (%s)\theap_inuse=%s->%s (%s)\tcache_lexer_dfa=%d->%d (%+d)\tcache_parser_dfa=%d->%d (%+d)\tcache_lexer_ctx=%d->%d (%+d)\tcache_parser_ctx=%d->%d (%+d)\terr=%v",
			index+1,
			len(fileList),
			path,
			formatASTMetricBytes(uint64(len(content))),
			parseDur,
			formatASTMetricBytes(memBefore.HeapAlloc),
			formatASTMetricBytes(memAfter.HeapAlloc),
			formatASTMetricBytesDelta(int64(memAfter.HeapAlloc)-int64(memBefore.HeapAlloc)),
			formatASTMetricBytes(memBefore.HeapInuse),
			formatASTMetricBytes(memAfter.HeapInuse),
			formatASTMetricBytesDelta(int64(memAfter.HeapInuse)-int64(memBefore.HeapInuse)),
			cacheBefore.LexerDFAStates, cacheAfter.LexerDFAStates, cacheAfter.LexerDFAStates-cacheBefore.LexerDFAStates,
			cacheBefore.ParserDFAStates, cacheAfter.ParserDFAStates, cacheAfter.ParserDFAStates-cacheBefore.ParserDFAStates,
			cacheBefore.LexerPredCtx, cacheAfter.LexerPredCtx, cacheAfter.LexerPredCtx-cacheBefore.LexerPredCtx,
			cacheBefore.ParserPredCtx, cacheAfter.ParserPredCtx, cacheAfter.ParserPredCtx-cacheBefore.ParserPredCtx,
			err,
		)

		if err == nil && cache != nil && resetEveryFiles > 0 && (index+1)%resetEveryFiles == 0 {
			cache.ResetRuntimeCaches()
		}
	}

	stats := antlr4util.SLLFirstCountersSnapshot()
	log.Debugf(
		"[antlr-sll-first] ll_only=%d sll_attempts=%d fallbacks=%d cancelled=%d error=%d",
		stats.LLOnly, stats.SLLAttempts, stats.Fallbacks, stats.FallbackCancelled, stats.FallbackError,
	)
}

func collectASTMetricFiles(t *testing.T, refFS *filesys.RelLocalFs) []string {
	t.Helper()

	limit := 0
	if raw := os.Getenv("YAK_AST_PARSE_FILE_METRICS_LIMIT"); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			limit = value
		}
	}

	if singleFile := os.Getenv("YAK_AST_PARSE_FILE_METRICS_FILE"); singleFile != "" {
		if _, err := refFS.Stat(singleFile); err != nil {
			t.Fatalf("metrics file not found: %s (%v)", singleFile, err)
		}
		return []string{singleFile}
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

	if limit > 0 && len(fileList) > limit {
		return fileList[:limit]
	}
	return fileList
}

func astMetricResetEveryFiles() int {
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

type astMetricMemSnapshot struct {
	HeapAlloc uint64
	HeapInuse uint64
}

func readASTMetricMemSnapshot() astMetricMemSnapshot {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return astMetricMemSnapshot{
		HeapAlloc: stats.HeapAlloc,
		HeapInuse: stats.HeapInuse,
	}
}

type astMetricCacheStats struct {
	LexerDFAStates  int
	ParserDFAStates int
	LexerPredCtx    int
	ParserPredCtx   int
}

func readASTMetricCacheStats(cache *ssa.AntlrCache) astMetricCacheStats {
	if cache == nil {
		return astMetricCacheStats{}
	}

	stats := astMetricCacheStats{}
	for _, dfa := range cache.LexerDfaCache {
		stats.LexerDFAStates += countASTMetricDFAStates(dfa)
	}
	for _, dfa := range cache.ParserDfaCache {
		stats.ParserDFAStates += countASTMetricDFAStates(dfa)
	}
	stats.LexerPredCtx = countASTMetricPredictionContexts(cache.LexerPredictionContextCache)
	stats.ParserPredCtx = countASTMetricPredictionContexts(cache.ParserPredictionContextCache)
	return stats
}

func countASTMetricPredictionContexts(cache *antlr.PredictionContextCache) int {
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

func countASTMetricDFAStates(dfa *antlr.DFA) int {
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

func formatASTMetricBytes(bytes uint64) string {
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

func formatASTMetricBytesDelta(delta int64) string {
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
	return sign + formatASTMetricBytes(uint64(delta))
}
