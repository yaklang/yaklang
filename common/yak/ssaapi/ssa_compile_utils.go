package ssaapi

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"
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
	antlrWorkerStateKey = "antlr_worker_state"
)

type antlrCacheResetConfig struct {
	enabled         bool
	resetEveryFiles int
}

type antlrWorkerState struct {
	cache           *ssa.AntlrCache
	filesParsed     int
	resetEveryFiles int
}

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

func newAntlrWorkerStore(language ssa.PreHandlerAnalyzer, resetCfg antlrCacheResetConfig) *utils.SafeMap[any] {
	ret := utils.NewSafeMap[any]()
	ret.Set(antlrWorkerStateKey, &antlrWorkerState{
		cache:           newAntlrWorkerCache(language),
		resetEveryFiles: resetCfg.resetEveryFiles,
	})
	return ret
}

func newAntlrWorkerCache(language ssa.PreHandlerAnalyzer) *ssa.AntlrCache {
	if language == nil || !ssa.WorkerAntlrCacheEnabled() {
		return nil
	}
	return language.GetAntlrCache()
}

func getAntlrWorkerState(
	store *utils.SafeMap[any],
	language ssa.PreHandlerAnalyzer,
	resetCfg antlrCacheResetConfig,
) *antlrWorkerState {
	if store == nil {
		return &antlrWorkerState{
			cache:           newAntlrWorkerCache(language),
			resetEveryFiles: resetCfg.resetEveryFiles,
		}
	}

	if raw, ok := store.Get(antlrWorkerStateKey); ok {
		if state, ok := raw.(*antlrWorkerState); ok && state != nil {
			return state
		}
	}

	state := &antlrWorkerState{
		cache:           newAntlrWorkerCache(language),
		resetEveryFiles: resetCfg.resetEveryFiles,
	}
	store.Set(antlrWorkerStateKey, state)
	return state
}

func (s *antlrWorkerState) Parse(language ssa.PreHandlerAnalyzer, source string) (ssa.FrontAST, error) {
	ast, err := language.ParseAST(source, s.cache)
	if s == nil || s.cache == nil || s.resetEveryFiles <= 0 {
		return ast, err
	}

	s.filesParsed++
	if s.filesParsed%s.resetEveryFiles == 0 {
		s.cache.ResetRuntimeCaches()
	}
	return ast, err
}

func parseFileAST(
	language ssa.PreHandlerAnalyzer,
	languageName string,
	path string,
	source string,
	store *utils.SafeMap[any],
	resetCfg antlrCacheResetConfig,
) (ssa.FrontAST, error) {
	if language == nil {
		return nil, utils.Errorf("not select language %s", languageName)
	}
	if !language.FilterParseAST(path) {
		log.Debugf("skip parse ast file: %s filter by %s", path, languageName)
		return nil, nil
	}

	state := getAntlrWorkerState(store, language, resetCfg)
	ast, err := state.Parse(language, source)
	if err != nil {
		log.Debugf("parsed file[%s] parse [%s]AST error[%s]", path, languageName, err)
	}
	return ast, err
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
			log.Debugf("pre-handler cost:%v parse ast: %s; size(%s)", time.Since(start), path, formatFileSize(len(content)))
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

		return parseFileAST(
			c.LanguageBuilder,
			string(c.GetLanguage()),
			path,
			utils.UnsafeBytesToString(content),
			store,
			resetCfg,
		)
	}
	initWorker := func() *utils.SafeMap[any] {
		return newAntlrWorkerStore(c.LanguageBuilder, resetCfg)
	}
	return ssareducer.FilesHandler(
		c.ctx, filesystem, preHandlerFiles,
		parse, initWorker,
		c.astSequence,
		int(c.GetCompileConcurrency()),
	)
}

func formatFileSize(size int) string {
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
