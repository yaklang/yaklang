package ssaapi

import (
	"fmt"
	"io"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	js2ssa "github.com/yaklang/yaklang/common/yak/JS2ssa"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"github.com/yaklang/yaklang/common/yak/php/php2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
	"github.com/yaklang/yaklang/common/yak/yak2ssa"
)

type Language string

const (
	Yak  Language = "yak"
	JS   Language = "js"
	PHP  Language = "php"
	JAVA Language = "java"
)

type Builder interface {
	Build(string, bool, *ssa.FunctionBuilder) error
	FilterFile(string) bool
}

var (
	LanguageBuilders = map[Language]Builder{
		Yak:  yak2ssa.Builder,
		JS:   js2ssa.Builder,
		PHP:  php2ssa.Builder,
		JAVA: java2ssa.Builder,
	}
)

func (c *config) parseProject() ([]*Program, error) {
	ret := make([]*Program, 0)

	localpath := c.fs.GetLocalFSPath()
	if localpath == "" {
		localpath = "."
	}

	log.Infof("parse project in fs: %T, localpath: %v", c.fs, localpath)

	// parse project
	err := ssareducer.ReducerCompile(
		localpath, // base
		ssareducer.WithFileSystem(c.fs),
		ssareducer.WithEntryFiles(c.entryFile...),
		ssareducer.WithCompileMethod(func(path string, f io.Reader) (includeFiles []string, err error) {
			log.Infof("start to compile from: %v", path)
			startTime := time.Now()
			prog, err := c.parseSimple(path, f)
			endTime := time.Now()
			log.Infof("compile %s cost: %v", path, endTime.Sub(startTime))
			if err != nil {
				log.Warnf("parse %#v failed: %v", path, err)
				return nil, utils.Errorf("parse file %s error : %v", path, err)
			}
			ret = append(ret, NewProgram(prog, c))
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
	return ret, nil
}

func (c *config) parseFile() (ret *Program, err error) {
	prog, err := c.parseSimple("", c.code)
	if err != nil {
		return nil, err
	}
	return NewProgram(prog, c), nil
}

func (c *config) feed(prog *ssa.Program, code io.Reader) error {
	builder := prog.GetAndCreateFunctionBuilder("main", "main")
	if err := prog.Build("", code, builder); err != nil {
		return err
	}
	builder.Finish()
	ssa4analyze.RunAnalyzer(prog)
	return nil
}

func (c *config) parseSimple(path string, r io.Reader) (ret *ssa.Program, err error) {
	defer func() {
		if r := recover(); r != nil {
			ret = nil
			err = utils.Errorf("parse error with panic : %v", r)
			log.Errorf("parse error with panic : %v", err)
		}
	}()

	prog, builder, err := c.init(path)
	if err != nil {
		return nil, err
	}
	// parse code
	if err := prog.Build(path, r, builder); err != nil {
		return nil, err
	}
	builder.Finish()
	ssa4analyze.RunAnalyzer(prog)
	prog.Finish()
	return prog, nil
}

func (c *config) init(path string) (*ssa.Program, *ssa.FunctionBuilder, error) {
	LanguageBuilder := c.Builder
	language := c.language
	programName := c.DatabaseProgramName

	if path != "" {
		// TODO: whether to use the same programName for all program ?? when call ParseProject
		// programName += "-" + path

		// auto select language by path filter, if not set language
		if LanguageBuilder != nil {
			if !LanguageBuilder.FilterFile(path) {
				return nil, nil, utils.Errorf("file[%s] is filtered by language [%s], skip this file", path, language)
			}
		} else {
			for lang, languageBuilder := range LanguageBuilders {
				if languageBuilder.FilterFile(path) {
					language = lang
					LanguageBuilder = languageBuilder
					break
				}
			}
			if LanguageBuilder == nil {
				return nil, nil, utils.Errorf("file[%s] is not supported by any language builder, skip this file", path)
			}
			log.Infof("file[%s] is supported by language [%s], use this language", path, language)
		}
	} else {
		// path is empty, use language or YakLang as default
		if LanguageBuilder == nil {
			LanguageBuilder = LanguageBuilders[Yak]
			log.Infof("use default language [%s] for empty path", Yak)
		}
	}

	prog := ssa.NewProgram(programName, c.fs)

	prog.Build = func(filePath string, src io.Reader, fb *ssa.FunctionBuilder) error {
		// check builder
		if LanguageBuilder == nil {
			return utils.Errorf("not support language %s", c.language)
		}

		// get source code
		if src == nil {
			return fmt.Errorf("reader is nil")
		}
		raw, err := io.ReadAll(src)
		if err != nil {
			return err
		}
		code := utils.UnsafeBytesToString(raw)

		// backup old editor (source code)
		originEditor := fb.GetEditor()

		// include source code will change the context of the origin editor
		newCodeEditor := memedit.NewMemEditor(code)
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
			progName, hash := prog.GetProgramName(), ret.SourceCodeMd5()
			if cache.IsExistedSourceCodeHash(progName, hash) {
				c.DatabaseProgramCacheHitter(fb)
			}
		} else {
			log.Warnf("(BUG or in DEBUG Mode)Range not found for %s", fb.GetName())
		}

		return LanguageBuilder.Build(code, c.ignoreSyntaxErr, fb)
	}

	builder := prog.GetAndCreateFunctionBuilder("main", "main")
	// TODO: this extern info should be set in program
	builder.WithExternLib(c.externLib)
	builder.WithExternValue(c.externValue)
	builder.WithExternMethod(c.externMethod)
	builder.WithDefineFunction(c.defineFunc)
	return prog, builder, nil
}
