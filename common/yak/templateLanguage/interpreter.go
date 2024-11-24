package templateLanguage

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"strings"
)

type Interpreter struct {
	Instructions  Instructions
	template      TemplateRender
	idx           int
	templateTyp   TemplateTyp
	generatedCode string
	// for fix java range
	rangeMap  map[int]memedit.RangeIf // java code line -> template range
	startLine int
}

func NewInterpreter(instructions Instructions) *Interpreter {
	return &Interpreter{
		Instructions: instructions,
		rangeMap:     map[int]memedit.RangeIf{},
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

func (i *Interpreter) GetRangeMap() map[int]memedit.RangeIf {
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

func (i *Interpreter) GenerateCode() {
	if i == nil || i.GetTemplate() == nil {
		log.Errorf("interpreter or template is nil")
		return
	}
	defer func() {
		if err := recover(); err != nil {
			ret := utils.Errorf("generate code panic: %v", err)
			log.Infof("%+v", ret)
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
		}
		i.SetRangeMap()
		i.idx++
	}
	i.template.Finish()
	i.generatedCode = i.template.String()
}
