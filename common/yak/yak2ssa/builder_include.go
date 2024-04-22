package yak2ssa

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/yaklang/yaklang/common/log"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

func (s *astbuilder) buildInclude(i *yak.IncludeStmtContext) {
	targetFile := i.StringLiteral().GetText()
	targetFile, _ = strconv.Unquote(targetFile)
	// var newCode string
	var fd *os.File
	var err error
	filename := ""
	if filepath.IsAbs(targetFile) {
		fd, err = os.Open(targetFile)
		filename = targetFile
	} else {
		filename, err = filepath.Abs(targetFile)
		if err != nil {
			log.Warnf("yaklang builder include %v failed: %v", targetFile, err)
		}
		fd, err = os.Open(filename)
	}

	if err != nil {
		log.Warnf("yaklang builder include %v failed: %v", targetFile, "empty file")
		return
	}

	// TODO: here need more test-case
	if err := s.GetProgram().Build(filename, fd, s.FunctionBuilder); err != nil {
		log.Errorf("yaklang builder include %v failed: %v", targetFile, err)
	}
}
