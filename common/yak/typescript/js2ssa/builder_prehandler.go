package js2ssa

import (
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"path/filepath"
)

func (s *SSABuilder) Create() ssa.Builder {
	return &SSABuilder{
		PreHandlerInit: ssa.NewPreHandlerInit().WithLanguageConfigOpts(
			ssa.WithLanguageConfigBind(true), // 设置处理语言闭包的副作用的策略
			ssa.WithLanguageBuilder(s)),
	}
}

func (*SSABuilder) FilterPreHandlerFile(path string) bool {
	extension := filepath.Ext(path)
	return extension == ".js"
}

func (*SSABuilder) PreHandlerFile(editor *memedit.MemEditor, builder *ssa.FunctionBuilder) {
	return
}

func (s *SSABuilder) PreHandlerProject(fileSystem fi.FileSystem, fb *ssa.FunctionBuilder, path string) error {
	return nil
}
