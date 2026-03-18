package ssaapi

import (
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

const envASTParseFileMetrics = "YAK_AST_PARSE_FILE_METRICS"

func astParseFileMetricsEnabled() bool {
	raw := strings.TrimSpace(os.Getenv(envASTParseFileMetrics))
	if raw == "" {
		return false
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "y", "on", "enable", "enabled":
		return true
	default:
		return false
	}
}

type astParseMemSnapshot struct {
	HeapAlloc   uint64
	HeapInuse   uint64
	HeapObjects uint64
	TotalAlloc  uint64
	Mallocs     uint64
	Frees       uint64
	NumGC       uint32
}

func readASTParseMemSnapshot() astParseMemSnapshot {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return astParseMemSnapshot{
		HeapAlloc:   ms.HeapAlloc,
		HeapInuse:   ms.HeapInuse,
		HeapObjects: ms.HeapObjects,
		TotalAlloc:  ms.TotalAlloc,
		Mallocs:     ms.Mallocs,
		Frees:       ms.Frees,
		NumGC:       ms.NumGC,
	}
}

type antlrRuntimeCacheStats struct {
	LexerDFAStates  int
	ParserDFAStates int
	LexerPredCtx    int
	ParserPredCtx   int
	LexerDFACnt     int
	ParserDFACnt    int
}

func readAntlrRuntimeCacheStats(cache *ssa.AntlrCache) antlrRuntimeCacheStats {
	if cache == nil {
		return antlrRuntimeCacheStats{}
	}
	stats := antlrRuntimeCacheStats{
		LexerDFACnt:  len(cache.LexerDfaCache),
		ParserDFACnt: len(cache.ParserDfaCache),
	}
	for _, dfa := range cache.LexerDfaCache {
		stats.LexerDFAStates += antlrDFAStatesCount(dfa)
	}
	for _, dfa := range cache.ParserDfaCache {
		stats.ParserDFAStates += antlrDFAStatesCount(dfa)
	}
	stats.LexerPredCtx = antlrPredictionContextCacheLen(cache.LexerPredictionContextCache)
	stats.ParserPredCtx = antlrPredictionContextCacheLen(cache.ParserPredictionContextCache)
	return stats
}

func antlrPredictionContextCacheLen(cache *antlr.PredictionContextCache) int {
	if cache == nil {
		return 0
	}
	v := reflect.ValueOf(cache)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return 0
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return 0
	}
	f := v.FieldByName("cache")
	if !f.IsValid() || f.Kind() != reflect.Map {
		return 0
	}
	return f.Len()
}

func antlrDFAStatesCount(dfa *antlr.DFA) int {
	if dfa == nil {
		return 0
	}
	v := reflect.ValueOf(dfa)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return 0
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return 0
	}
	// Prefer JStore.len for accuracy (dfa.numstates is not maintained in this runtime version).
	states := v.FieldByName("states")
	if states.IsValid() && states.Kind() == reflect.Ptr && !states.IsNil() {
		sv := states.Elem()
		if sv.IsValid() && sv.Kind() == reflect.Struct {
			lf := sv.FieldByName("len")
			if lf.IsValid() && lf.Kind() == reflect.Int {
				return int(lf.Int())
			}
		}
	}
	return 0
}

func formatSignedDelta(delta int64) string {
	if delta >= 0 {
		return "+" + strconv.FormatInt(delta, 10)
	}
	return strconv.FormatInt(delta, 10)
}

func formatBytesHuman(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}

	div := uint64(unit)
	exp := 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	suffixes := []string{"KB", "MB", "GB", "TB", "PB", "EB"}
	if exp >= len(suffixes) {
		exp = len(suffixes) - 1
	}
	return fmt.Sprintf("%.2f%s", float64(bytes)/float64(div), suffixes[exp])
}

func formatBytesDeltaHuman(delta int64) string {
	if delta == 0 {
		return "0B"
	}
	sign := ""
	abs := delta
	if abs < 0 {
		abs = -abs
	} else {
		sign = "+"
	}
	return sign + formatBytesHuman(uint64(abs))
}
