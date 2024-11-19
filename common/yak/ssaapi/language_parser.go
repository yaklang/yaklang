package ssaapi

import (
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/consts"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/memedit"
	js2ssa "github.com/yaklang/yaklang/common/yak/JS2ssa"
	"github.com/yaklang/yaklang/common/yak/go2ssa"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"github.com/yaklang/yaklang/common/yak/php/php2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
	"github.com/yaklang/yaklang/common/yak/yak2ssa"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
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

func (c *config) parseProject() (Programs, error) {

	defer func() {
		if r := recover(); r != nil {
			// err = utils.Errorf("parse [%s] error %v  ", path, r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	if c.reCompile {
		ssadb.DeleteProgram(ssadb.GetDB(), c.ProgramName)
		if c.SaveToProfile {
			ssadb.DeleteSSAProgram(c.ProgramName)
		}
	}
	if c.databasePath != "" {
		consts.SetSSADataBasePath(c.databasePath)
	}
	programPath := c.programPath
	prog, builder, err := c.init()

	if err != nil {
		return nil, err
	}
	if prog.Name != "" {
		ssadb.SaveFolder(prog.Name, []string{"/"})
	}

	totalProcess := 0
	handledProcess := 0
	prog.ProcessInfof = func(s string, v ...any) {
		msg := fmt.Sprintf(s, v...)
		// handled := len(prog.FileList)
		if c.process != nil {
			c.process(msg, float64(handledProcess)/float64(totalProcess))
		} else {
			log.Info(msg)
		}
	}
	preHandlerSize := 0
	parseSize := 0

	// get total size
	filesys.Recursive(programPath,
		filesys.WithFileSystem(c.fs),
		filesys.WithContext(c.ctx),
		filesys.WithDirStat(func(s string, fi fs.FileInfo) error {
			_, name := c.fs.PathSplit(s)
			if name == "test" || name == ".git" {
				return filesys.SkipDir
			}
			return nil
		}),
		filesys.WithFileStat(func(path string, fi fs.FileInfo) error {
			if fi.Size() == 0 {
				return nil
			}
			if c.checkLanguage(path) == nil {
				parseSize++
			}
			if c.checkLanguagePreHandler(path) == nil {
				preHandlerSize++
			}
			// log.Infof("nomatch when calc total: %s", path)
			return nil
		}),
	)
	if c.isStop() {
		return nil, utils.Errorf("parse project stop")
	}
	if (parseSize + preHandlerSize) == 0 {
		return nil, utils.Errorf("no file can compile with language[%s]", c.language)
	}
	totalProcess = parseSize + preHandlerSize + 1

	// pre handler
	prog.SetPreHandler(true)
	prog.ProcessInfof("pre-handler parse project in fs: %v, path: %v", c.fs, programPath)
	filesys.Recursive(programPath,
		filesys.WithFileSystem(c.fs),
		filesys.WithContext(c.ctx),
		filesys.WithDirStat(func(s string, fi fs.FileInfo) error {
			_, name := c.fs.PathSplit(s)
			if name == "test" || name == ".git" {
				return filesys.SkipDir
			}
			return nil
		}),
		filesys.WithFileStat(func(path string, fi fs.FileInfo) (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = utils.Errorf("parse [%s] error %v  ", path, r)
					utils.PrintCurrentGoroutineRuntimeStack()
				}
			}()
			if fi.Size() == 0 {
				return nil
			}
			// check
			if err := c.checkLanguagePreHandler(path); err != nil {
				return nil
			}
			handledProcess++
			if language := c.LanguageBuilder; language != nil {
				language.InitHandler(builder)
				language.PreHandlerProject(c.fs, builder, path)
			}
			return nil
		}),
	)
	if c.isStop() {
		return nil, utils.Errorf("parse project stop")
	}
	prog.ProcessInfof("pre-handler parse project finish")
	handledProcess = preHandlerSize // finish pre-handler 50%

	// parse project
	prog.ProcessInfof("parse project start")
	prog.SetPreHandler(false)
	err = ssareducer.ReducerCompile(
		programPath, // base
		ssareducer.WithFileSystem(c.fs),
		ssareducer.WithProgramName(c.ProgramName),
		ssareducer.WithEntryFiles(c.entryFile...),
		ssareducer.WithContext(c.ctx),
		ssareducer.WithStrictMode(c.strictMode),
		// ssareducer.with
		ssareducer.WithCompileMethod(func(path string, raw string) (includeFiles []string, err error) {
			defer func() {
				if r := recover(); r != nil {
					// ret = nil
					includeFiles = prog.GetIncludeFiles()
					// TODO: panic shuold be upload
					// err = utils.Errorf("parse error with panic : %v", r)
					log.Errorf("parse [%s] error %v  ", path, r)
					utils.PrintCurrentGoroutineRuntimeStack()
				}
			}()

			// check
			if err := c.checkLanguage(path); err != nil {
				log.Warnf("parse file %s error: %v", path, err)
				return nil, nil
			}
			handledProcess++

			// build
			if err := prog.Build(path, memedit.NewMemEditor(raw), builder); err != nil {
				log.Debugf("parse %#v failed: %v", path, err)
				return nil, utils.Wrapf(err, "parse file %s error", path)
			}
			exclude := prog.GetIncludeFiles()
			if len(exclude) > 0 {
				log.Infof("program include files: %v will not be as the entry from project", len(exclude))
			}
			return exclude, nil
		}),
	)
	if err != nil {
		return nil, utils.Wrap(err, "parse project error")
	}
	if c.isStop() {
		return nil, utils.Errorf("parse project stop")
	}
	handledProcess = preHandlerSize + parseSize
	prog.ProcessInfof("program %s finishing", prog.Name) // %99
	prog.LazyBuild()
	prog.Finish()
	var progs = []*Program{NewProgram(prog, c)}
	if c.SaveToProfile {
		ssadb.SaveSSAProgram(c.ProgramName, c.ProgramDescription, string(c.language))
	}
	handledProcess = preHandlerSize + parseSize + 1
	prog.ProcessInfof("program %s finish", prog.Name) // %100
	return progs, nil
}

