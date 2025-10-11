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

func (c *config) getFileHandler(
	filesystem filesys_interface.FileSystem,
	preHandlerFiles []string,
	handlerFilesMap map[string]struct{},
	concurrency int,
) <-chan *ssareducer.FileContent {
	return ssareducer.FilesHandler(
		c.ctx, filesystem, preHandlerFiles,
		func(path string, content []byte) (ssa.FrontAST, error) {
			start := time.Now()
			defer func() {
				c.Processf(0, "pre-handler parse ast: %s, cost: %v", path, time.Since(start))
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

			if language := c.LanguageBuilder; language != nil {
				if language.FilterParseAST(path) {
					return language.ParseAST(utils.UnsafeBytesToString(content))
				} else {
					log.Debugf("skip parse ast file: %s", path)
					return nil, nil
				}
			}
			return nil, utils.Errorf("not select language %s", c.language)
		},
		int(c.astSequence),
		concurrency,
	)
}
