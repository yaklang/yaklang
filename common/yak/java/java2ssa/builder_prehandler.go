package java2ssa

import (
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
	fileList := []string{".java", ".jpg", ".png", ".gif", ".jpeg", ".css", ".js", ".avi", ".mp4", ".mp3", ".pdf", ".doc", ".php", ".go"}
	return !slices.Contains(fileList, extension)
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

	// handlerFile := func(path string) {
	dirname, filename := fileSystem.PathSplit(path)
	_ = dirname
	_ = filename

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

	switch strings.ToLower(fileSystem.Ext(path)) {
	case ".java", ".jpg", ".png", ".gif", ".jpeg", ".css", ".js", ".avi", ".mp4", ".mp3", ".pdf", ".doc":
		return nil
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
	return nil
}
