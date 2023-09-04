package ssa

import "fmt"

func (p *Program) show(flag FunctionAsmFlag) {
	for _, pkg := range p.Packages {
		for _, i := range pkg.Funcs {
			fmt.Println(i.DisAsm(flag))
		}
	}
}

func (p *Program) Show() {
	p.show(DisAsmDefault)
}

func (p *Program) ShowWithSource() {
	p.show(DisAsmWithSource)
}
