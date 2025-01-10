package jsp

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	jspparser "github.com/yaklang/yaklang/common/yak/java/jsp/parser"
	tl "github.com/yaklang/yaklang/common/yak/templateLanguage"
)

var Creator tl.VisitorCreator = (*VisitorCreator)(nil)

type VisitorCreator struct {
}

func (b *VisitorCreator) Create(editor *memedit.MemEditor) (tl.TemplateVisitor, error) {
	visitor := NewJSPVisitor()
	visitor.Editor = editor
	ast, err := Front(editor.GetSourceCode())
	if err != nil {
		return nil, utils.Errorf("failed to get jsp.AST, got: %T", err)
	}
	visitor.VisitJspDocuments(ast)
	return visitor, nil
}

func Front(code string) (jspparser.IJspDocumentsContext, error) {
	errListener := antlr4util.NewErrorListener()
	lexer := jspparser.NewJSPLexer(antlr.NewInputStream(code))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := jspparser.NewJSPParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	ast := parser.JspDocuments()
	if len(errListener.GetErrors()) == 0 {
		return ast, nil
	}
	return nil, utils.Errorf("parse AST FrontEnd error: %v", errListener.GetErrorString())
}
