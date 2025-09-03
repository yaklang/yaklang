package templateLanguage

import "github.com/yaklang/yaklang/common/utils/memedit"

type TemplateVisitor interface {
	GetInstructions() Instructions
}

type VisitorCreator interface {
	Create(editor *memedit.MemEditor) (TemplateVisitor, error)
}

type Visitor struct {
	Instructions Instructions
	CurrentRange *memedit.Range
	Editor       *memedit.MemEditor
}

func NewVisitor() *Visitor {
	return &Visitor{}
}

func (y *Visitor) GetInstructions() Instructions {
	return y.Instructions
}
