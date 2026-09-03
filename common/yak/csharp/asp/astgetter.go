package asp

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	aspparser "github.com/yaklang/yaklang/common/yak/csharp/asp/parser"
)

func Front(code string) (aspparser.IAspDocumentsContext, error) {
	ast, err := antlr4util.ParseASTWithSLLFirst(
		code,
		aspparser.NewASPLexer,
		aspparser.NewASPParser,
		nil,
		nil,
		func(parser *aspparser.ASPParser) aspparser.IAspDocumentsContext {
			return parser.AspDocuments()
		},
	)
	if err != nil {
		return nil, utils.Errorf("parse ASP AST FrontEnd error: %v", err)
	}
	return ast, nil
}
