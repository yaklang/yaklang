package ts2ssa

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/sca"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (*SSABuilder) FilterPreHandlerFile(path string) bool {
	extension := filepath.Ext(path)
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

	fileList := []string{".xml", ".jpg", ".png", ".gif", ".jpeg", ".css", ".java", ".avi", ".mp4", ".mp3", ".pdf", ".doc", ".php", ".go", ".jsp", ".ico", ".svg", ".scss", ".icon"}
	// TS support direct import of json type file but currently we will not handle json import
	return !slices.Contains(fileList, extension)
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
	if prog.ExtraFile == nil {
		prog.ExtraFile = make(map[string]string)
	}

	filename := editor.GetFilename()
	// pom.xml
	if strings.TrimLeft(filename, string(fileSystem.GetSeparators())) == "package.json" {
		fb.SetEditor(editor)
		vfs := filesys.NewVirtualFs()
		vfs.AddFile(filename, editor.GetSourceCode())
		pkgs, err := sca.ScanFilesystem(vfs)
		if err != nil {
			log.Warnf("scan package.json error: %v", err)
			return nil
		}
		prog.SCAPackages = append(prog.SCAPackages, pkgs...)
		fb.GenerateDependence(pkgs, filename)
	}

	saveExtraFile := func(path string) {
		if prog.GetProgramName() == "" {
			prog.ExtraFile[path] = editor.GetIrSourceHash()
		} else {
			prog.ExtraFile[path] = editor.GetIrSourceHash()
		}
	}
	path := editor.GetUrl()
	switch strings.ToLower(fileSystem.Ext(path)) {
	case ".ts", ".js", ".tsx":
		prog.Build(ast, editor, fb)
	case ".jpg", ".png", ".gif", ".jpeg", ".css", ".avi", ".mp4", ".mp3", ".pdf", ".doc":
		return nil
	case ".json":
		saveExtraFile(path)
		if fileSystem.Base(path) == "tsconfig.json" {
			err := prog.ParseProjectConfig([]byte(editor.GetSourceCode()), path, ssa.PROJECT_CONFIG_JSON)
			return err
		}
	default:
		saveExtraFile(path)
	}
	return nil
}
