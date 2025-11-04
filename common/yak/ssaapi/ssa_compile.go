package ssaapi

import (
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/utils/memedit"

	//js2ssa "github.com/yaklang/yaklang/common/yak/JS2ssa"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

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
	// 添加defer清理逻辑，确保编译失败或panic时清理已保存的数据
	defer func() {
		if r := recover(); r != nil {
			err = utils.Errorf("compile panic: %v", r)
			log.Errorf("compile panic: %v", r)
			utils.PrintCurrentGoroutineRuntimeStack()
			// panic时清理已保存的Program数据
			if prog != nil && prog.Name != "" && prog.DatabaseKind != ssa.ProgramCacheMemory {
				log.Infof("cleaning up program data due to panic: %s", prog.Name)
				ssadb.DeleteProgram(ssadb.GetDB(), prog.Name)
			}
		} else if err != nil {
			// 编译出错时清理已保存的Program数据
			if prog != nil && prog.Name != "" && prog.DatabaseKind != ssa.ProgramCacheMemory {
				log.Infof("cleaning up program data due to error: %s", prog.Name)
				ssadb.DeleteProgram(ssadb.GetDB(), prog.Name)
			}
		}
	}()

	if c.SSACompile != nil && c.SSACompile.StopOnCliCheck && c.originEditor != nil {
		if err := c.truncateCodeBeforeCliCheck(); err != nil {
			log.Warnf("failed to truncate code before cli.check(): %v", err)
		}
	}

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

// truncateCodeBeforeCliCheck 查找第一个 cli.check() 并截取到该位置之前的代码
func (c *Config) truncateCodeBeforeCliCheck() error {
	if c.originEditor == nil {
		return nil
	}

	var cliCheckOffset int = -1
	_, found := c.originEditor.FindStringRangeIndexFirst(0, "cli.check", func(r *memedit.Range) {
		cliCheckOffset = r.GetStartOffset()
	})

	if !found || cliCheckOffset == -1 {
		return nil
	}

	codeBeforeCliCheck := c.originEditor.GetSourceCode(0, cliCheckOffset)

	newEditor := memedit.NewMemEditor(codeBeforeCliCheck)
	newEditor.SetProgramName(c.originEditor.GetProgramName())
	newEditor.SetUrl(c.originEditor.GetUrl())
	newEditor.SetFolderPath(c.originEditor.GetFolderPath())

	c.originEditor = newEditor

	return nil
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
	// 确保 diagnostics 包被识别为已使用
	var _ *diagnostics.Recorder
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
		c.LanguageBuilder = LanguageBuilderCreater[ssaconfig.Yak]()
		log.Debugf("use default language [%s] for empty path", ssaconfig.Yak)
	}

	prog, builder, err := c.init(c.fs, 1)
	prog.SetPreHandler(true)
	c.LanguageBuilder.InitHandler(builder)
	// builder.SetRangeInit(r)
	if err != nil {
		return nil, err
	}

	// AST 解析阶段
	astStart := time.Now()
	ast, err := c.LanguageBuilder.ParseAST(r.GetSourceCode(), nil)
	astDuration := time.Since(astStart)
	if c.diagnosticsRecorder != nil {
		c.diagnosticsRecorder.RecordDuration("SSA AST Parse", astDuration)
	}
	defer c.LanguageBuilder.Clearup()
	if !c.ignoreSyntaxErr && err != nil {
		return nil, utils.Errorf("parse file error: %v", err)
	}
	c.LanguageBuilder.PreHandlerFile(ast, r, builder)

	// Build 阶段
	prog.SetPreHandler(false)
	buildStart := time.Now()
	if err := prog.Build(ast, r, builder); err != nil {
		return nil, err
	}
	builder.Finish()
	ssa4analyze.RunAnalyzer(prog)
	buildDuration := time.Since(buildStart)
	if c.diagnosticsRecorder != nil {
		c.diagnosticsRecorder.RecordDuration("SSA Build", buildDuration)
	}
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

func (c *Config) swapLanguageFs(fs fi.FileSystem) fi.FileSystem {
	if c.LanguageBuilder != nil {
		return c.LanguageBuilder.WrapWithPreprocessedFS(fs)
	}
	return c.fs
}
