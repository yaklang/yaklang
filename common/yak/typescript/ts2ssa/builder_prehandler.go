package ts2ssa

import (
	"path/filepath"
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
	return extension == ".js" || extension == ".ts" || extension == ".tsx" || extension == ".d.ts" || extension == ".jsx"
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
			log.Warnf("scan pom.xml error: %v", err)
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
	case ".ts", ".js":
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
