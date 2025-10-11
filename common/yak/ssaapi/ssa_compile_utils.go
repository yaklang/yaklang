package ssaapi

import (
	"errors"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssaprofile"
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
) <-chan *ssareducer.FileContent {
	parse := func(path string, content []byte) (ssa.FrontAST, error) {
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

		if language := c.LanguageBuilder; language != nil {
			if language.FilterParseAST(path) {
				var ast ssa.FrontAST
				var err error
				ssaprofile.ProfileAdd(true, "ParseAST ", func() {
					ast, err = language.ParseAST(utils.UnsafeBytesToString(content))
				})
				return ast, err
			} else {
				log.Debugf("skip parse ast file: %s", path)
				return nil, nil
			}
		}
		return nil, utils.Errorf("not select language %s", c.language)
	}
	return ssareducer.FilesHandler(
		c.ctx, filesystem, preHandlerFiles,
		parse,
		c.astSequence,
		c.concurrency,
	)
}
