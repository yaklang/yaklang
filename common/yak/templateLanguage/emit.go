package templateLanguage

func (y *Visitor) EmitPureText(text string) {
	inst := newInstruction(OpPureText, y.CurrentRange)
	inst.Text = text
	y.Instructions = append(y.Instructions, inst)
}

func (y *Visitor) EmitOutput(variable string) {
	inst := newInstruction(OpOutput, y.CurrentRange)
	inst.Text = variable
	y.Instructions = append(y.Instructions, inst)
}

func (y *Visitor) EmitEscapeOutput(variable string) {
	inst := newInstruction(OpEscapeOutput, y.CurrentRange)
	inst.Text = variable
	y.Instructions = append(y.Instructions, inst)
}

func (y *Visitor) EmitPureOutput(expression string) {
	inst := newInstruction(OpPureOutPut, y.CurrentRange)
	inst.Text = expression
	y.Instructions = append(y.Instructions, inst)
}

func (y *Visitor) EmitPureCode(code string) {
	inst := newInstruction(OpPureCode, y.CurrentRange)
	inst.Text = code
	y.Instructions = append(y.Instructions, inst)
}

func (y *Visitor) EmitImport(path string) {
	inst := newInstruction(OpImport, y.CurrentRange)
	inst.Text = path
	y.Instructions = append(y.Instructions, inst)
}
