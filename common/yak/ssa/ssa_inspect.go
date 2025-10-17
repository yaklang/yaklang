package ssa

import (
	"fmt"
)

func (p *Program) show(flag FunctionAsmFlag) {

	var showFunc func(i *Function)
	showFunc = func(f *Function) {
		fmt.Println(f.DisAsm(flag))
		fmt.Println("extern type:")
		// for name, typ := range i.externType {
		// 	fmt.Printf("%s: %s\n", name, typ.RawString())
		// }
		// fmt.Println("extern Value:")
		// for name, v := range i.externInstance {
		// 	fmt.Printf("%s: %s\n", name, v)
		// }

		for _, childID := range f.ChildFuncs {
			childValue, ok := f.GetValueById(childID)
			if !ok {
				log.Warnf("function %d is not a ssa.Value", childID)
				continue
			}
			childFunc, ok := ToFunction(childValue)
			if !ok {
				log.Warnf("function %s is not a ssa.Function", childValue.GetName())
				continue
			}
			showFunc(childFunc)
		}
	}

	fmt.Println("==============================\npackage:", p.Name, p.ProgramKind)
	p.Funcs.ForEach(func(i string, v *Function) bool {
		showFunc(v)
		return true
	})
}

func (p *Program) Show() *Program {
	p.show(DisAsmDefault)
	p.UpStream.ForEach(func(i string, v *Program) bool {
		v.show(DisAsmDefault)
		return true
	})

	return p
}

func (p *Program) ShowWithSource() {
	p.show(DisAsmWithSource)
}
