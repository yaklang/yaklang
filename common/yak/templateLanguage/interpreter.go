package templateLanguage

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

type Interpreter struct {
	Instructions  Instructions
	template      TemplateRender
	idx           int
	templateTyp   TemplateTyp
	generatedCode string
	// for fix  range
	rangeMap  map[int]*memedit.Range // generated code line -> template range
	startLine int
}

func NewInterpreter(instructions Instructions) *Interpreter {
	return &Interpreter{
		Instructions: instructions,
		rangeMap:     map[int]*memedit.Range{},
	}
}

func (i *Interpreter) GetGeneratedCode() string {
	if i == nil {
		return ""
	}
	return i.generatedCode
}

func (i *Interpreter) SetTemplate(template TemplateRender) {
	if i == nil {
		return
	}
	i.template = template
}

func (i *Interpreter) GetCurrentLine() int {
	if i == nil || i.template == nil {
		return 0
	}
	s := i.template.String()
	return len(strings.Split(s, "\n"))
}

func (i *Interpreter) GetTemplate() TemplateRender {
	return i.template
}

func (i *Interpreter) GetRangeMap() map[int]*memedit.Range {
	return i.rangeMap
}

func (i *Interpreter) SetRangeMap() {
	if i == nil {
		return
	}
	currentLine := i.GetCurrentLine()
	ins := i.Instructions[i.idx]
	for j := i.startLine; j <= currentLine; j++ {
		i.rangeMap[j] = ins.Range
	}
}

func (i *Interpreter) SetStartLine() {
	i.startLine = i.GetCurrentLine()
}

func (i *Interpreter) GenerateCode() (err error) {
	if i == nil || i.GetTemplate() == nil {
		return utils.Errorf("interpreter or template is nil")
	}
	defer func() {
		if rec := recover(); err != nil {
			err = utils.Errorf("failed to generate code, got: %v", rec)
			return
		}
	}()
	for {
		if i.idx >= len(i.Instructions) {
			break
		}

		i.SetStartLine()
		ins := i.Instructions[i.idx]
		switch ins.Opcode {
		case OpPureText:
			i.template.WritePureText(ins.Text)
		case OpOutput:
			i.template.WriteOutput(ins.Text)
		case OpEscapeOutput:
			i.template.WriteEscapeOutput(ins.Text)
		case OpPureCode:
			i.template.WritePureCode(ins.Text)
		case OpImport:
			i.template.WriteImport(ins.Text)
		case OpDeclarationCode:
			i.template.WriteDeclaration(ins.Text)
		default:
			return utils.Errorf("unknown opcode: %v", ins.Opcode)
		}
		i.SetRangeMap()
		i.idx++
	}
	i.template.Finish()
	i.generatedCode = i.template.String()
	return nil
}
