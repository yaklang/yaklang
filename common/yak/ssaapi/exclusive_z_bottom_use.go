package ssaapi

import (
	"sort"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (v *Value) GetBottomUses(opt ...OperationOption) Values {
	actx := NewAnalyzeContext(opt...)
	actx.Self = v
	ret := v.getBottomUses(actx, opt...)
	return MergeValues(ret)
}

func (v Values) GetBottomUses(opts ...OperationOption) Values {
	ret := make(Values, 0)
	for _, sub := range v {
		ret = append(ret, sub.GetBottomUses(opts...)...)
	}
	return MergeValues(ret)
}

func (v *Value) visitUserFallback(actx *AnalyzeContext, opt ...OperationOption) Values {
	var vals Values
	v.GetUsers().ForEach(func(value *Value) {
		if ret := value.AppendDependOn(v).getBottomUses(actx, opt...); len(ret) > 0 {
			vals = append(vals, ret...)
		}
	})

	// member.IsUndefined()
	undefineMember := false
	if un, ok := ssa.ToUndefined(v.node); ok {
		if un.Kind == ssa.UndefinedMemberInValid || un.Kind == ssa.UndefinedMemberValid {
			undefineMember = true
		}
	}
	if v.IsMember() && !undefineMember {
		obj := v.GetObject()
		if err := actx.pushObject(obj, v.GetKey(), v); err != nil {
			log.Errorf("%v", err)
			return v.visitedDefs(actx, opt...)
		}
		vals = append(vals, obj.getBottomUses(actx, opt...)...)
		actx.popObject()
	}
	if len(vals) <= 0 {
		return Values{v}
	}
	return vals
}

func (v *Value) getBottomUses(actx *AnalyzeContext, opt ...OperationOption) Values {
	if v == nil {
		return nil
	}
	if actx == nil {
		actx = NewAnalyzeContext(opt...)
	}
	actx.depth++
	defer func() {
		actx.depth--
	}()

	if ins, ok := ssa.ToLazyInstruction(v.node); ok {
		v.node, ok = ins.Self().(ssa.Value)
		if !ok {
			log.Debugf("BUG: (lazy instruction) unknown instruction: %v - %v - ID:%v", v.String(), v.GetVerboseName(), v.GetId())
			return Values{}
		}
		return v.getBottomUses(actx, opt...)
	}

	shouldExit, recoverStack := actx.check(v)
	defer recoverStack()
	if shouldExit {
		return Values{v}
	}

	v.SetDepth(actx.depth)
	err := actx.hook(v)
	if err != nil {
		return Values{v}
	}
	switch ins := v.node.(type) {
	case *ssa.Phi:
		return v.visitUserFallback(actx, opt...)
	case *ssa.Call:
		if ins.Method == nil {
			// log.Infof("fallback: (call instruction 's method/func is not *Function) unknown caller, got: %v", ins.Method.String())
			return v.visitUserFallback(actx, opt...)
		}
		// enter function via call
		f, ok := ssa.ToFunction(ins.Method)
		if !ok {
			//log.Infof("fallback: (call instruction 's method/func is not *Function) unknown caller, got: %v", ins.Method.String())
			return v.visitUserFallback(actx, opt...)
		}
		funcValue := v.NewBottomUseValue(f)
		if ValueCompare(funcValue, actx.Self) {
			return v.visitUserFallback(actx, opt...)
		}
		if actx.haveCrossProcess(funcValue) {
			return v.visitUserFallback(actx, opt...)
		} else {
			actx.setCauseValue(v)
		}
		//try to find formal param index from call
		//v is calling instruction
		//funcValue is the function
		getCalledFormalParams := func(f *ssa.Function) Values {
			existed := map[int64]struct{}{}
			v.DependOn.ForEach(func(value *Value) {
				existed[value.GetId()] = struct{}{}
			})

			var formalParamsIndex = make([]int, 0, len(ins.Args))
			for argIndex, targetIndex := range ins.Args {
				if _, ok := existed[targetIndex.GetId()]; ok {
					formalParamsIndex = append(formalParamsIndex, argIndex)
				}
			}
			var params = omap.NewOrderedMap(map[int64]ssa.Value{})
			lo.ForEach(f.Params, func(param ssa.Value, index int) {
				for _, i := range formalParamsIndex {
					if index == i {
						params.Set(param.GetId(), param)
					}
				}
			})
			if lo.Max(formalParamsIndex) >= len(f.Params) && len(f.Params) > 0 {
				last, _ := lo.Last(f.Params)
				if last != nil {
					params.Set(last.GetId(), last)
				}
			}

			var formalParams Values
			if params.Len() > 0 {
				for _, formalParam := range params.Values() {
					formalParams = append(formalParams, funcValue.NewBottomUseValue(formalParam))
				}
				return formalParams
			}
			return nil
		}
		formalParams := getCalledFormalParams(f)
		if formalParams != nil {
			var vals Values
			formalParams.ForEach(func(actualParam *Value) {
				rets := actualParam.getBottomUses(actx, opt...)
				vals = append(vals, rets...)
			})
			if len(vals) > 0 {
				return vals
			}
		}

		// no formal parameters found!
		// enter return
		var vals Values
		for _, retStmt := range f.Return {
			retVals := funcValue.NewBottomUseValue(retStmt).getBottomUses(actx, opt...)
			vals = append(vals, retVals...)
		}
		return vals
	case *ssa.Return:
		// enter function via return
		fallback := func() Values {
			// var results Values
			results := make(Values, 0)
			if f := ins.GetFunc(); f != nil {
				v.NewBottomUseValue(f).GetCalledBy().ForEach(func(value *Value) {
					dep := value.AppendDependOn(v)
					if !actx.haveCrossProcess(dep) {
						actx.setCauseValue(dep)
					}
					results = append(results, dep.getBottomUses(actx, opt...)...)
				})
			}
			if len(results) > 0 {
				return results
			}
			for _, result := range ins.Results {
				results = append(results, v.NewBottomUseValue(result))
			}
			return results
		}
		// if actx.IsInPositiveStack() {
		existed := make(map[int64]struct{})
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

		currentCallValue := actx.getLastCauseValue()
		if currentCallValue == nil {
			return fallback()
		}
		call, ok := ssa.ToCall(currentCallValue.node)
		if !ok {
			log.Warnf("BUG: (call's fun is not clean!) call stack's value is not call: %v", currentCallValue.String())
			return fallback()
		}
		fun, ok := ssa.ToFunction(call.Method)
		if !ok {
			log.Warnf("BUG: (call's fun is not clean!) unknown function: %v", v.String())
			return fallback()
		}
		_ = fun //TODO: fun can tell u, which return value is the target

		var vals Values
		if !call.IsObject() || len(indexes) <= 0 {
			currentCallValue.GetUsers().ForEach(func(user *Value) {
				if ret := user.AppendDependOn(currentCallValue).getBottomUses(actx); len(ret) > 0 {
					vals = append(vals, ret...)
				}
			})

			if len(vals) > 0 {
				return vals
			}
			return v.NewBottomUseValue(call).getBottomUses(actx, opt...)
		}

		// handle indexed return to call return
		orderedIndex := lo.Keys(indexes)
		sort.Ints(orderedIndex)
		for _, idx := range orderedIndex {
			indexedReturn, ok := call.GetIndexMember(idx)
			if !ok {
				continue
			}

			returnReceiver := v.NewBottomUseValue(indexedReturn)
			actx.pushObject(currentCallValue, returnReceiver.GetKey(), returnReceiver)
			if newVals := returnReceiver.getBottomUses(actx); len(newVals) > 0 {
				vals = append(vals, newVals...)
			}
			actx.popObject()
		}
		if len(vals) > 0 {
			return vals
		}
		return v.NewBottomUseValue(call).getBottomUses(actx, opt...)
		// }
	case *ssa.Function:

	}
	return v.visitUserFallback(actx, opt...)
}
