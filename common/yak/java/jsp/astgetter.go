package jsp

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	jspparser "github.com/yaklang/yaklang/common/yak/java/jsp/parser"
)

func GetAST(code string) (*jspparser.JSPParser, error) {
	var ins any = antlr4util.GetASTParser(code, jspparser.NewJSPLexer, jspparser.NewJSPParser)
	switch ret := ins.(type) {
	case *jspparser.JSPParser:
		return ret, nil
	}
	return nil, utils.Errorf("failed to get jsp.AST, got: %T", ins)
}
