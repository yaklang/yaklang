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
	CurrentRange memedit.RangeIf
	Editor       *memedit.MemEditor
}

func NewVisitor() *Visitor {
	return &Visitor{}
}

func (v *Visitor) GetInstructions() Instructions {
	return v.Instructions
}
