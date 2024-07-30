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

func (s *SSABuilder) ExtraFileAnalyze(fileSystem fi.FileSystem, prog *ssa.Program, path string) error {
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
		folders := []string{prog.GetProgramName()}
		folders = append(folders,
			strings.Split(dirname, string(fileSystem.GetSeparators()))...,
		)
		prog.ExtraFile[path] = ssadb.SaveFile(filename, string(raw), folders)
	case ".properties":
		raw, err := fileSystem.ReadFile(path)
		if err != nil {
			log.Warnf("read file %s error: %v", path, err)
			return nil
		}

		folders := []string{prog.GetProgramName()}
		folders = append(folders,
			strings.Split(dirname, string(fileSystem.GetSeparators()))...,
		)
		prog.ExtraFile[path] = ssadb.SaveFile(filename, string(raw), folders)
	}
	// }
	return nil
}
