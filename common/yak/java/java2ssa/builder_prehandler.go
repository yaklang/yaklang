package java2ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	tj "github.com/yaklang/yaklang/common/yak/java/template2java"
	tl "github.com/yaklang/yaklang/common/yak/templateLanguage"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/utils/memedit"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/sca"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"golang.org/x/exp/slices"
)

var _ ssa.PreHandlerAnalyzer = &SSABuilder{}

func (s *SSABuilder) Create() ssa.Builder {
	return &SSABuilder{
		PreHandlerInit: ssa.NewPreHandlerInit(),
	}
}

func (*SSABuilder) FilterPreHandlerFile(path string) bool {
	extension := filepath.Ext(path)
	fileList := []string{".jpg", ".png", ".gif", ".jpeg", ".css", ".js", ".avi", ".mp4", ".mp3", ".pdf", ".doc", ".php", ".go"}
	return !slices.Contains(fileList, extension)
}

func (s *SSABuilder) PreHandlerFile(editor *memedit.MemEditor, builder *ssa.FunctionBuilder) {
	builder.GetProgram().GetApplication().Build("", editor, builder)
}

func (s *SSABuilder) PreHandlerProject(fileSystem fi.FileSystem, fb *ssa.FunctionBuilder, path string) error {
	prog := fb.GetProgram()
	if prog == nil {
		log.Errorf("program is nil")
		return nil
	}
	if prog.ExtraFile == nil {
		prog.ExtraFile = make(map[string]string)
	}

	dirname, filename := fileSystem.PathSplit(path)
	// pom.xml
	if strings.TrimLeft(filename, string(fileSystem.GetSeparators())) == "pom.xml" {
		raw, err := fileSystem.ReadFile(path)
		if err != nil {
			log.Warnf("read pom.xml error: %v", err)
			return nil
		}
		editor := memedit.NewMemEditor(string(raw))
		editor.SetUrl(path)
		fb.SetEditor(editor)
		vfs := filesys.NewVirtualFs()
		vfs.AddFile(filename, string(raw))
		pkgs, err := sca.ScanFilesystem(vfs)
		if err != nil {
			log.Warnf("scan pom.xml error: %v", err)
			return nil
		}
		prog.SCAPackages = append(prog.SCAPackages, pkgs...)
		fb.GenerateDependence(pkgs, filename)
	}

	saveExtraFile := func(path string, raw []byte) {
		if prog.GetProgramName() == "" {
			prog.ExtraFile[path] = string(raw)
		} else {
			folders := []string{prog.GetProgramName()}
			folders = append(folders,
				strings.Split(dirname, string(fileSystem.GetSeparators()))...,
			)
			prog.ExtraFile[path] = ssadb.SaveFile(filename, string(raw), folders)
		}
	}
	switch strings.ToLower(fileSystem.Ext(path)) {
	case ".java", ".class":
		raw, err := fileSystem.ReadFile(path)
		if err != nil {
			return err
		}
		prog.Build(path, memedit.NewMemEditor(string(raw)), fb)
	case ".jpg", ".png", ".gif", ".jpeg", ".css", ".js", ".avi", ".mp4", ".mp3", ".pdf", ".doc":
		return nil
	case ".jsp":
		raw, err := fileSystem.ReadFile(path)
		if err != nil {
			return err
		}
		saveExtraFile(path, raw)
		var info tl.TemplateGeneratedInfo
		info, err = tj.ConvertTemplateToJava(tj.JSP, string(raw), path)
		if err != nil {
			return utils.Errorf("convert jsp to java error: %v", err)
		}
		saveExtraFile(path, raw)
		javaPath := path[:len(path)-4] + "_jsp.java"
		prog.SetTemplate(javaPath, info)
		err = prog.Build(javaPath, memedit.NewMemEditor(info.GetContent()), fb)
		if err != nil {
			return err
		}
	case ".properties":
		raw, err := fileSystem.ReadFile(path)
		if err != nil {
			return err
		}
		saveExtraFile(path, raw)
		err = prog.ParseProjectConfig(string(raw), ssa.PROJECT_CONFIG_PROPERTIES)
		if err != nil {
			return err
		}
	default:
		fs, err := fileSystem.Open(path)
		if err != nil {
			log.Warnf("open file %s error: %v", path, err)
			return nil
		}
		info, err := fs.Stat()
		if err != nil {
			return nil
		}
		if info.Size() > 10*1024*1024 {
			log.Warnf("too large file: %s, skip it.", path)
		}

		raw, err := fileSystem.ReadFile(path)
		if err != nil {
			log.Warnf("read file %s error: %v", path, err)
			return nil
		}
		saveExtraFile(path, raw)
	}
	return nil
}
