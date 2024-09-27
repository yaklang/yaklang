package java2ssa

import (
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/sca"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"golang.org/x/exp/slices"
)

var _ ssa.PreHandlerAnalyzer = &SSABuilder{}

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
		s.InitHandlerOnce.Do(func() {
			fb.SetEmptyRange()
			variable := fb.CreateVariable("__dependency__")
			container := fb.EmitEmptyContainer()
			fb.AssignVariable(variable, container)
		})
		handlerDependency(pkgs, fb, filename)
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

func handlerDependency(pkgs []*dxtypes.Package, fb *ssa.FunctionBuilder, filename string) {
	container := fb.ReadValue("__dependency__")
	if container == nil {
		return
	}

	setDependencyRange := func(name string) {
		id := strings.Split(name, ":")
		if len(id) != 2 {
			return
		}
		group, artifact := id[0], id[1]
		rs1 := fb.GetRangeByText(artifact)
		if len(rs1) == 1 {
			fb.SetRangeByRangeIf(rs1[0])
			return
		}
		rs2 := fb.GetRangeByText(group)
		if len(rs2) == 1 {
			fb.SetRangeByRangeIf(rs2[0])
			return
		}
		fb.SetEmptyRange()
	}
	/*
		__dependency__.name?{}
	*/
	fb.SetEmptyRange()
	for _, pkg := range pkgs {
		sub := fb.EmitEmptyContainer()
		// check item
		// 1. name
		// 2. version
		// 3. filename
		// 4. group
		// 5. artifact
		for k, v := range map[string]string{
			"name":     pkg.Name,
			"version":  pkg.Version,
			"filename": filename,
		} {
			if k == "name" {
				setDependencyRange(v)
			}
			fb.AssignVariable(
				fb.CreateMemberCallVariable(sub, fb.EmitConstInst(k)),
				fb.EmitConstInst(v),
			)
		}

		pkgItem := fb.CreateMemberCallVariable(container, fb.EmitConstInst(pkg.Name))
		fb.AssignVariable(pkgItem, sub)
	}
}
