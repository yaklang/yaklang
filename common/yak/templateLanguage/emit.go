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

type IfBuilder struct {
	items     []*conditionItem
	elseBlock string
}

type conditionItem struct {
	condition string
	block     string
}

func (ib *IfBuilder) SetIfCondition(condition, block string) {
	ib.items = append(ib.items, &conditionItem{condition, block})
}

func (ib *IfBuilder) SetElse(block string) {
	ib.elseBlock = block
}

func (y *Visitor) NewIfBuilder() *IfBuilder {
	return &IfBuilder{}
}

func (y *Visitor) EmitIfStatement(builder *IfBuilder) {
	inst := newInstruction(OpIfStmt, y.CurrentRange)
	inst.ifBuilder = builder
	y.Instructions = append(y.Instructions, inst)
}
