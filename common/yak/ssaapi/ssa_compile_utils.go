package ssaapi

import (
	"errors"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
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
			log.Infof("pre-handler cost:%v parse ast: %s", time.Since(start), path)
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
		if !ok {
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
				return language.ParseAST(utils.UnsafeBytesToString(content), cache)
			} else {
				log.Debugf("skip parse ast file: %s", path)
				return nil, nil
			}
		}
		return nil, utils.Errorf("not select language %s", c.language)
	}
	initWorker := func() *utils.SafeMap[any] {
		ret := utils.NewSafeMap[any]()
		ret.Set(key, c.LanguageBuilder.GetAntlrCache())
		log.Errorf("get antlr cache from store failed, new one, goroutine id: %d", getGID())
		return ret
	}
	return ssareducer.FilesHandler(
		c.ctx, filesystem, preHandlerFiles,
		parse, initWorker,
		c.astSequence,
		c.concurrency,
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
