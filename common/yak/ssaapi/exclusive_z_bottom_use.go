package ssaapi

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"sort"
)

func (v *Value) GetBottomUses() Values {
	return v.getBottomUses(nil)
}

func (v *Value) visitUserFallback(actx *AnalyzeContext) Values {
	var vals Values
	for _, user := range v.node.GetUsers() {
		if ret := NewValue(user).AppendDependOn(v).getBottomUses(actx); len(ret) > 0 {
			vals = append(vals, ret...)
		}
	}
	if len(vals) <= 0 {
		return Values{v}
	}
	return vals
}

func (v *Value) getBottomUses(actx *AnalyzeContext, opt ...OperationOption) Values {
	if actx == nil {
		actx = NewAnalyzeContext(opt...)
	}

	actx.depth++
	defer func() {
		actx.depth--
	}()
	v.SetDepth(actx.depth)
	if actx.config.MaxDepth > 0 && actx.depth > actx.config.MaxDepth {
		return Values{}
	}
	if actx.config.MinDepth < 0 && actx.depth < actx.config.MinDepth {
		return Values{}
	}

	if actx.config.HookEveryNode != nil {
		err := actx.config.HookEveryNode(v)
		if err != nil {
			log.Errorf("hook every node failed: %v", err)
		}
	}

	switch ins := v.node.(type) {
	case *ssa.Phi:
		// enter function via phi
		if !actx.ThePhiShouldBeVisited(v) {
			// the phi is existed, visited in the same stack.
			return Values{}
		}
		actx.VisitPhi(v)
		return v.visitUserFallback(actx)
	case *ssa.Call:
		if !actx.TheCallShouldBeVisited(v) {
			// call existed
			return v.visitUserFallback(actx)
		}

		if ins.Method == nil {
			log.Warnf("fallback: (callStack is not clean!) unknown caller: %v", ins.Method)
			return v.visitUserFallback(actx)
		}

		// enter function via call
		f, ok := ssa.ToFunction(ins.Method)
		if !ok {
			log.Warnf("fallback: (callStack is not clean!) unknown function(not valid func): %v", ins.Method)
			return v.visitUserFallback(actx)
		}

		// push call
		err := actx.PushCall(v)
		if err != nil {
			log.Errorf("push call error: %v", err)
		} else {
			defer actx.PopCall()
		}

		funcValue := NewValue(f).AppendDependOn(v)

		// try to find formal param index from call
		// v is calling instruction
		// funcValue is the function
		existed := map[int]struct{}{}
		v.DependOn.ForEach(func(value *Value) {
			existed[value.GetId()] = struct{}{}
		})
		var formalParamsIndex = make([]int, 0, len(ins.Args))
		for argIndex, targetIndex := range ins.Args {
			if _, ok := existed[targetIndex.GetId()]; ok {
				formalParamsIndex = append(formalParamsIndex, argIndex)
			}
		}
		var params = omap.NewOrderedMap(map[int]*ssa.Parameter{})
		lo.ForEach(f.Param, func(param *ssa.Parameter, index int) {
			for _, i := range formalParamsIndex {
				if index == i {
					params.Set(param.GetId(), param)
				}
			}
		})
		if lo.Max(formalParamsIndex) >= len(f.Param) && len(f.Param) > 0 {
			last, _ := lo.Last(f.Param)
			if last != nil {
				params.Set(last.GetId(), last)
			}
		}

		var vals Values
		if params.Len() > 0 {
			for _, formalParam := range params.Values() {
				rets := NewValue(formalParam).AppendDependOn(funcValue).getBottomUses(actx, opt...)
				vals = append(vals, rets...)
			}
			return vals
		}

		// no formal parameters found!
		// enter return
		for _, retStmt := range f.Return {
			retVals := NewValue(retStmt).AppendDependOn(funcValue)
			vals = append(vals, retVals)
		}
		return vals
	case *ssa.Return:
		// enter function via return
		fallback := func() Values {
			var results Values
			for _, result := range ins.Results {
				results = append(results, NewValue(result).AppendDependOn(v))
			}
			return results
		}
		if actx._callStack.Len() > 0 {
			existed := make(map[int]struct{})
			v.DependOn.ForEach(func(value *Value) {
				existedId := value.GetId()
				existed[existedId] = struct{}{}
			})
			var indexes = make(map[int]struct{})
			for idx, ret := range ins.Results {
				if _, ok := existed[ret.GetId()]; ok {
					indexes[idx] = struct{}{}
				}
			}

			val := actx.GetCurrentCall()
			if val == nil {
				return fallback()
			}
			call := val.node.(*ssa.Call)
			fun, ok := call.Method.(*ssa.Function)
			if !ok {
				log.Warnf("BUG: (call's fun is not clean!) unknown function: %v", v.String())
				return fallback()
			}
			_ = fun //TODO: fun can tell u, which return value is the target

			var vals Values
			if !call.IsObject() || len(indexes) <= 0 {
				// non-unpack
				for _, u := range call.GetUsers() {
					if ret := NewValue(u).AppendDependOn(val).AppendDependOn(v).getBottomUses(actx); len(ret) > 0 {
						vals = append(vals, ret...)
					}
				}

				if len(vals) > 0 {
					return vals
				}
				return NewValue(call).AppendDependOn(v).getBottomUses(actx)
			}

			// handle indexed return to call return
			orderedIndex := lo.Keys(indexes)
			sort.Ints(orderedIndex)
			for _, idx := range orderedIndex {
				indexedReturn, ok := call.GetIndexMember(idx)
				if !ok {
					continue
				}
				if newVals := NewValue(indexedReturn).AppendDependOn(val).AppendDependOn(v).getBottomUses(actx); len(newVals) > 0 {
					vals = append(vals, newVals...)
				}
			}
			if len(vals) > 0 {
				return vals
			}
			return NewValue(call).AppendDependOn(v).getBottomUses(actx)
		}
		return fallback()
	}
	return v.visitUserFallback(actx)
}
