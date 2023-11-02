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

func (p *Program) Ref(name string) Values {
	ret := make(Values, 0)
	tmp := make(map[*Value]struct{})
	lo.ForEach(p.Packages, func(pkg *ssa.Package, index int) {
		lo.ForEach(pkg.Funcs, func(fun *ssa.Function, index int) {
			for _, v := range fun.GetValuesByName(name) {
				value := NewValue(v)
				if _, ok := tmp[value]; !ok {
					ret = append(ret, value)
					tmp[value] = struct{}{}
				}
			}
			// ret = lo.Uniq(append(ret, fun.GetValuesByName(name)...))
		})
	})
	return ret
}
