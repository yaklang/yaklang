package ssaapi

import (
	"github.com/yaklang/yaklang/common/utils"
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
	if v.IsObject() {
		exist := false
		actx.foreachObjectStack(func(obj *Value, key *Value, val *Value) bool {
			if obj.GetId() == v.GetId() {
				exist = true
				return false
			}
			return true
		})
		if !exist {
			v.GetAllMember().ForEach(func(value *Value) {
				_ = actx.pushObject(v, value.GetKey(), value)
				vals = append(vals, value.AppendDependOn(v).getBottomUses(actx, opt...)...)
				actx.popObject()
			})
		}
	}
	if v.IsMember() {
		currentObject := v.GetObject()
		currentKey := v.GetKey()
		exist := false
		actx.foreachObjectStack(func(obj *Value, key *Value, value *Value) bool {
			if currentObject.GetId() == obj.GetId() && key.GetId() == currentKey.GetId() {
				exist = true
				return false
			}
			return true
		})
		if !exist {
			_ = actx.pushObject(currentObject, currentKey, v)
			vals = append(vals, currentObject.AppendDependOn(v).getBottomUses(actx, opt...)...)
			actx.popObject()
		}
	}
	v.GetUsers().ForEach(func(value *Value) {
		if ret := value.AppendDependOn(v).getBottomUses(actx, opt...); len(ret) > 0 {
			vals = append(vals, ret...)
		}
	})
	if vals.Len() == 0 {
		return Values{v}
	}
	return vals
}

func (v *Value) getBottomUses(actx *AnalyzeContext, opt ...OperationOption) (result Values) {
	//defer func() {
	//	for _, ret := range result {
	//		if ret.GetEffectOn() != nil {
	//			log.Errorf("BUG:(bottom-use's result is not a tree node,%s have depend on %s", ret.String(), ret.GetDependOn().String())
	//		}
	//	}
	//}()

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

	if ins, ok := ssa.ToLazyInstruction(v.innerValue); ok {
		v.innerValue, ok = ins.Self().(ssa.Value)
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
	switch inst := v.GetSSAInst().(type) {
	case *ssa.Phi:
		return v.visitUserFallback(actx, opt...)
	case *ssa.Call:
		method := inst.GetValueById(inst.Method)
		if method == nil {
			log.Infof("fallback: (call instruction 's method/func is not *Function) unknown caller, ")
			return v.visitUserFallback(actx, opt...)
		}
		actx.pushCall(inst)
		//分析的当前值相同，说明进来就是当前值
		if ValueCompare(v, actx.Self) {
			log.Debugf("value analysis: (call instruction) caller is self")
			return v.visitUserFallback(actx, opt...)
		}
		existed := map[int64]struct{}{}
		var vals Values
		v.DependOn.ForEach(func(value *Value) {
			existed[value.GetId()] = struct{}{}
		})
		checkVal := func(vs []int64, get func(index int, arg int64)) {
			for index, value := range vs {
				get(index, value)
			}
		}
		var getRealMethod func(ssa.Value, int) ssa.Value
		getRealMethod = func(method ssa.Value, callIndex int) ssa.Value {
			if _, isFunction := ssa.ToFunction(method); isFunction {
				return method
			}
			_, isparam := ssa.ToParameter(method)
			_, isParameterMember := ssa.ToParameterMember(method)
			if !(isParameterMember || isparam) {
				return method
			}
			methodId := method.GetId()
			call := actx.peekCall(callIndex)
			if utils.IsNil(call) {
				return method
			}
			function := call.GetValueById(call.Method)
			toFunction, isFunction := ssa.ToFunction(function)
			if !isFunction {
				return method
			}
			var val int64
			checkVal(toFunction.Params, func(index int, arg int64) {
				if index >= len(call.Args) {
					return
				}
				if arg == methodId {
					val = call.Args[index]
				}
			})
			checkVal(toFunction.ParameterMembers, func(index int, arg int64) {
				if index >= len(call.ArgMember) {
					return
				}
				if arg == methodId {
					val = call.ArgMember[index]
				}
			})
			if val <= 0 {
				return method
			}
			return getRealMethod(call.GetValueById(val), callIndex+1)
		}
		real := getRealMethod(method, 1)

		fun, isFunc := ssa.ToFunction(real)
		if !isFunc && method.GetReference() != nil {
			fun, isFunc = ssa.ToFunction(method.GetReference())
		}

		if isFunc {
			checkVal(inst.Args, func(index int, arg int64) {
				if index >= len(fun.Params) {
					return
				}
				_, ok := existed[arg]
				if !ok {
					return
				}
				val := v.NewBottomUseValue(fun.GetValueById(fun.Params[index]))
				vals = append(vals, val.getBottomUses(actx, opt...)...)
			})
			checkVal(inst.ArgMember, func(index int, arg int64) {
				if index >= len(fun.ParameterMembers) {
					return
				}
				_, ok := existed[arg]
				if !ok {
					return
				}
				val := v.NewBottomUseValue(fun.GetValueById(fun.ParameterMembers[index]))
				vals = append(vals, val.getBottomUses(actx, opt...)...)
			})
		}
		if vals.Len() > 0 {
			return vals
		} else {
			return v.visitUserFallback(actx, opt...)
		}

	case *ssa.Return:
		var vals Values
		function := inst.GetFunc()
		if function == nil {
			log.Errorf("BUG: (return instruction 's function is nil)")
			log.Errorf("BUG: (return instruction 's function is nil)")
			log.Errorf("BUG: (return instruction 's function is nil)")
			log.Errorf("BUG: (return instruction 's function is nil)")
			return nil
		}
		call := actx.getLastCauseCall(BottomUseAnalysis)
		if call == nil {
			called := v.NewBottomUseValue(function).GetCalledBy()
			called.ForEach(func(value *Value) {
				vals = append(vals, value.getBottomUses(actx, opt...)...)
			})
			return vals
		}
		exists := make(map[int64]struct{})
		v.DependOn.ForEach(func(value *Value) {
			exists[value.GetId()] = struct{}{}
		})

		getReturnIndex := -1
		for index, result := range inst.Results {
			if _, ok := exists[result]; ok {
				getReturnIndex = index
			}
		}
		if getReturnIndex != -1 {
			member := call.GetMember(v.NewValue(ssa.NewConst(getReturnIndex)))
			if member == nil {
				log.Errorf("BUG: (return instruction 's member is nil),check it")
			} else {
				actx.pushObject(call, member.GetKey(), member)
				vals = append(vals, member.AppendDependOn(v).getBottomUses(actx, opt...)...)
				actx.popObject()
			}
		}
		if vals.Len() == 0 {
			vals = append(vals, v.NewBottomUseValue(call.innerValue).getBottomUses(actx, opt...)...)
		}
		return vals
	}
	return v.visitUserFallback(actx, opt...)
}
