package templateLanguage

import "github.com/yaklang/yaklang/common/utils/memedit"

func (t *Visitor) EmitPureText(text string, currentRange ...memedit.RangeIf) {
	inst := newInstruction(OpPureText, currentRange...)
	inst.Text = text
	t.Instructions = append(t.Instructions, inst)
}

func (t *Visitor) EmitOutput(variable string, currentRange ...memedit.RangeIf) {
	inst := newInstruction(OpOutput, currentRange...)
	inst.Text = variable
	t.Instructions = append(t.Instructions, inst)
}

func (t *Visitor) EmitEscapeOutput(variable string, currentRange ...memedit.RangeIf) {
	inst := newInstruction(OpEscapeOutput, currentRange...)
	inst.Text = variable
	t.Instructions = append(t.Instructions, inst)
}
