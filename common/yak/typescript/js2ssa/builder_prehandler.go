package js2ssa

import (
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (*SSABuilder) FilterPreHandlerFile(path string) bool {
	// 排除 node_modules 目录
	if strings.Contains(path, "node_modules") {
		return false
	}

	// 排除其他常见的不需要解析的目录
	excludeDirs := []string{
		".git", ".svn", ".hg", // 版本控制
		"dist", "build", "out", // 构建输出目录
		".next", ".nuxt", ".vitepress", // 框架构建目录
		"coverage", ".nyc_output", // 测试覆盖率
		".cache", "tmp", "temp", // 缓存和临时目录
	}

	for _, dir := range excludeDirs {
		if strings.Contains(path, dir+string(filepath.Separator)) ||
			strings.HasSuffix(path, dir) {
			return false
		}
	}
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