func (c *config) parseFile() (ret *Program, err error) {
	if c.databasePath != "" {
		consts.SetSSADataBasePath(c.databasePath)
	}
	prog, err := c.parseSimple(c.originEditor)
	if err != nil {
		return nil, err
	}
	prog.LazyBuild()
	prog.Finish()
	if c.SaveToProfile {
		ssadb.SaveSSAProgram(c.ProgramName, c.ProgramDescription, string(c.language))
	}
	return NewProgram(prog, c), nil
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
		log.Infof("use default language [%s] for empty path", Yak)
	} else {
		c.LanguageBuilder = c.SelectedLanguageBuilder
	}
	c.LanguageBuilder = c.LanguageBuilder.Create()
	prog, builder, err := c.init()
	prog.SetPreHandler(true)
	c.LanguageBuilder.InitHandler(builder)
	// builder.SetRangeInit(r)
	if err != nil {
		return nil, err
	}
	c.LanguageBuilder.PreHandlerFile(r, builder)
	// parse code
	prog.SetPreHandler(false)
	if err := prog.Build("", r, builder); err != nil {
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

func (c *config) init() (*ssa.Program, *ssa.FunctionBuilder, error) {
	programName := c.ProgramName
	application := ssa.NewProgram(programName, c.ProgramName != "", ssa.Application, c.fs, c.programPath)
	application.Language = string(c.language)

	application.ProcessInfof = func(s string, v ...any) {
		msg := fmt.Sprintf(s, v...)
		log.Info(msg)
	}
	application.Build = func(
		filePath string, src *memedit.MemEditor, fb *ssa.FunctionBuilder,
	) (err error) {
		application.ProcessInfof("start to compile : %v", filePath)
		start := time.Now()
		defer func() {
			application.ProcessInfof(
				"compile finish file: %s, cost: %v",
				filePath, time.Since(start),
			)
		}()

		LanguageBuilder := c.LanguageBuilder
		// check builder
		if LanguageBuilder == nil {
			return utils.Errorf("not support language %s", c.language)
		}
		if application.Language == "" {
			application.Language = string(LanguageBuilder.GetLanguage())
		}

		// get source code
		if src == nil {
			return fmt.Errorf("origin source code (MemEditor) is nil")
		}
		// backup old editor (source code)
		originEditor := fb.GetEditor()
		// TODO: check prog.FileList avoid duplicate file save to sourceDB,
		// in php include just build file in child program, will cause the same file save to sourceDB, when the file include multiple times
		// this check should be more readable, we should use Editor and `prog.PushEditor..` save sourceDB.
		if _, exist := application.FileList[filePath]; !exist {
			if programName != "" {
				folderName, fileName := c.fs.PathSplit(filePath)
				folders := []string{programName}
				folders = append(folders,
					strings.Split(folderName, string(c.fs.GetSeparators()))...,
				)
				src.ResetSourceCodeHash()
				ssadb.SaveFile(fileName, src.GetSourceCode(), folders)
			}
		}
		// include source code will change the context of the origin editor
		newCodeEditor := src
		newCodeEditor.SetUrl(filePath)
		fb.SetEditor(newCodeEditor)
		if originEditor == nil && newCodeEditor != nil {
			if fb.EnterBlock != nil && fb.EnterBlock.GetRange() == nil {
				fb.EnterBlock.SetRange(src.GetFullRange())
			}
		}
		if originEditor != nil {
			originEditor.PushSourceCodeContext(newCodeEditor.SourceCodeMd5())
		}
		// push into program for recording what code is compiling
		application.PushEditor(newCodeEditor)
		defer func() {
			// recover source code context
			fb.SetEditor(originEditor)
			save := true
			if c.strictMode && err != nil {
				save = false
			}
			application.PopEditor(save)
		}()

		if ret := fb.GetEditor(); ret != nil {
			cache := application.Cache
			progName, hash := application.GetProgramName(), codec.Sha256(ret.GetSourceCode())
			if cache.IsExistedSourceCodeHash(progName, hash) {
				c.DatabaseProgramCacheHitter(fb)
			}
		} else {
			log.Warnf("(BUG or in DEBUG Mode)Range not found for %s", fb.GetName())
		}
		return LanguageBuilder.Build(src.GetSourceCode(), c.ignoreSyntaxErr, fb)
	}
	builder := application.GetAndCreateFunctionBuilder("main", "main")
	// TODO: this extern info should be set in program
	builder.WithExternLib(c.externLib)
	builder.WithExternValue(c.externValue)
	builder.WithExternMethod(c.externMethod)
	builder.WithExternBuildValueHandler(c.externBuildValueHandler)
	builder.WithDefineFunction(c.defineFunc)
	builder.SetContext(c.ctx)
	return application, builder, nil
}
