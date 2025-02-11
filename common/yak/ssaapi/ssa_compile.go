package ssaapi

import (
	"fmt"

	"github.com/yaklang/yaklang/common/consts"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	js2ssa "github.com/yaklang/yaklang/common/yak/JS2ssa"
	"github.com/yaklang/yaklang/common/yak/go2ssa"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"github.com/yaklang/yaklang/common/yak/php/php2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
	"github.com/yaklang/yaklang/common/yak/yak2ssa"
)

const (
	Yak  = consts.Yak
	JS   = consts.JS
	PHP  = consts.PHP
	JAVA = consts.JAVA
	GO   = consts.GO
)

var LanguageBuilders = map[consts.Language]ssa.Builder{
	Yak:  yak2ssa.Builder,
	JS:   js2ssa.Builder,
	PHP:  php2ssa.Builder,
	JAVA: java2ssa.Builder,
	GO:   go2ssa.Builder,
}

var AllLanguageBuilders = []ssa.Builder{
	php2ssa.Builder,
	java2ssa.Builder,

	yak2ssa.Builder,
	js2ssa.Builder,
	go2ssa.Builder,
}

const simpleFileName = "$$File$$Input"

func SimpleFilePath(language ssa.Builder) string {
	if language != nil {
		return fmt.Sprintf("%s.%s", simpleFileName, language.GetCodeFileExt())
	} else {
		return simpleFileName
	}
}
func (c *config) isStop() bool {
	if c == nil || c.ctx == nil {
		return false
	}
	select {
	case <-c.ctx.Done():
		return true
	default:
		return false
	}
}

func (c *config) feed(prog *ssa.Program, code *memedit.MemEditor) error {
	builder := prog.GetAndCreateFunctionBuilder("main", "main")
	if err := prog.Build("", code, builder); err != nil {
		return err
	}
	builder.Finish()
	ssa4analyze.RunAnalyzer(prog)
	return nil
}

var SkippedError = ssareducer.SkippedError

func (c *config) checkLanguagePreHandler(path string) error {
	return c.checkLanguageEx(path, func(builder ssa.Builder) bool {
		return builder.FilterPreHandlerFile(path)
	})
}

func (c *config) checkLanguage(path string) error {
	return c.checkLanguageEx(path, func(builder ssa.Builder) bool {
		return builder.FilterFile(path)
	})
}

func (c *config) checkLanguageEx(path string, handler func(ssa.Builder) bool) error {

	processBuilders := func(builders ...ssa.Builder) (ssa.Builder, error) {
		for _, instance := range builders {
			if handler(instance) {
				return instance, nil
			}
		}
		return nil, utils.Wrapf(ssareducer.SkippedError, "file[%s] is not supported by any language builder, skip this file", path)
	}

	// TODO: whether to use the same programName for all program ?? when call ParseProject
	// programName += "-" + path
	var err error
	LanguageBuilder := c.SelectedLanguageBuilder
	if LanguageBuilder != nil {
		LanguageBuilder, err = processBuilders(LanguageBuilder)
	} else {
		log.Warn("no language builder specified, try to use all language builders, but it may cause some error and extra file analyzing disabled")
		LanguageBuilder, err = processBuilders(AllLanguageBuilders...)
	}
	if err != nil {
		return err
	}
	c.LanguageBuilder = LanguageBuilder.Create()
	return nil
}
