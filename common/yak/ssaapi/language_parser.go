package ssaapi

import (
	"fmt"
	"io"
	"io/fs"
	"runtime/debug"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/memedit"
	js2ssa "github.com/yaklang/yaklang/common/yak/JS2ssa"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"github.com/yaklang/yaklang/common/yak/php/php2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
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

func (c *config) parse() (ret *ssa.Program, err error) {
	if c.Builder == nil {
		return nil, utils.Errorf("not support language %s", c.language)
	}
	defer func() {
		if r := recover(); r != nil {
			ret = nil
			err = utils.Errorf("parse error with panic : %v", r)
			debug.PrintStack()
		}
	}()

	prog := ssa.NewProgram(c.DatabaseProgramName, c.fs)
	prog.Build = func(filePath string, src io.Reader, fb *ssa.FunctionBuilder) error {
		// check builder
		if c.Builder == nil {
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

		return c.Builder.Build(code, c.ignoreSyntaxErr, fb)
	}

	builder := prog.GetAndCreateFunctionBuilder("main", "main")
	// TODO: this extern info should be set in program
	builder.WithExternLib(c.externLib)
	builder.WithExternValue(c.externValue)
	builder.WithExternMethod(c.externMethod)
	builder.WithDefineFunction(c.defineFunc)
	if c.fs != nil {
		// parse project
		filesys.Recursive(".",
			filesys.WithFileSystem(c.fs),
			filesys.WithFileStat(func(path string, f fs.File, fi fs.FileInfo) error {
				if !c.Builder.FilterFile(path) {
					return nil
				}
				if err := prog.Build(path, f, builder); err != nil {
					log.Errorf(
						"ssaapi: build file %s with language %s error: %s",
						path, c.language, err,
					)
				}
				return nil
			}),
		)
	} else if c.code != nil {
		// parse code
		if err := prog.Build("", c.code, builder); err != nil {
			return nil, err
		}
	}
	builder.Finish()
	ssa4analyze.RunAnalyzer(prog)
	prog.Finish()
	return prog, nil
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
