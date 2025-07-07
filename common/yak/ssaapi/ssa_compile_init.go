package ssaapi

import (
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (c *config) init(filesystem filesys_interface.FileSystem) (*ssa.Program, *ssa.FunctionBuilder, error) {
	programName := c.ProgramName
	application := ssa.NewProgram(programName, c.enableDatabase, ssa.Application, filesystem, c.programPath, c.cacheTTL...)
	application.Language = string(c.language)

	application.ProcessInfof = func(s string, v ...any) {
		msg := fmt.Sprintf(s, v...)
		log.Info(msg)
	}
	application.Build = func(
		ast ssa.FrontAST, filePath string, src *memedit.MemEditor, fb *ssa.FunctionBuilder,
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
		if src.GetFilename() == "" {
			src.SetUrl(filePath)
		}
		// backup old editor (source code)
		originEditor := fb.GetEditor()
		// TODO: check prog.FileList avoid duplicate file save to sourceDB,
		// in php include just build file in child program, will cause the same file save to sourceDB, when the file include multiple times
		// this check should be more readable, we should use Editor and `prog.PushEditor..` save sourceDB.
		if _, exist := application.FileList[filePath]; !exist {
			if c.enableDatabase {
				folderName, fileName := filesystem.PathSplit(filePath)
				folders := strings.Split(folderName, string(filesystem.GetSeparators()))
				ssadb.SaveFile(fileName, src.GetSourceCode(), programName, folders)
			}
		}
		// include source code will change the context of the origin editor
		newCodeEditor := src
		newCodeEditor.SetUrl(filePath)
		fb.SetEditor(newCodeEditor)
		if originEditor == nil && newCodeEditor != nil {
			enter := fb.GetBasicBlockByID(fb.EnterBlock)
			if enter != nil && enter.GetRange() == nil {
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
			if c.strictMode && err != nil {
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
