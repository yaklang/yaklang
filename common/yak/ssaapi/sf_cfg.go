package ssaapi

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type userNodeItems struct {
	names []string
	value []ssa.Value
}

func SearchUser(value *Value, mod int, compare func(string) bool, opt ...sfvm.AnalysisContextOption) Values {
	var newValue Values
	inst := value.innerUser
	if utils.IsNil(inst) {
		return newValue
	}

	add := func(vvs ...ssa.Value) {
		for _, vv := range vvs {
			v := value.NewValue(vv)
			v.AppendPredecessor(value, opt...)
			newValue = append(newValue, v)
		}
	}

	items := []*userNodeItems{}

	addItems := func(names []string, value ...ssa.Value) {
		items = append(items, &userNodeItems{
			names: names,
			value: value,
		})
	}

	switch inst := inst.(type) {
	case *ssa.ErrorHandler:
		addItems([]string{"catch"}, inst.Catch...)
		addItems([]string{"finally"}, inst.Final)
		addItems([]string{"try"}, inst.Try)
		addItems([]string{"final"}, inst.Final)
	}

	for _, item := range items {
		for _, name := range item.names {
			if compare(name) {
				add(item.value...)
			}
		}
	}

	return newValue

}
