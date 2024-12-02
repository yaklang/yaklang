package jsp

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	tl "github.com/yaklang/yaklang/common/yak/templateLanguage"
)

var Creator tl.VisitorCreator = (*VisitorCreator)(nil)

type VisitorCreator struct {
}

func (b *VisitorCreator) Create(editor *memedit.MemEditor) (tl.TemplateVisitor, error) {
	visitor := NewJSPVisitor()
	visitor.Editor = editor
	ast, err := GetAST(editor.GetSourceCode())
	if err != nil {
		return nil, utils.Errorf("failed to get jsp.AST, got: %T", err)
	}
	visitor.VisitJspDocuments(ast.JspDocuments())
	return visitor, nil
}
