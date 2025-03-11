package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
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

	object, _, _ := actx.getCurrentObject()
	if v.IsObject() && utils.IsNil(object) || object.GetId() != v.GetId() {
		v.GetAllMember().ForEach(func(value *Value) {
			vals = append(vals, value.AppendDependOn(v).getBottomUses(actx, opt...)...)
		})
	}
	if v.IsMember() {
		obj := v.GetObject()
		key := v.GetKey()
		if err := actx.pushObject(obj, key, v); err != nil {
			log.Errorf("BUG: (visitUserFallback) pushObject failed: %v", err)
		} else {
			vals = append(vals, obj.AppendDependOn(v).getBottomUses(actx, opt...)...)
			actx.popObject()
		}
	}
	v.GetUsers().ForEach(func(value *Value) {
		if ret := value.AppendDependOn(v).getBottomUses(actx, opt...); len(ret) > 0 {
			vals = append(vals, ret...)
		}
	})
	if vals.Len() <= 0 {
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
	switch inst := v.node.(type) {
	case *ssa.Call:
		method := inst.Method
		if method == nil {
			log.Infof("fallback: (call instruction 's method/func is not *Function) unknown caller, got: %v", inst.Method.String())
			return v.visitUserFallback(actx, opt...)
		}
		actx.pushCall(inst)
		//分析的当前值相同，说明进来就是当前值
		if ValueCompare(v, actx.Self) {
			log.Infof("value analysis: (call instruction) caller is self")
			return v.visitUserFallback(actx, opt...)
		}
		existed := map[int64]struct{}{}
		var vals Values
		checkVal := func(args []ssa.Value, get func(index int, arg ssa.Value) (*Value, bool), handle func(value *Value)) {
			for index, arg := range args {
				value, ok := get(index, arg)
				if ok {
					if !utils.IsNil(value) {
						handle(value)
					}
				}
			}
		}
		v.DependOn.ForEach(func(value *Value) {
			existed[value.GetId()] = struct{}{}
		})
		fun, isFunc := ssa.ToFunction(method)
		if !isFunc && method.GetReference() != nil {
			fun, isFunc = ssa.ToFunction(method.GetReference())
		}
		if isFunc {
			checkVal(inst.Args, func(index int, arg ssa.Value) (*Value, bool) {
				if index >= len(fun.Params) {
					return nil, false
				}
				_, ok := existed[arg.GetId()]
				if !ok {
					return nil, false
				}
				value := fun.Params[index]
				return v.NewBottomUseValue(value), true
			}, func(value *Value) {
				vals = append(vals, value)
			})
			checkVal(inst.ArgMember, func(index int, arg ssa.Value) (*Value, bool) {
				if index >= len(fun.ParameterMembers) {
					return nil, false
				}
				_, ok := existed[arg.GetId()]
				if !ok {
					return nil, false
				}
				value := fun.ParameterMembers[index]
				return v.NewBottomUseValue(value), true
			}, func(value *Value) {
				vals = append(vals, value)
			})
			var result Values
			for _, val := range vals {
				actx.setCauseValue(v)
				result = append(result, val.getBottomUses(actx, opt...)...)
			}
			if result.Len() == 0 {
				result = append(result, v.visitUserFallback(actx, opt...)...)
			}
			return result
		}
		var backTrackSearch func(ssa.Value, int) *Value
		backTrackSearch = func(method ssa.Value, callIndex int) *Value {
			_, isparam := ssa.ToParameter(method)
			_, isParameterMember := ssa.ToParameterMember(method)
			if !(isParameterMember || isparam) {
				return v.NewValue(method)
			}
			methodId := method.GetId()
			call := actx.peekCall(callIndex)
			if utils.IsNil(call) {
				return v.NewValue(method)
			}
			function := call.Method
			toFunction, isFunction := ssa.ToFunction(function)
			if !isFunction {
				return v.NewValue(method)
			}
			var val *Value
			checkVal(toFunction.Params, func(index int, arg ssa.Value) (*Value, bool) {
				if index >= len(call.Args) {
					return nil, false
				}
				if arg.GetId() == methodId {
					return v.NewValue(call.Args[index]), true
				}
				return nil, false
			}, func(value *Value) {
				val = value
			})
			checkVal(toFunction.ParameterMembers, func(index int, arg ssa.Value) (*Value, bool) {
				if index >= len(call.ArgMember) {
					return nil, false
				}
				if arg.GetId() == methodId {
					return v.NewValue(call.ArgMember[index]), true
				}
				return nil, false
			}, func(value *Value) {
				val = value
			})
			if val == nil {
				return v.NewValue(method)
			}
			return backTrackSearch(val.node, callIndex+1)
		}
		search := backTrackSearch(method, 1)
		if search.GetId() == method.GetId() {
			return v.visitUserFallback(actx, opt...)
		}
		//todo： copy？
		s := &ssa.Call{
			Method:          search.node,
			Args:            inst.Args,
			Binding:         inst.Binding,
			ArgMember:       inst.ArgMember,
			Async:           inst.Async,
			Unpack:          inst.Unpack,
			IsDropError:     inst.IsDropError,
			IsEllipsis:      inst.IsEllipsis,
			SideEffectValue: inst.SideEffectValue,
		}
		for _, user := range v.node.GetUsers() {
			s.AddUser(user)
		}
		value := v.NewBottomUseValue(s)
		value.DependOn = append(value.DependOn, v.DependOn...)
		return value.getBottomUses(actx, opt...)
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
		call := actx.getLastCauseValue()
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
			if _, ok := exists[result.GetId()]; ok {
				getReturnIndex = index
			}
		}
		if getReturnIndex != -1 {
			member := call.GetMember(v.NewValue(ssa.NewConst(getReturnIndex)))
			if member == nil {
				log.Errorf("BUG: (return instruction 's member is nil),check it")
			} else {
				vals = append(vals, member.AppendDependOn(v).getBottomUses(actx, opt...)...)
			}
		}
		if vals.Len() == 0 {
			vals = append(vals, v.NewBottomUseValue(call.node).getBottomUses(actx, opt...)...)
		}
		return vals
	}
	return v.visitUserFallback(actx, opt...)
}
