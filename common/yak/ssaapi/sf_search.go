package ssaapi

import (
	"fmt"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type userNodeItems struct {
	names []string
	value []ssa.Value
}

func SearchWithCFG(value *Value, mod int, compare func(string) bool, opt ...sfvm.AnalysisContextOption) Values {
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
	case *ssa.Function:
		addItems([]string{"throws"}, inst.Throws...)
	case *ssa.ErrorHandler:
		addItems([]string{"catch"}, inst.Catch...)
		addItems([]string{"finally"}, inst.Final)
		addItems([]string{"try"}, inst.Try)
		addItems([]string{"final"}, inst.Final)
	case *ssa.ErrorCatch:
		addItems([]string{"body"}, inst.CatchBody)
		addItems([]string{"exception"}, inst.Exception)
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

func SearchWithValue(value *Value, mod int, compare func(string) bool, opt ...sfvm.AnalysisContextOption) Values {
	var newValue Values

	inst := value.innerValue
	if utils.IsNil(inst) {
		return newValue
	}

	add := func(v *Value) {
		v.AppendPredecessor(value, opt...)
		newValue = append(newValue, v)
	}
	check := func(value *Value) bool {
		if compare(value.GetName()) || compare(value.String()) {
			return true
		}

		if value.IsConstInst() && compare(codec.AnyToString(value.GetConstValue())) {
			return true
		}

		for name := range value.GetAllVariables() {
			if compare(name) {
				return true
			}
		}

		if key := value.GetKey(); key != nil {
			keyName := fmt.Sprint(key.GetConstValue())
			if keyName != "" && compare(keyName) {
				return true
			}
		}

		return false
	}
	if mod&ssadb.ConstType != 0 {
		if check(value) {
			add(value)
		}
	}
	if mod&ssadb.NameMatch != 0 {
		// handler self
		if check(value) {
			add(value)
		}
	}
	if mod&ssadb.KeyMatch != 0 {
		if value.IsObject() {
			allMember := inst.GetAllMember()
			for k, v := range allMember {
				if check(value.NewValue(k)) {
					add(value.NewValue(v))
				}
			}

		}

		for _, ov := range inst.FlatOccultation() {
			allMember := ov.GetAllMember()
			for k, v := range allMember {
				if check(value.NewValue(k)) {
					add(value.NewValue(v))
				}
			}
		}
	}
	return newValue
}
