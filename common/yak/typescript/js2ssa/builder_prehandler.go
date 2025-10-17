package js2ssa

import (
	"path/filepath"

	"github.com/yaklang/yaklang/common/log"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (*SSABuilder) FilterPreHandlerFile(path string) bool {
	extension := filepath.Ext(path)
	return extension == ".js"
}

func (s *SSABuilder) PreHandlerFile(ast ssa.FrontAST, editor *memedit.MemEditor, builder *ssa.FunctionBuilder) {
	builder.GetProgram().GetApplication().Build(ast, editor, builder)
}

func (s *SSABuilder) PreHandlerProject(fileSystem fi.FileSystem, ast ssa.FrontAST, fb *ssa.FunctionBuilder, editor *memedit.MemEditor) error {
	prog := fb.GetProgram()
	if prog == nil {
		log.Errorf("program is nil")
		return nil
	}
	prog.Build(ast, editor, fb)
	return nil
}
