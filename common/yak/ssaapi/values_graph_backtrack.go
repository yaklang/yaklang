package ssaapi

import (
	"bytes"
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/cartesian"
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
		return value.GetDependOn()
	}, h)
}

// RecursiveEffects is used to get all the effects of the value
func (v *Value) RecursiveEffects(h func(value *Value) error) {
	visited := make(map[*Value]struct{})
	v.recursive(visited, func(value *Value) []*Value {
		return value.GetEffectOn()
	}, h)
}

func (v *Value) RecursiveDependsAndEffects(h func(value *Value) error) {
	visited := make(map[*Value]struct{})
	v.recursive(visited, func(value *Value) []*Value {
		return append(value.GetDependOn(), value.GetEffectOn()...)
	}, h)
}

// FindStrictCommonDepends 在给定的值集合中查找具有严格共同依赖的值。
//
// FindStrictCommonDepends searches for values with strictly common dependencies
// in the given collection of values.
//
// 它遍历给定的值集合，比较每对值之间的依赖关系，并返回所有具有严格共同依赖的值的集合。
//
// It iterates over the given collection of values, compares the dependencies
// between each pair of values, and returns a collection of all values with strictly
// common dependencies.
//
// 严格共同依赖是指只有当值 A 依赖于值 B，而值 B 也依赖于值 A 时，这两个值才被认为具有严格共同依赖。
//
// Strict common dependencies refer to the scenario where value A depends on
// value B, and value B depends on value A for them to be considered to have
// strict common dependencies.
func FindStrictCommonDepends(val Values) Values {
	for _, v := range val {
		if (v.DependOn == nil || v.DependOn.Count() == 0) &&
			(v.EffectOn == nil || v.EffectOn.Count() == 0) {
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

// FindFlexibleCommonDepends 在给定的值集合中查找具有灵活共同依赖的值。
//
// FindFlexibleCommonDepends searches for values with flexible common dependencies
// in the given collection of values.
//
// 它与 FindStrictCommonDepends 类似，但在查找共同依赖时，会尝试重新构建值的顶层定义。
//
// It is similar to FindStrictCommonDepends, but when searching for common dependencies,
// it attempts to rebuild the top-level definition of values.
//
// 具有灵活共同依赖的值是指通过重新构建顶层定义，从而可能包括更多的依赖关系。
//
// Values with flexible common dependencies are those that may include more
// dependencies by rebuilding the top-level definition.
func FindFlexibleCommonDepends(val Values) Values {
	for _, v := range val {
		if (v.DependOn == nil || v.DependOn.Count() == 0) &&
			(v.EffectOn == nil || v.EffectOn.Count() == 0) {
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

		// rebuild the top defs
		from.GetTopDefs()
		from.RecursiveDepends(func(value *Value) error {
			if value.GetId() == to.GetId() {
				common = append(common, value)
			}
			return nil
		})
	}

	var retValues = make(Values, 0, len(common))
	var r = map[int64]struct{}{}
	for _, i := range common {
		if _, ok := r[i.GetId()]; ok {
			continue
		}
		r[i.GetId()] = struct{}{}
		if i != nil {
			retValues = append(retValues, i)
		}
	}
	return retValues
}

func (v *Value) Backtrack() *omap.OrderedMap[string, *Value] {
	ret := omap.NewOrderedMap[string, *Value](map[string]*Value{})
	var vals = utils.NewStack[*Value]()
	var count = 1
	var current = v
	vals.Push(v)
	visited := make(map[int64]bool)
	for current != nil {
		deps := current.DependOn
		var p *Value
		if deps != nil && deps.Count() > 0 {
			deps.ForEach(func(key string, result *Value) bool {
				if _, ok := visited[result.GetId()]; !ok {
					visited[result.GetId()] = true
					p = result
					return false // break
				}
				return true // continue
			})
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
		log.Infof(buf.String())
		return
	}

	// for index, track := range om.Values() {
	// 	if track == nil {
	// 		continue
	// 	}
	// 	indent := strings.Repeat(" ", index*2) + fmt.Sprintf("[depth:%2d]->", track.GetDepth())
	// 	buf.WriteString(indent + track.String() + "\n")
	// }
	log.Infof(buf.String())
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
