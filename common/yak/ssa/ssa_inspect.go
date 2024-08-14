package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
)

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
			child, ok := ToFunction(f)
			if !ok {
				log.Warnf("function %s is not a ssa.Function", f.GetName())
				continue
			}
			showFunc(child)
		}
	}

	fmt.Println("==============================\npackage:", p.Name, p.ProgramKind)
	for _, i := range p.Funcs {
		showFunc(i)
	}
}

func (p *Program) Show() *Program {
	p.show(DisAsmDefault)
	for _, up := range p.UpStream {
		up.show(DisAsmDefault)
	}
	for _, child := range p.ChildApplication {
		child.show(DisAsmDefault)
	}
	return p
}

func (p *Program) ShowWithSource() {
	p.show(DisAsmWithSource)
}
