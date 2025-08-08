package ssaapi

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/c2ssa"
	"github.com/yaklang/yaklang/common/yak/typescript/js2ssa"

	//js2ssa "github.com/yaklang/yaklang/common/yak/JS2ssa"
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
	C    = consts.C
)

var LanguageBuilders = map[consts.Language]ssa.Builder{
	Yak:  yak2ssa.Builder,
	JS:   js2ssa.Builder,
	PHP:  php2ssa.Builder,
	JAVA: java2ssa.Builder,
	GO:   go2ssa.Builder,
	C:    c2ssa.Builder,
}

var AllLanguageBuilders = []ssa.Builder{
	php2ssa.Builder,
	java2ssa.Builder,

	yak2ssa.Builder,
	js2ssa.Builder,
	go2ssa.Builder,
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

func (c *config) parseFile() (ret *Program, err error) {
	var prog *ssa.Program
	prog, err = c.parseSimple(c.originEditor)
	if err != nil {
		return nil, err
	}
	c.originEditor.SetProgramName(prog.GetProgramName())
	prog.SaveEditor(c.originEditor)
	prog.Finish()
	wait := func() {}
	if prog.DatabaseKind != ssa.ProgramCacheMemory { // save program
		wait = prog.UpdateToDatabase()
	}
	total := prog.Cache.CountInstruction()
	prog.ProcessInfof("program %s finishing save cache instruction(len:%d) to database", prog.Name, total) // %90
	prog.Cache.SaveToDatabase()
	c.SaveConfig()
	wait()
	return NewProgram(prog, c), nil
}

func (c *config) feed(prog *ssa.Program, code *memedit.MemEditor) error {
	return utils.Errorf("not implemented")
	// builder := prog.GetAndCreateFunctionBuilder(string(ssa.MainFunctionName), string(ssa.MainFunctionName))
	// if err := prog.Build("", code, builder); err != nil {
	// 	return err
	// }
	// builder.Finish()
	// ssa4analyze.RunAnalyzer(prog)
	// return nil
}

func (c *config) parseSimple(r *memedit.MemEditor) (ret *ssa.Program, err error) {
	defer func() {
		if r := recover(); r != nil {
			ret = nil
			err = utils.Errorf("parse error with panic : %v", r)
			log.Errorf("parse error with panic : %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	// path is empty, use language or YakLang as default
	if c.SelectedLanguageBuilder == nil {
		c.LanguageBuilder = LanguageBuilders[Yak]
		log.Debugf("use default language [%s] for empty path", Yak)
	} else {
		c.LanguageBuilder = c.SelectedLanguageBuilder
	}
	c.LanguageBuilder = c.LanguageBuilder.Create()
	prog, builder, err := c.init(c.fs, 1)
	prog.SetPreHandler(true)
	c.LanguageBuilder.InitHandler(builder)
	// builder.SetRangeInit(r)
	if err != nil {
		return nil, err
	}
	ast, err := c.LanguageBuilder.ParseAST(r.GetSourceCode())
	if !c.ignoreSyntaxErr && err != nil {
		return nil, utils.Errorf("parse file error: %v", err)
	}
	c.LanguageBuilder.PreHandlerFile(ast, r, builder)
	// parse code
	prog.SetPreHandler(false)
	if err := prog.Build(ast, r, builder); err != nil {
		return nil, err
	}
	builder.Finish()
	ssa4analyze.RunAnalyzer(prog)
	return prog, nil
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
