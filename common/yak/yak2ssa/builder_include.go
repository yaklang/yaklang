package yak2ssa

import (
	"errors"
	"strconv"

	"github.com/yaklang/yaklang/common/log"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

func (s *astbuilder) buildInclude(i *yak.IncludeStmtContext) {
	targetFile := i.StringLiteral().GetText()
	targetFile, _ = strconv.Unquote(targetFile)

	if err := s.frontend.includeFile(s, targetFile); err != nil {
		s.err = errors.Join(s.err, err)
		log.Errorf("yaklang builder include %v failed: %v", targetFile, err)
	}
}
