package java2ssa

import (
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	tj "github.com/yaklang/yaklang/common/yak/java/template2java"
	tl "github.com/yaklang/yaklang/common/yak/templateLanguage"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/sca"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"golang.org/x/exp/slices"
)

var _ ssa.PreHandlerAnalyzer = &SSABuilder{}

func (*SSABuilder) FilterPreHandlerFile(path string) bool {
	extension := filepath.Ext(path)
	fileList := []string{".jpg", ".png", ".gif", ".jpeg", ".css", ".js", ".avi", ".mp4", ".mp3", ".pdf", ".doc", ".php", ".go"}
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
	if strings.TrimLeft(filename, string(fileSystem.GetSeparators())) == "pom.xml" {
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
	case ".java":
		prog.Build(ast, editor, fb)
	case ".jpg", ".png", ".gif", ".jpeg", ".css", ".js", ".avi", ".mp4", ".mp3", ".pdf", ".doc":
		return nil

	case ".properties":
		saveExtraFile(path)
		if err := prog.ParseProjectConfig([]byte(editor.GetSourceCode()), path, ssa.PROJECT_CONFIG_PROPERTIES); err != nil {
			return err
		}
	case ".yaml", ".yml":
		saveExtraFile(path)
		if err := prog.ParseProjectConfig([]byte(editor.GetSourceCode()), path, ssa.PROJECT_CONFIG_YAML); err != nil {
			return err
		}
	case ".jsp":
		info, err := tj.ConvertTemplateToJavaWithEditor(tj.JSP, editor)
		if err != nil {
			return utils.Errorf("convert jsp to java error: %v", err)
		}
		prog.SetTemplate(path, info)
		ast, err := s.ParseAST(info.GetContent())
		if err != nil {
			log.Infof("parse jsp file %s error: %v", path, err)
			return err
		}
		editor := prog.CreateEditor([]byte(info.GetContent()), path)
		// editor := memedit.NewMemEditor(info.GetContent())
		err = prog.Build(ast, editor, fb)
		if err != nil {
			return err
		}
	default:
		if isFreemarkerFile(prog, path) {
			var info tl.TemplateGeneratedInfo
			info, err := tj.ConvertTemplateToJavaWithEditor(tj.Freemarker, editor)
			if err != nil {
				return utils.Errorf("convert freemarker to java error: %v", err)
			}
			prog.SetTemplate(path, info)
			saveExtraFile(path)
			ast, err := s.ParseAST(info.GetContent())
			if err != nil {
				return err
			}

			err = prog.Build(ast, memedit.NewMemEditor(info.GetContent()), fb)
			if err != nil {
				return err
			}
			return nil
		}
		saveExtraFile(path)
	}
	return nil
}
