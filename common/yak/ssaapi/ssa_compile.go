package ssaapi

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/c2ssa"
	"github.com/yaklang/yaklang/common/yak/typescript/js2ssa"
	"github.com/yaklang/yaklang/common/yak/yak2ssa"

	//js2ssa "github.com/yaklang/yaklang/common/yak/JS2ssa"
	"github.com/yaklang/yaklang/common/yak/go2ssa"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"github.com/yaklang/yaklang/common/yak/php/php2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
)

const (
	Yak  = consts.Yak
	JS   = consts.JS
	PHP  = consts.PHP
	JAVA = consts.JAVA
	GO   = consts.GO
	C    = consts.C
)

var LanguageBuilderCreater = map[consts.Language]ssa.CreateBuilder{
	Yak:  yak2ssa.CreateBuilder,
	JS:   js2ssa.CreateBuilder,
	PHP:  php2ssa.CreateBuilder,
	JAVA: java2ssa.CreateBuilder,
	GO:   go2ssa.CreateBuilder,
	C:    c2ssa.CreateBuilder,
}

func (c *Config) isStop() bool {
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

func (c *Config) parseFile() (ret *Program, err error) {
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
	wait()
	p := NewProgram(prog, c)
	SaveConfig(c, p)
	return p, nil
}

func (c *Config) feed(prog *ssa.Program, code *memedit.MemEditor) error {
	return utils.Errorf("not implemented")
	// builder := prog.GetAndCreateFunctionBuilder(string(ssa.MainFunctionName), string(ssa.MainFunctionName))
	// if err := prog.Build("", code, builder); err != nil {
	// 	return err
	// }
	// builder.Finish()
	// ssa4analyze.RunAnalyzer(prog)
	// return nil
}

func (c *Config) parseSimple(r *memedit.MemEditor) (ret *ssa.Program, err error) {
	defer func() {
		if r := recover(); r != nil {
			ret = nil
			err = utils.Errorf("parse error with panic : %v", r)
			log.Errorf("parse error with panic : %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	// path is empty, use language or YakLang as default
	if c.LanguageBuilder == nil {
		c.LanguageBuilder = LanguageBuilderCreater[Yak]()
		log.Debugf("use default language [%s] for empty path", Yak)
	}

	prog, builder, err := c.init(c.fs, 1)
	prog.SetPreHandler(true)
	c.LanguageBuilder.InitHandler(builder)
	// builder.SetRangeInit(r)
	if err != nil {
		return nil, err
	}
	ast, err := c.LanguageBuilder.ParseAST(r.GetSourceCode(), nil)
	defer c.LanguageBuilder.Clearup()
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

func (c *Config) checkLanguagePreHandler(path string) error {
	return c.checkLanguageEx(path, func(builder ssa.Builder) bool {
		return builder.FilterPreHandlerFile(path)
	})
}

func (c *Config) checkLanguage(path string) error {
	return c.checkLanguageEx(path, func(builder ssa.Builder) bool {
		return builder.FilterFile(path)
	})
}

func (c *Config) checkLanguageEx(path string, handler func(ssa.Builder) bool) error {

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
	languageBuilder := c.LanguageBuilder
	if languageBuilder != nil {
		languageBuilder, err = processBuilders(languageBuilder)
	} else {
		log.Warn("no language builder specified, try to use all language builders, but it may cause some error and extra file analyzing disabled")
		for _, builder := range LanguageBuilderCreater {
			languageBuilder, err = processBuilders(builder())
			if err == nil {
				break
			}
		}
	}
	if err != nil {
		return err
	}
	c.LanguageBuilder = languageBuilder
	return nil
}
