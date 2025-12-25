package ssaapi

import (
	"context"
	"errors"
	"io/fs"
	"runtime"
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

const key = "antlr_cache"

func (c *Config) GetFileHandler(
	filesystem filesys_interface.FileSystem,
	preHandlerFiles []string,
	handlerFilesMap map[string]struct{},
) <-chan *ssareducer.FileContent {
	parse := func(path string, content []byte, store *utils.SafeMap[any]) (ssa.FrontAST, error) {
		start := time.Now()
		defer func() {
			log.Infof("pre-handler cost:%v parse ast: %s; size(%v)", time.Since(start), path, Size(len(content)))
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
		raw, ok := store.Get(key)
		if ok {
			if raw, ok := raw.(*ssa.AntlrCache); ok && raw != nil {
				cache = raw
			}
		}
		if cache == nil {
			cache = c.LanguageBuilder.GetAntlrCache()
			log.Errorf("get antlr cache from store failed, new one, path: %s, goroutine id: %d", path, getGID())
			store.Set(key, cache)
		}

		if language := c.LanguageBuilder; language != nil {
			if language.FilterParseAST(path) {
				ast, err := language.ParseAST(utils.UnsafeBytesToString(content), cache)
				if err != nil {
					log.Infof("parsed file[%s] parse [%s]AST error[%s]", path, language.GetLanguage(), err)
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
		ret.Set(key, c.LanguageBuilder.GetAntlrCache())
		log.Debugf("create antrl cache, goroutine id: %d", getGID())
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
