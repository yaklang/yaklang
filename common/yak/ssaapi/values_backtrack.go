package ssaapi

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/cartesian"
	"strings"
)

func (v *Value) recursive(visited map[*Value]struct{}, itemGetter func(value *Value) []*Value, h func(*Value) error) {
	if _, ok := visited[v]; ok {
		return
	}
	visited[v] = struct{}{}
	if err := h(v); err != nil {
		log.Warn(err)
		return
	}
	for _, dep := range itemGetter(v) {
		dep.recursive(visited, itemGetter, h)
	}
}

// RecursiveDepends is used to get all the dependencies of the value
func (v *Value) RecursiveDepends(h func(value *Value) error) {
	visited := make(map[*Value]struct{})
	v.recursive(visited, func(value *Value) []*Value {
		return value.DependOn
	}, h)
}

// RecursiveEffects is used to get all the effects of the value
func (v *Value) RecursiveEffects(h func(value *Value) error) {
	visited := make(map[*Value]struct{})
	v.recursive(visited, func(value *Value) []*Value {
		return value.EffectOn
	}, h)
}

func (v *Value) RecursiveDependsAndEffects(h func(value *Value) error) {
	visited := make(map[*Value]struct{})
	v.recursive(visited, func(value *Value) []*Value {
		return append(value.DependOn, value.EffectOn...)
	}, h)
}

func FindStrictCommonDepends(val Values) Values {
	for _, v := range val {
		if len(v.DependOn) == 0 && len(v.EffectOn) == 0 {
			v.GetTopDefs()
		}
	}
	results, err := cartesian.Product([][]*Value{val, val})
	if err != nil {
		log.Warn(err)
		return nil
	}
	var common Values
	for _, fromTo := range results {
		from := fromTo[0]
		to := fromTo[1]

		if from.GetId() == to.GetId() {
			continue
		}

		from.RecursiveDepends(func(value *Value) error {
			if value.GetId() == to.GetId() {
				common = append(common, value)
			}
			return nil
		})
	}
	return common
}

func FindFlexibleCommonDepends(val Values) Values {
	for _, v := range val {
		if len(v.DependOn) == 0 && len(v.EffectOn) == 0 {
			v.GetTopDefs()
		}
	}
	results, err := cartesian.Product([][]*Value{val, val})
	if err != nil {
		log.Warn(err)
		return nil
	}
	var common Values
	for _, fromTo := range results {
		from := fromTo[0]
		to := fromTo[1]
		// rebuild the top defs
		from.GetTopDefs()
		from.RecursiveDepends(func(value *Value) error {
			if value.GetId() == to.GetId() {
				common = append(common, value)
			}
			return nil
		})
	}
	return common
}

func (v *Value) Backtrack() *omap.OrderedMap[string, *Value] {
	ret := omap.NewOrderedMap[string, *Value](map[string]*Value{})
	var vals = utils.NewStack[*Value]()
	var count = 1
	var current = v
	vals.Push(v)
	visited := make(map[int]bool)
	for current != nil {
		deps := current.DependOn
		var p *Value
		if len(deps) > 0 {
			for _, result := range deps {
				if _, ok := visited[result.GetId()]; !ok {
					visited[result.GetId()] = true
					p = result
					break
				}
			}
		} else {
			break
		}
		count++
		vals.Push(p)
		current = p
	}
	for i := 0; i < count; i++ {
		err := ret.Push(vals.Pop())
		if err != nil {
			log.Warn(err)
		}
	}
	return ret
}

func (v *Value) ShowBacktrack() {
	var buf bytes.Buffer
	om := v.Backtrack()
	buf.WriteString("===================== Backtrack from [t" + fmt.Sprint(v.GetId()) + "]`" + v.String() + "` =====================: \n\n")
	if om == nil || om.Len() <= 0 {
		buf.WriteString("empty parent\n")
		fmt.Println(buf.String())
		return
	}

	for index, track := range om.Values() {
		if track == nil {
			continue
		}
		indent := strings.Repeat(" ", index*2) + fmt.Sprintf("[depth:%2d]->", track.GetDepth())
		buf.WriteString(indent + track.String() + "\n")
	}
	fmt.Println(buf.String())
}

// FlexibleDepends is used to get all the dependencies of the value
// e.g:  a = b + c; d = a + e; the e is not filled in the depends of a, but call FlexibleDepends will get it
func (v *Value) FlexibleDepends() *Value {
	v.GetBottomUses()
	v.GetTopDefs()
	v.RecursiveEffects(func(value *Value) error {
		value.GetTopDefs()
		return nil
	})
	return v
}

func (v Values) FlexibleDepends() Values {
	for _, val := range v {
		val.FlexibleDepends()
	}
	return v
}

func (V Values) ShowDot() Values {
	for _, v := range V {
		v.ShowDot()
	}
	return V
}
