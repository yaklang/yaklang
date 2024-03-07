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
	var newCode string
	if filepath.IsAbs(targetFile) {
		codeRaw, _ := os.ReadFile(targetFile)
		newCode = string(codeRaw)
	} else {
		filename, err := filepath.Abs(targetFile)
		if err != nil {
			log.Warnf("yaklang builder include %v failed: %v", targetFile, err)
		}
		codeRaw, _ := os.ReadFile(filename)
		newCode = string(codeRaw)
	}

	if newCode == "" {
		log.Warnf("yaklang builder include %v failed: %v", targetFile, "empty file")
		return
	}

	s.recordIncludeFile(targetFile, newCode)
	// TODO: here need more test-case
	if err := Build(newCode, false, s.FunctionBuilder); err != nil {
		log.Errorf("yaklang builder include %v failed: %v", targetFile, err)
	}
}

func (v *astbuilder) recordIncludeFile(i string, code string) {
	v.Function.PushReferenceFile(i, code)
}
