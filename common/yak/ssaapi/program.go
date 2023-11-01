package ssaapi

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type Program struct {
	*ssa.Program
}

func NewProgram(prog *ssa.Program) *Program {
	return &Program{
		Program: prog,
	}
}

func (p *Program) Ref(name string) *Values {
	ret := make([]ssa.Node, 0)
	lo.ForEach(p.Packages, func(pkg *ssa.Package, index int) {
		lo.ForEach(pkg.Funcs, func(fun *ssa.Function, index int) {
			ret = lo.Uniq(append(ret, fun.GetValuesByName(name)...))
		})
	})
	return NewValue(ret)
}
