package java2ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/sca"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"strings"
)

var _ ssa.ExtraFileAnalyzer = &SSABuilder{}

func (*SSABuilder) EnableExtraFileAnalyzer() bool {
	return true
}

func (s *SSABuilder) ExtraFileAnalyze(fs filesys.FileSystem, path string) error {
	dirname, filename := fs.PathSplit(path)
	_ = dirname
	_ = filename

	// pom.xml
	if strings.TrimLeft(filename, "/") == "pom.xml" {
		raw, err := fs.ReadFile(path)
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
		s.Dependencies = append(s.Dependencies, pkgs...)
		return nil
	}

	switch strings.ToLower(fs.Ext(path)) {
	case ".xml":
		log.Infof("scan xml file: %v", path)
	case ".properties":
		log.Infof("scan properties file: %v", path)
	}
	return nil
}
