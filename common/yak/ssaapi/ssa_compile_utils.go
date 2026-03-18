package ssaapi

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
)

var (
	ErrContextCancel       error = errors.New("context cancel")
	ErrNoFoundCompiledFile error = errors.New("not found can compiled file")
)

const (
	key                 = "antlr_cache"
	antlrWorkerStatsKey = "antlr_worker_stats"
	envSSAReparseAST    = "YAK_SSA_REPARSE_AST_FOR_BUILD"
)

type antlrWorkerStats struct {
	filesParsed int
}

type antlrCacheResetConfig struct {
	enabled         bool
	resetEveryFiles int
}

var (
	ssaReparseASTOnce   sync.Once
	ssaReparseASTCached bool
)

func getAntlrCacheResetConfig() antlrCacheResetConfig {
	cfg := antlrCacheResetConfig{
		enabled:         true,
		resetEveryFiles: 100,
	}
	if raw := strings.TrimSpace(os.Getenv("YAK_ANTLR_CACHE_RESET_FILES")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			if v <= 0 {
				cfg.enabled = false
				cfg.resetEveryFiles = 0
			} else {
				cfg.resetEveryFiles = v
			}
		}
	}
	if cfg.resetEveryFiles <= 0 {
		cfg.enabled = false
	}
	return cfg
}

func getSSAReparseASTForBuildEnabled() bool {
	ssaReparseASTOnce.Do(func() {
		raw := strings.TrimSpace(os.Getenv(envSSAReparseAST))
		switch strings.ToLower(raw) {
		case "1", "true", "yes", "on":
			ssaReparseASTCached = true
		default:
			ssaReparseASTCached = false
		}
	})
	return ssaReparseASTCached
}

