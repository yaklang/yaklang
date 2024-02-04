package ssa

import "fmt"

func (p *Program) show(flag FunctionAsmFlag) {

	var showFunc func(i *Function)
	showFunc = func(i *Function) {
		fmt.Println(i.DisAsm(flag))
		fmt.Println("extern type:")
		// for name, typ := range i.externType {
		// 	fmt.Printf("%s: %s\n", name, typ.RawString())
		// }
		// fmt.Println("extern Value:")
		// for name, v := range i.externInstance {
		// 	fmt.Printf("%s: %s\n", name, v)
		// }

		for _, f := range i.ChildFuncs {
			showFunc(f)
		}
	}

	for _, pkg := range p.Packages {
		for _, i := range pkg.Funcs {
			showFunc(i)
		}
	}
}

func (p *Program) Show() *Program {
	p.show(DisAsmDefault)
	return p
}

func (p *Program) ShowWithSource() {
	p.show(DisAsmWithSource)
}
