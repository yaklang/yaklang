package ssa

func (p *Program) show(flag FunctionAsmFlag) {
	for _, pkg := range p.Packages {
		for _, i := range pkg.funcs {
			i.DisAsm(flag)
		}
	}
}

func (p *Program) Show() {
	p.show(DisAsmWithoutSource)
}

func (p *Program) ShowWithSource() {
	p.show(DisAsmDefault)
}
