package ssaapi

import (
	"errors"
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
		return ret
	}
	return ssareducer.FilesHandler(
		c.ctx, filesystem, preHandlerFiles,
		parse, initWorker,
		c.astSequence,
		c.concurrency,
	)
}
