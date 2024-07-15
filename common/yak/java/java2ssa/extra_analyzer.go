package java2ssa

import (
	"io/fs"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/sca"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

var _ ssa.ExtraFileAnalyzer = &SSABuilder{}

func (*SSABuilder) EnableExtraFileAnalyzer() bool {
	return true
}

func (s *SSABuilder) ExtraFileAnalyze(fileSystem filesys.FileSystem, prog *ssa.Program, base string) error {
	if prog.ExtraFile == nil {
		prog.ExtraFile = make(map[string]string)
	}

	handlerFile := func(path string) {
		dirname, filename := fileSystem.PathSplit(path)
		_ = dirname
		_ = filename

		// pom.xml
		if strings.TrimLeft(filename, "/") == "pom.xml" {
			raw, err := fileSystem.ReadFile(path)
			if err != nil {
				log.Warnf("read pom.xml error: %v", err)
				return
			}
			vfs := filesys.NewVirtualFs()
			vfs.AddFile("pom.xml", string(raw))
			pkgs, err := sca.ScanFilesystem(vfs)
			if err != nil {
				log.Warnf("scan pom.xml error: %v", err)
				return
			}
			if prog == nil {
				prog = &ssa.Program{}
			}
			prog.SCAPackages = append(prog.SCAPackages, pkgs...)
		}

		switch strings.ToLower(fileSystem.Ext(path)) {
		case ".xml":
			raw, err := fileSystem.ReadFile(path)
			if err != nil {
				log.Warnf("read file %s error: %v", path, err)
				return
			}
			// log.Infof("scan xml file: %v", path)
			prog.ExtraFile[path] = string(raw)
		case ".properties":
			raw, err := fileSystem.ReadFile(path)
			if err != nil {
				log.Warnf("read file %s error: %v", path, err)
				return
			}
			prog.ExtraFile[path] = string(raw)
		}
	}

	filesys.Recursive(base,
		filesys.WithFileSystem(fileSystem),
		filesys.WithFileStat(func(s string, fi fs.FileInfo) error {
			handlerFile(s)
			return nil
		}),
	)

	return nil
}
