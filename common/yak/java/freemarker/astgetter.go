package freemarker

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	freemarkerparser "github.com/yaklang/yaklang/common/yak/java/freemarker/parser"
)

func GetAST(code string) (*freemarkerparser.FreemarkerParser, error) {
	var ins any = antlr4util.GetASTParser(code, freemarkerparser.NewFreemarkerLexer, freemarkerparser.NewFreemarkerParser)
	switch ret := ins.(type) {
	case *freemarkerparser.FreemarkerParser:
		return ret, nil
	}
	return nil, utils.Errorf("failed to get freemarker.AST, got: %T", ins)
}
