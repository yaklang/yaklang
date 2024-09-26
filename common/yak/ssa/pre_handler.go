package ssa

func (b *FunctionBuilder) PreHandler() bool {
	return b.GetProgram().GetApplication().PreHandler()
}

func (p *Program) PreHandler() bool {
	return p.GetApplication()._preHandler
}

func (p *Program) SetPreHandler(b bool) {
	p.GetApplication()._preHandler = b
}
