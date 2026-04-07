package ssaapi

import (
	"context"
	"errors"
	"io/fs"
	"os"
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
	antlrWorkerStateKey = "antlr_worker_state"
)

var (
	antlrCacheResetEveryFilesOnce   sync.Once
	antlrCacheResetEveryFilesCached int
)

func antlrCacheResetEveryFiles() int {
	antlrCacheResetEveryFilesOnce.Do(func() {
		antlrCacheResetEveryFilesCached = 100
		if raw := strings.TrimSpace(os.Getenv("YAK_ANTLR_CACHE_RESET_FILES")); raw != "" {
			if v, err := strconv.Atoi(raw); err == nil {
				antlrCacheResetEveryFilesCached = v
			}
		}
		if antlrCacheResetEveryFilesCached <= 0 {
			antlrCacheResetEveryFilesCached = 0
		}
	})
	return antlrCacheResetEveryFilesCached
}

type antlrWorkerState struct {
	cache       *ssa.AntlrCache
	filesParsed int
}

type antlrASTParseWorker struct {
	language        ssa.PreHandlerAnalyzer
	languageName    string
	resetEveryFiles int
}

func newAntlrASTParseWorker(c *Config) *antlrASTParseWorker {
	if c == nil {
		return &antlrASTParseWorker{}
	}
	return &antlrASTParseWorker{
		language:        c.LanguageBuilder,
		languageName:    string(c.GetLanguage()),
		resetEveryFiles: antlrCacheResetEveryFiles(),
	}
}

func (p *antlrASTParseWorker) initWorker() *utils.SafeMap[any] {
	store := utils.NewSafeMap[any]()
	store.Set(antlrWorkerStateKey, &antlrWorkerState{
		cache: p.newWorkerCache(),
	})
	return store
}

func (p *antlrASTParseWorker) newWorkerCache() *ssa.AntlrCache {
	if p == nil || p.language == nil || !ssa.WorkerAntlrCacheEnabled() {
		return nil
	}
	return p.language.GetAntlrCache()
}

func (p *antlrASTParseWorker) workerState(store *utils.SafeMap[any]) *antlrWorkerState {
	if store == nil {
		return &antlrWorkerState{
			cache: p.newWorkerCache(),
		}
	}

	if raw, ok := store.Get(antlrWorkerStateKey); ok {
		if state, ok := raw.(*antlrWorkerState); ok && state != nil {
			return state
		}
	}

	state := &antlrWorkerState{
		cache: p.newWorkerCache(),
	}
	store.Set(antlrWorkerStateKey, state)
	return state
}

func (p *antlrASTParseWorker) parseFileAST(path string, source string, store *utils.SafeMap[any]) (ssa.FrontAST, error) {
	if p == nil || p.language == nil {
		return nil, utils.Errorf("not select language %s", p.languageName)
	}
	if !p.language.FilterParseAST(path) {
		log.Debugf("skip parse ast file: %s filter by %s", path, p.languageName)
		return nil, nil
	}

	state := p.workerState(store)
	ast, err := p.language.ParseAST(source, state.cache)
	if state.cache == nil || p.resetEveryFiles <= 0 {
		return ast, err
	}
	state.filesParsed++
	if state.filesParsed%p.resetEveryFiles == 0 {
		state.cache.ResetRuntimeCaches()
	}
	if err != nil {
		log.Debugf("parsed file[%s] parse [%s]AST error[%s]", path, p.languageName, err)
	}
	return ast, err
}

func (c *Config) GetFileHandler(
	filesystem filesys_interface.FileSystem,
	preHandlerFiles []string,
	handlerFilesMap map[string]struct{},
) <-chan *ssareducer.FileContent {
	parser := newAntlrASTParseWorker(c)
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

		return parser.parseFileAST(path, utils.UnsafeBytesToString(content), store)
	}
	initWorker := func() *utils.SafeMap[any] {
		return parser.initWorker()
	}
	return ssareducer.FilesHandler(
		c.ctx, filesystem, preHandlerFiles,
		parse, initWorker,
		c.GetCompileASTSequence(),
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

// Size formats bytes into a human-readable string.
// Kept for backward compatibility across packages/tests.
func Size(size int) string {
	return formatFileSize(size)
}

type ScanResult struct {
	HandlerFiles    []string
	PreHandlerFiles []string
	HandlerFilesMap map[string]struct{}
	Folders         [][]string
	HandlerTotal    int
	PreHandlerTotal int
	// HandlerBytes is the total source byte size of files that enter the compile
	// stage. It is used to choose adaptive IR cache defaults for small vs large
	// projects; it is not persisted as part of user-facing project metadata.
	HandlerBytes int64
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
				result.HandlerBytes += fi.Size()
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