func (c *Config) GetFileHandler(
	filesystem filesys_interface.FileSystem,
	preHandlerFiles []string,
	handlerFilesMap map[string]struct{},
) <-chan *ssareducer.FileContent {
	resetCfg := getAntlrCacheResetConfig()
	parse := func(path string, content []byte, store *utils.SafeMap[any]) (ssa.FrontAST, error) {
		start := time.Now()
		defer func() {
			log.Debugf("pre-handler cost:%v parse ast: %s; size(%v)", time.Since(start), path, Size(len(content)))
		}()

		defer func() {
			if r := recover(); r != nil {
				log.Errorf("pre-handler parse [%s] error %v  ", path, r)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()
		if _, needBuild := handlerFilesMap[path]; !needBuild {
			// don't need parse ast
			return nil, nil
		}

		var cache *ssa.AntlrCache
		if ssa.WorkerAntlrCacheEnabled() {
			raw, ok := store.Get(key)
			if ok {
				if raw, ok := raw.(*ssa.AntlrCache); ok && raw != nil {
					cache = raw
				}
			}
			if cache == nil {
				cache = c.LanguageBuilder.GetAntlrCache()
				if cache != nil {
					store.Set(key, cache)
				}
			}
		}

		if language := c.LanguageBuilder; language != nil {
			if language.FilterParseAST(path) {
				metricsEnabled := astParseFileMetricsEnabled()
				var memBefore astParseMemSnapshot
				var cacheBefore antlrRuntimeCacheStats
				if metricsEnabled {
					memBefore = readASTParseMemSnapshot()
					cacheBefore = readAntlrRuntimeCacheStats(cache)
				}

				parseStart := time.Now()
				ast, err := language.ParseAST(utils.UnsafeBytesToString(content), cache)
				parseDur := time.Since(parseStart)

				if metricsEnabled {
					memAfter := readASTParseMemSnapshot()
					cacheAfter := readAntlrRuntimeCacheStats(cache)
					sizeBytes := uint64(len(content))
					heapAllocDelta := int64(memAfter.HeapAlloc) - int64(memBefore.HeapAlloc)
					heapInuseDelta := int64(memAfter.HeapInuse) - int64(memBefore.HeapInuse)
					totalAllocDelta := memAfter.TotalAlloc - memBefore.TotalAlloc

					log.Infof(
						"AST_METRIC\tworker=%d\tfile=%s\tsize=%d\tsize_h=%s\tparse_ms=%d\tparse_h=%s"+
							"\theap_alloc_b=%d\theap_alloc_b_h=%s\theap_alloc_a=%d\theap_alloc_a_h=%s\theap_alloc_d=%s\theap_alloc_d_h=%s"+
							"\theap_inuse_b=%d\theap_inuse_b_h=%s\theap_inuse_a=%d\theap_inuse_a_h=%s\theap_inuse_d=%s\theap_inuse_d_h=%s"+
							"\theap_obj_b=%d\theap_obj_a=%d\theap_obj_d=%s"+
							"\ttotal_alloc_b=%d\ttotal_alloc_b_h=%s\ttotal_alloc_a=%d\ttotal_alloc_a_h=%s\ttotal_alloc_d=%d\ttotal_alloc_d_h=%s"+
							"\tmallocs_b=%d\tmallocs_a=%d\tmallocs_d=%d"+
							"\tfrees_b=%d\tfrees_a=%d\tfrees_d=%d"+
							"\tnum_gc_b=%d\tnum_gc_a=%d\tnum_gc_d=%d"+
							"\tcache_lexer_dfa_states_b=%d\tcache_lexer_dfa_states_a=%d\tcache_lexer_dfa_states_d=%d"+
							"\tcache_parser_dfa_states_b=%d\tcache_parser_dfa_states_a=%d\tcache_parser_dfa_states_d=%d"+
							"\tcache_lexer_pred_ctx_b=%d\tcache_lexer_pred_ctx_a=%d\tcache_lexer_pred_ctx_d=%d"+
							"\tcache_parser_pred_ctx_b=%d\tcache_parser_pred_ctx_a=%d\tcache_parser_pred_ctx_d=%d"+
							"\tcache_lexer_dfa_cnt=%d\tcache_parser_dfa_cnt=%d",
						getGID(),
						path,
						len(content),
						formatBytesHuman(sizeBytes),
						parseDur.Milliseconds(),
						parseDur.String(),
						memBefore.HeapAlloc, formatBytesHuman(memBefore.HeapAlloc), memAfter.HeapAlloc, formatBytesHuman(memAfter.HeapAlloc), formatSignedDelta(heapAllocDelta), formatBytesDeltaHuman(heapAllocDelta),
						memBefore.HeapInuse, formatBytesHuman(memBefore.HeapInuse), memAfter.HeapInuse, formatBytesHuman(memAfter.HeapInuse), formatSignedDelta(heapInuseDelta), formatBytesDeltaHuman(heapInuseDelta),
						memBefore.HeapObjects, memAfter.HeapObjects, formatSignedDelta(int64(memAfter.HeapObjects)-int64(memBefore.HeapObjects)),
						memBefore.TotalAlloc, formatBytesHuman(memBefore.TotalAlloc), memAfter.TotalAlloc, formatBytesHuman(memAfter.TotalAlloc), totalAllocDelta, formatBytesHuman(totalAllocDelta),
						memBefore.Mallocs, memAfter.Mallocs, memAfter.Mallocs-memBefore.Mallocs,
						memBefore.Frees, memAfter.Frees, memAfter.Frees-memBefore.Frees,
						memBefore.NumGC, memAfter.NumGC, uint64(memAfter.NumGC-memBefore.NumGC),
						cacheBefore.LexerDFAStates, cacheAfter.LexerDFAStates, cacheAfter.LexerDFAStates-cacheBefore.LexerDFAStates,
						cacheBefore.ParserDFAStates, cacheAfter.ParserDFAStates, cacheAfter.ParserDFAStates-cacheBefore.ParserDFAStates,
						cacheBefore.LexerPredCtx, cacheAfter.LexerPredCtx, cacheAfter.LexerPredCtx-cacheBefore.LexerPredCtx,
						cacheBefore.ParserPredCtx, cacheAfter.ParserPredCtx, cacheAfter.ParserPredCtx-cacheBefore.ParserPredCtx,
						cacheAfter.LexerDFACnt, cacheAfter.ParserDFACnt,
					)
				}

				if err != nil {
					log.Debugf("parsed file[%s] parse [%s]AST error[%s]", path, language.GetLanguage(), err)
				}
				if resetCfg.enabled && cache != nil {
					if raw, ok := store.Get(antlrWorkerStatsKey); ok && raw != nil {
						if stats, _ := raw.(*antlrWorkerStats); stats != nil {
							stats.filesParsed++
							if stats.filesParsed%resetCfg.resetEveryFiles == 0 {
								cache.ResetRuntimeCaches()
							}
						}
					}
				}
				return ast, err
			} else {
				log.Debugf("skip parse ast file: %s filter by %s", path, language.GetLanguage())
				return nil, nil
			}
		}
		return nil, utils.Errorf("not select language %s", c.GetLanguage())
	}
	initWorker := func() *utils.SafeMap[any] {
		ret := utils.NewSafeMap[any]()
		ret.Set(antlrWorkerStatsKey, &antlrWorkerStats{})
		if ssa.WorkerAntlrCacheEnabled() {
			cache := c.LanguageBuilder.GetAntlrCache()
			if cache != nil {
				ret.Set(key, cache)
			}
		}
		return ret
	}
	return ssareducer.FilesHandler(
		c.ctx, filesystem, preHandlerFiles,
		parse, initWorker,
		c.astSequence,
		int(c.GetCompileConcurrency()),
	)
}
func getGID() uint64 {
	var buf [64]byte
	// false=不获取全堆栈，仅当前G的ID
	n := runtime.Stack(buf[:], false)
	// 堆栈开头格式: "goroutine 123 [running]:"
	idStr := strings.Fields(strings.TrimPrefix(string(buf[:n]), "goroutine "))[0]
	id, _ := strconv.ParseUint(idStr, 10, 64)
	return id
}

func Size(size int) string {
	if size < 1024 {
		return strconv.Itoa(size) + "B"
	}
	sizeKB := float64(size) / 1024.0
	if sizeKB < 1024 {
		return strconv.FormatFloat(sizeKB, 'f', 2, 64) + "KB"
	}
	sizeMB := sizeKB / 1024.0
	if sizeMB < 1024 {
		return strconv.FormatFloat(sizeMB, 'f', 2, 64) + "MB"
	}
	sizeGB := sizeMB / 1024.0
	return strconv.FormatFloat(sizeGB, 'f', 2, 64) + "GB"
}

type ScanResult struct {
	HandlerFiles    []string
	PreHandlerFiles []string
	HandlerFilesMap map[string]struct{}
	Folders         [][]string
	HandlerTotal    int
	PreHandlerTotal int
}

type ScanConfig struct {
	ProgramName     string
	ProgramPath     string
	FileSystem      filesys_interface.FileSystem
	ExcludeFunc     func(string) bool
	CheckLanguage   func(string) error
	CheckPreHandler func(string) error
	Context         context.Context
}

// ScanProjectFiles scans the project directory and returns the files to be processed
func ScanProjectFiles(cfg ScanConfig) (*ScanResult, error) {
	result := &ScanResult{
		HandlerFiles:    make([]string, 0),
		PreHandlerFiles: make([]string, 0),
		HandlerFilesMap: make(map[string]struct{}),
		Folders:         make([][]string, 0),
	}

	err := filesys.Recursive(cfg.ProgramPath,
		filesys.WithFileSystem(cfg.FileSystem),
		filesys.WithContext(cfg.Context),
		filesys.WithDirStat(func(fullPath string, fi fs.FileInfo) error {
			// check folder folderName
			_, folderName := cfg.FileSystem.PathSplit(fullPath)
			if folderName == "test" || folderName == ".git" {
				return filesys.SkipDir
			}
			if cfg.ExcludeFunc != nil && cfg.ExcludeFunc(fullPath) {
				return filesys.SkipDir
			}

			folders := []string{cfg.ProgramName}
			// Use the filesystem's separator to split the path
			// Note: In the original code, this used c.fs.GetSeparators().
			// We should use cfg.FileSystem.GetSeparators() if it matches, or pass it in.
			// Assuming cfg.FileSystem is the one to use.
			sep := string(cfg.FileSystem.GetSeparators())
			folders = append(folders,
				strings.Split(fullPath, sep)...,
			)
			result.Folders = append(result.Folders, folders)
			return nil
		}),
		filesys.WithFileStat(func(path string, fi fs.FileInfo) error {
			if fi.Size() == 0 {
				return nil
			}
			if cfg.ExcludeFunc != nil && cfg.ExcludeFunc(path) {
				return nil
			}
			if cfg.CheckLanguage != nil && cfg.CheckLanguage(path) == nil {
				result.HandlerTotal++
				result.HandlerFiles = append(result.HandlerFiles, path)
			}
			if cfg.CheckPreHandler != nil && cfg.CheckPreHandler(path) == nil {
				result.PreHandlerTotal++
				result.PreHandlerFiles = append(result.PreHandlerFiles, path)
				result.HandlerFilesMap[path] = struct{}{}
			}
			return nil
		}),
	)

	return result, err
}
