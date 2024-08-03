package java2ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/sca"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

var _ ssa.ExtraFileAnalyzer = &SSABuilder{}

func (*SSABuilder) EnableExtraFileAnalyzer() bool {
	return true
}

func (s *SSABuilder) ProgramHandler(fileSystem fi.FileSystem, functionBuilder *ssa.FunctionBuilder, path string) error {
	prog := functionBuilder.GetProgram()
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
		vfs := filesys.NewVirtualFs()
		vfs.AddFile("pom.xml", string(raw))
		pkgs, err := sca.ScanFilesystem(vfs)
		if err != nil {
			log.Warnf("scan pom.xml error: %v", err)
			return nil
		}
		prog.SCAPackages = append(prog.SCAPackages, pkgs...)
	}

	switch strings.ToLower(fileSystem.Ext(path)) {
	case ".xml":
		raw, err := fileSystem.ReadFile(path)
		if err != nil {
			log.Warnf("read file %s error: %v", path, err)
			return nil
		}
		// log.Infof("scan xml file: %v", path)
		if prog.GetProgramName() == "" {
			prog.ExtraFile[path] = string(raw)
		} else {
			folders := []string{prog.GetProgramName()}
			folders = append(folders,
				strings.Split(dirname, string(fileSystem.GetSeparators()))...,
			)
			prog.ExtraFile[path] = ssadb.SaveFile(filename, string(raw), folders)
		}
	case ".properties":
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
