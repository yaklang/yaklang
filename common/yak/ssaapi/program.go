package ssaapi

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
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

func (p *Program) IsNil() bool {
	return utils.IsNil(p.Program)
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

	return getValuesWithUpdate(ret)
}

func getValuesWithUpdateSingle(v *Value) Values {
	ret := make(Values, 0)
	ret = append(ret, v)
	// check if: a[0] = value.Name; also append a[0]
	v.GetUsers().ForEach(func(user *Value) {
		if user.IsUpdate() && v.Compare(user.GetOperand(1)) {
			ret = append(ret, getValuesWithUpdateSingle(user.GetOperand(0))...)
		}
	})
	return ret
}

func getValuesWithUpdate(vs Values) Values {
	ret := make(Values, 0, len(vs))
	// copy(ret, vs)

	vs.ForEach(func(v *Value) {
		ret = append(ret, getValuesWithUpdateSingle(v)...)
	})

	return ret
}
