package java2ssa

import (
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"strings"
)

var _ ssa.ExtraFileAnalyzer = &SSABuilder{}

func (*SSABuilder) EnableExtraFileAnalyzer() bool {
	return true
}

func (s *SSABuilder) ExtraFileAnalyze(fs filesys.FileSystem, path string) error {
	idx := strings.LastIndexFunc(path, func(r rune) bool {
		if r == fs.GetSeparators() {
			return true
		}
		return false
	})
	if idx == -1 {
		return nil
	}
	return nil
}
