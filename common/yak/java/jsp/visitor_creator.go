package jsp

import (
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

// TODO: This parser is on the real template2java path for JSP. If profiling
// later shows JSP mixed static-content/template parsing has the same kind of
// ANTLR decision explosion as PHP, consider adding a JSP-specific token-source
// coalescing optimization here instead of treating it as test-only code.
func Front(code string) (jspparser.IJspDocumentsContext, error) {
	ast, err := antlr4util.ParseASTWithSLLFirst(
		code,
		jspparser.NewJSPLexer,
		jspparser.NewJSPParser,
		nil,
		nil,
		func(parser *jspparser.JSPParser) jspparser.IJspDocumentsContext {
			return parser.JspDocuments()
		},
	)
	if err != nil {
		return nil, utils.Errorf("parse AST FrontEnd error: %v", err)
	}
	return ast, nil
}
