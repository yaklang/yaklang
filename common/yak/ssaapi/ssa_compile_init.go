package ssaapi

import (
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (c *Config) init(filesystem filesys_interface.FileSystem, fileSize int) (*ssa.Program, *ssa.FunctionBuilder, error) {
	programName := c.GetProgramName()
	application := ssa.NewProgram(programName, c.databaseKind, ssa.Application, filesystem, c.programPath, fileSize, c.cacheTTL...)
	application.Language = c.GetLanguage()
	application.ProjectName = c.GetProjectName()
	application.ProcessInfof = func(s string, v ...any) {
		msg := fmt.Sprintf(s, v...)
		log.Info(msg)
	}

	application.Build = func(
		ast ssa.FrontAST, src *memedit.MemEditor, fb *ssa.FunctionBuilder,
	) (err error) {
		filePath := src.GetUrl()
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
			return utils.Errorf("not support language %s", c.GetLanguage())
		}
		if application.Language == "" {
			application.Language = LanguageBuilder.GetLanguage()
		}

		// get source code
		if src == nil {
			return fmt.Errorf("origin source code (MemEditor) is nil")
		}
		if src.GetFilename() == "" {
			src.SetUrl(filePath)
		}
		// backup old editor (source code)
		originEditor := fb.GetEditor()
		// include source code will change the context of the origin editor
		newCodeEditor := src
		fb.SetEditor(newCodeEditor)
		if originEditor == nil && newCodeEditor != nil {
			enter, ok := fb.GetBasicBlockByID(fb.EnterBlock)
			if ok && enter != nil && enter.GetRange() == nil {
				enter.SetRange(src.GetFullRange())
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
			if c.GetCompileStrictMode() && err != nil {
				save = false
			}
			application.PopEditor(save)
		}()

		if editor := fb.GetEditor(); editor != nil {
			// cache := application.Cache
			// progName := application.GetProgramName()
			// go func() {
			// 	hash := editor.GetIrSourceHash(programName)
			// 	if cache.IsExistedSourceCodeHash(progName, hash) {
			// 		c.DatabaseProgramCacheHitter(fb)
			// 	}
			// }()
		} else {
			log.Warnf("(BUG or in DEBUG Mode)Range not found for %s", fb.GetName())
		}
		err = LanguageBuilder.BuildFromAST(ast, fb)
		return err
	}
	builder := application.GetAndCreateFunctionBuilder(string(ssa.MainFunctionName), string(ssa.MainFunctionName))
	// TODO: this extern info should be set in program
	builder.WithExternLib(c.externLib)
	builder.WithExternValue(c.externValue)
	builder.WithExternMethod(c.externMethod)
	builder.WithExternBuildValueHandler(c.externBuildValueHandler)
	builder.WithDefineFunction(c.defineFunc)
	builder.SetContext(c.ctx)
	return application, builder, nil
}
