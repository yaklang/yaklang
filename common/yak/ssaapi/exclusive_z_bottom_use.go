package ssaapi

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (v *Value) GetBottomUses(opt ...OperationOption) (ret Values) {
	defer func() {
		if r := recover(); r != nil {
			if r == errRecursiveDepth {
				log.Warnf("Value GetBottomUses recursive call too deep, stop it: %s", v.String())
				ret = nil
				return
			}
			log.Errorf("Value GetBottomUses panic: %v", r)
			utils.PrintCurrentGoroutineRuntimeStack()
			ret = nil
		}
	}()
	actx := NewAnalyzeContext(opt...)
	actx.Self = v
	actx.direct = BottomUseAnalysis
	ret = v.getBottomUses(actx, opt...)
	if actx.HasUntilNode() {
		ret = actx.untilMatch
	}
	if ret.Count() > dataflowValueLimit {
		log.Warnf("Value BottomUse %v too many: %d", v.StringWithRange(), ret.Count())
		return nil
	}
	ret = MergeValues(ret)
	return
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
				vals = append(vals, value.getBottomUses(actx, opt...)...)
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
			vals = append(vals, currentObject.getBottomUses(actx, opt...)...)
			actx.popObject()
		}
	}
	// log.Infof("current Value: %s", v)
	v.GetUsers().ForEach(func(value *Value) {
		// log.Infof("value %s", value)
		if ret := value.getBottomUses(actx, opt...); len(ret) > 0 {
			vals = append(vals, ret...)
		}
	})
	if vals.Len() == 0 {
		return Values{v}
	}
	return vals
}

func (v *Value) getBottomUses(actx *AnalyzeContext, opt ...OperationOption) (result Values) {

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
	defer func() {
		actx.SavePath(result)
	}()

	if shouldExit {
		return Values{v}
	}
	err := actx.hook(v)
	if err != nil {
		return Values{v}
	}
	if actx.isUntilNode(v) {
		return Values{v}
	}

	switch inst := v.GetSSAInst().(type) {
	case *ssa.Phi:
		return v.visitUserFallback(actx, opt...)
	case *ssa.Call:
		method, ok := inst.GetValueById(inst.Method)
		if !ok || method == nil {
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

		existed[actx.nodeStack.PeekN(1).GetId()] = struct{}{}

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
			function, ok := call.GetValueById(call.Method)
			if !ok {
				return method
			}
			toFunction, isFunction := ssa.ToFunction(function)
			if !isFunction {
				return method
			}
			var val int64
			for index, arg := range toFunction.Params {
				if index >= len(call.Args) {
					continue
				}
				if arg == methodId {
					val = call.Args[index]
				}
			}
			for index, arg := range toFunction.ParameterMembers {
				if index >= len(call.ArgMember) {
					continue
				}
				if arg == methodId {
					val = call.ArgMember[index]
				}
			}
			if val <= 0 {
				return method
			}
			valValue, ok := call.GetValueById(val)
			if !ok {
				return method
			}
			return getRealMethod(valValue, callIndex+1)
		}
		real := getRealMethod(method, 1)

		fun, isFunc := ssa.ToFunction(real)
		if !isFunc && method.GetReference() != nil {
			fun, isFunc = ssa.ToFunction(method.GetReference())
		}

		if isFunc {
			for index, arg := range inst.Args {
				if index >= len(fun.Params) {
					continue
				}
				_, ok := existed[arg]
				if !ok {
					continue
				}
				paramValue, ok := fun.GetValueById(fun.Params[index])
				if !ok || paramValue == nil {
					continue
				}
				val := v.NewValue(paramValue)
				if val != nil {
					vals = append(vals, val.getBottomUses(actx, opt...)...)
				}
			}
			for index, arg := range inst.ArgMember {
				if index >= len(fun.ParameterMembers) {
					continue
				}
				_, ok := existed[arg]
				if !ok {
					continue
				}
				memberValue, ok := fun.GetValueById(fun.ParameterMembers[index])
				if !ok || memberValue == nil {
					continue
				}
				val := v.NewValue(memberValue)
				if val != nil {
					vals = append(vals, val.getBottomUses(actx, opt...)...)
				}
			}
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
			callee := v.NewValue(function)
			called := callee.GetCalledBy()
			for index, call := range called {
				if index > dataflowValueLimit {
					break
				}
				val := call.getBottomUses(actx, opt...)
				vals = append(vals, val...)
			}

			// called.ForEach(func(value *Value) {
			// 	vals = append(vals, value.getBottomUses(actx, opt...)...)
			// })
			return vals
		}
		exists := make(map[int64]struct{})
		exists[actx.nodeStack.PeekN(1).GetId()] = struct{}{}

		getReturnIndex := -1
		for index, result := range inst.Results {
			if _, ok := exists[result]; ok {
				getReturnIndex = index
			}
		}
		if getReturnIndex != -1 {
			members := call.GetMember(v.NewValue(ssa.NewConst(getReturnIndex)))
			if members == nil {
				// TODO:这个日志报太多了，先注释了，后面遇到问题再修一下
				//log.Errorf("BUG: (return instruction 's member is nil),check it")
			} else {
				for i, member := range members {
					if i == 0 {
						actx.pushObject(call, member.GetKey(), member)
					}
					vals = append(vals, member.getBottomUses(actx, opt...)...)
					actx.popObject()
				}
			}
		}
		if vals.Len() == 0 {
			vals = append(vals, v.NewValue(call.innerValue).getBottomUses(actx, opt...)...)
		}
		return vals
	}
	return v.visitUserFallback(actx, opt...)
}
