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
)

var LanguageBuilders = map[consts.Language]ssa.Builder{
	Yak:  yak2ssa.Builder,
	JS:   js2ssa.Builder,
	PHP:  php2ssa.Builder,
	JAVA: java2ssa.Builder,
}

var AllLanguageBuilders = []ssa.Builder{
	php2ssa.Builder,
	java2ssa.Builder,

	yak2ssa.Builder,
	js2ssa.Builder,
}

func (c *config) parseProject() (Programs, error) {
	// ret := make([]*ssa.Program, 0)

	programPath := c.programPath
	prog, builder, err := c.init()

	if prog.Name != "" {
		ssadb.SaveFolder(prog.Name, []string{"/"})
	}

	log.Infof("parse project in fs: %T, localpath: %v", c.fs, programPath)

	totalSize := 0
	filesys.Recursive(programPath,
		filesys.WithFileSystem(c.fs),
		filesys.WithFileStat(func(path string, fi fs.FileInfo) error {
			if language := c.LanguageBuilder; language != nil && language.EnableExtraFileAnalyzer() {
				language.ExtraFileAnalyze(c.fs, prog, path)
			}
			totalSize++
			return nil
		}),
	)

	// parse project
	err = ssareducer.ReducerCompile(
		programPath, // base
		ssareducer.WithFileSystem(c.fs),
		ssareducer.WithProgramName(c.DatabaseProgramName),
		ssareducer.WithEntryFiles(c.entryFile...),
		ssareducer.WithCompileMethod(func(path string, raw string) (includeFiles []string, err error) {
			defer func() {
				if r := recover(); r != nil {
					// ret = nil
					includeFiles = nil
					err = utils.Errorf("parse error with panic : %v", r)
					log.Errorf("parse [%s] error %v  ", path, err)
					utils.PrintCurrentGoroutineRuntimeStack()
				}
			}()
			log.Debugf("start to compile from: %v", path)
			startTime := time.Now()

			// check
			if err := c.checkLanguage(path); err != nil {
				return nil, err
			}

			// build
			if err := prog.Build(path, memedit.NewMemEditor((raw)), builder); err != nil {
				log.Debugf("parse %#v failed: %v", path, err)
				return nil, utils.Wrapf(err, "parse file %s error", path)
			}

			endTime := time.Now()
			log.Infof("compile %s cost: %v", path, endTime.Sub(startTime))
			// ret = append(ret, prog)
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
	prog.Finish()
	return []*Program{
		NewProgram(prog, c),
	}, nil
}

func (c *config) parseFile() (ret *Program, err error) {
	prog, err := c.parseSimple(c.originEditor)
	if err != nil {
		return nil, err
	}
	prog.Finish()
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
	if c.LanguageBuilder == nil {
		c.LanguageBuilder = LanguageBuilders[Yak]
		// log.Infof("use default language [%s] for empty path", Yak)
	}
	prog, builder, err := c.init()
	// builder.SetRangeInit(r)
	if err != nil {
		return nil, err
	}
	// parse code
	if err := prog.Build("", r, builder); err != nil {
		return nil, err
	}
	builder.Finish()
	ssa4analyze.RunAnalyzer(prog)
	// prog.Finish()
	return prog, nil
}

var SkippedError = ssareducer.SkippedError

func (c *config) checkLanguage(path string) error {
	LanguageBuilder := c.LanguageBuilder

	processBuilders := func(builders ...ssa.Builder) (ssa.Builder, error) {
		for _, instance := range builders {
			if instance.FilterFile(path) {
				return instance, nil
			}
		}
		return nil, utils.Wrapf(ssareducer.SkippedError, "file[%s] is not supported by any language builder, skip this file", path)
	}

	var err error
	if LanguageBuilder != nil {
		LanguageBuilder, err = processBuilders(LanguageBuilder)
	} else {
		log.Warn("no language builder specified, try to use all language builders, but it may cause some error and extra file analyzing disabled")
		LanguageBuilder, err = processBuilders(AllLanguageBuilders...)
	}
	if err != nil {
		return err
	}
	c.LanguageBuilder = LanguageBuilder
	return nil
}

func (c *config) init() (*ssa.Program, *ssa.FunctionBuilder, error) {
	programName := c.DatabaseProgramName

	prog := ssa.NewProgram(programName, c.DatabaseProgramName != "", ssa.Application, c.fs, c.programPath)
	prog.Language = string(c.language)

	prog.Build = func(filePath string, src *memedit.MemEditor, fb *ssa.FunctionBuilder) error {
		LanguageBuilder := c.LanguageBuilder
		// check builder
		if LanguageBuilder == nil {
			return utils.Errorf("not support language %s", c.language)
		}

		if prog.Language == "" {
			prog.Language = string(LanguageBuilder.GetLanguage())
		}

		// get source code
		if src == nil {
			return fmt.Errorf("origin source code (MemEditor) is nil")
		}
		// backup old editor (source code)
		originEditor := fb.GetEditor()

		if programName != "" {
			folderName, fileName := c.fs.PathSplit(filePath)
			folders := []string{programName}
			folders = append(folders,
				strings.Split(folderName, string(c.fs.GetSeparators()))...,
			)
			src.ResetSourceCodeHash()
			ssadb.SaveFile(fileName, src.GetSourceCode(), folders)
		}
		// include source code will change the context of the origin editor
		newCodeEditor := src
		newCodeEditor.SetUrl(filePath)
		fb.SetEditor(newCodeEditor) // set for current builder
		if originEditor != nil {
			originEditor.PushSourceCodeContext(newCodeEditor.SourceCodeMd5())
		}

		// push into program for recording what code is compiling
		prog.PushEditor(newCodeEditor)
		defer func() {
			// recover source code context
			fb.SetEditor(originEditor)
			prog.PopEditor()
		}()

		if ret := fb.GetEditor(); ret != nil {
			prog := fb.GetProgram()
			cache := prog.Cache
			progName, hash := prog.GetProgramName(), codec.Sha256(ret.GetSourceCode())
			if cache.IsExistedSourceCodeHash(progName, hash) {
				c.DatabaseProgramCacheHitter(fb)
			}
		} else {
			log.Warnf("(BUG or in DEBUG Mode)Range not found for %s", fb.GetName())
		}

		return LanguageBuilder.Build(src.GetSourceCode(), c.ignoreSyntaxErr, fb)
	}

	builder := prog.GetAndCreateFunctionBuilder("main", "main")
	// TODO: this extern info should be set in program
	builder.WithExternLib(c.externLib)
	builder.WithExternValue(c.externValue)
	builder.WithExternMethod(c.externMethod)
	builder.WithExternBuildValueHandler(c.externBuildValueHandler)
	builder.WithDefineFunction(c.defineFunc)
	return prog, builder, nil
}
