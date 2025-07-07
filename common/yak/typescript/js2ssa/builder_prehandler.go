package js2ssa

import (
	"path/filepath"

	"github.com/yaklang/yaklang/common/log"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (s *SSABuilder) Create() ssa.Builder {
	return &SSABuilder{
		PreHandlerInit: ssa.NewPreHandlerInit().WithLanguageConfigOpts(
			ssa.WithLanguageConfigBind(true), // 设置处理语言闭包的副作用的策略
			ssa.WithLanguageConfigSupportClass(true),
			ssa.WithLanguageConfigIsSupportClassStaticModifier(true),
			ssa.WithLanguageBuilder(s),
			ssa.WithLanguageConfigTryBuildValue(true),
		),
	}
}

func (*SSABuilder) FilterPreHandlerFile(path string) bool {
	extension := filepath.Ext(path)
	return extension == ".js"
}

func (s *SSABuilder) PreHandlerFile(ast ssa.FrontAST, editor *memedit.MemEditor, builder *ssa.FunctionBuilder) {
	builder.GetProgram().GetApplication().Build(ast, "", editor, builder)
}

func (s *SSABuilder) PreHandlerProject(fileSystem fi.FileSystem, ast ssa.FrontAST, fb *ssa.FunctionBuilder, path string) error {
	prog := fb.GetProgram()
	if prog == nil {
		log.Errorf("program is nil")
		return nil
	}
	file, err := fileSystem.ReadFile(path)
	if err != nil {
		log.Errorf("read file %s error: %v", path, err)
		return nil
	}
	prog.Build(ast, path, memedit.NewMemEditor(string(file)), fb)
	return nil
}
