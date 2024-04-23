package yak2ssa

import (
	"strconv"

	"github.com/yaklang/yaklang/common/log"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

func (s *astbuilder) buildInclude(i *yak.IncludeStmtContext) {
	targetFile := i.StringLiteral().GetText()
	targetFile, _ = strconv.Unquote(targetFile)

	if err := s.BuildFilePackage(targetFile, false); err != nil {
		log.Errorf("yaklang builder include %v failed: %v", targetFile, err)
	}
}
