package ssaapi

import (
	"context"

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
			if r == context.Canceled {
				log.Warnf("Value GetBottomUses context canceled, stop it: %s", v.String())
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
	if actx.widen != nil {
		defer func() { lastWidenTrace.Store(actx.widen) }()
	}
	ret = v.getBottomUses(actx, opt...)
	if actx.HasUntilNode() {
		ret = actx.untilMatch
	}
	if ret.Count() > dataflowValueLimit {
		log.Warnf("Value BottomUse too many: %d:\n\t%s", ret.Count(), v.StringWithRange())
		if report := actx.widenReport(); report != "" {
			log.Warnf("Value BottomUse widening: %s", report)
		}
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
		// Keyed descent: the trace reached this object because some site read
		// `v.key`. Resolve that key directly -- with the whole-table walk every
		// sibling member is traced in turn and each of those traces can re-enter
		// the object, which is what makes a wide type explode on a large project.
		obj, key, member := actx.getCurrentObject()
		if obj != nil && obj.GetId() == v.GetId() {
			if matched := resolveKeyedMembers(v, key); len(matched) > 0 {
				for _, m := range matched {
					if utils.IsNil(m) || ValueCompare(m, member) {
						continue
					}
					if err := actx.pushObject(v, m.GetKey(), m); err != nil {
						continue
					}
					vals = append(vals, m.getBottomUses(actx, opt...)...)
					actx.popObject()
				}
				goto users
			}
		}
		exist := false
		actx.foreachObjectStack(func(obj *Value, key *Value, val *Value) bool {
			if obj.GetId() == v.GetId() {
				exist = true
				return false
			}
			return true
		})
		if !exist {
			members := v.GetAllMember()
			walked := 0
			members.ForEach(func(value *Value) {
				if callableMemberValue(value) {
					return
				}
				walked++
				_ = actx.pushObject(v, value.GetKey(), value)
				vals = append(vals, value.getBottomUses(actx, opt...)...)
				actx.popObject()
			})
			actx.traceObjectExpansion(walked)
		}
	}
users:
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
	users := v.GetUsers()
	actx.traceUserFanout(len(users))
	users.ForEach(func(value *Value) {
		// log.Infof("value %s", value)
		if ret := value.getBottomUses(actx, opt...); len(ret) > 0 {
			vals = append(vals, ret...)
		}
	})
	if len(vals) == 0 {
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
	actx.traceNodeVisit()

	// if not shadow value return i self
	v = actx.CovertShadowValue(v)

	if !actx.structAllowValue(v) {
		return Values{}
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
	case *ssa.LazyInstruction:
		new := v.NewValue(v.getValue())
		return new.getBottomUses(actx, opt...)
	case *ssa.Phi:
		return v.visitUserFallback(actx, opt...)
	case *ssa.Call:
		method, ok := actx.getResolvedValue(inst, inst.Method)
		if !ok || method == nil {
			log.Infof("fallback: (call instruction 's method/func is not *Function) unknown caller, ")
			return v.visitUserFallback(actx, opt...)
		}
		actx.pushCall(inst)
		// getRealMethod walks the call stack upwards (peekCall(1) is the call
		// that invoked the current function) to bind a Parameter/ParameterMember
		// callee to the function it actually receives at that site. That lookup
		// is only meaningful while the stack holds the current nest of entered
		// calls: without a matching pop, every sibling call left its frame
		// behind, so a later call in the same body saw an unrelated earlier
		// sibling as its "parent" and bound parameters to the wrong arguments.
		// Pop on every exit from this call's subtree -- all of them return from
		// this function -- so the stack mirrors the live nest.
		defer actx.popCall()
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
			function, ok := actx.getResolvedValue(call, call.Method)
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
			valValue, ok := actx.getResolvedValue(call, val)
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

		// 如果 real 不是函数，检查是否是 FreeValue Parameter
		// TypeScript 等语言会将闭包捕获的外部函数表示为 FreeValue
		if !isFunc {
			if param, ok := ssa.ToParameter(real); ok && param.IsFreeValue {
				// FreeValue 的 GetDefault() 返回实际捕获的值
				if defaultVal := param.GetDefault(); !utils.IsNil(defaultVal) {
					fun, isFunc = ssa.ToFunction(defaultVal)
				}
			}
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
				paramValue, ok := actx.getResolvedValue(fun, fun.Params[index])
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
				memberValue, ok := actx.getResolvedValue(fun, fun.ParameterMembers[index])
				if !ok || memberValue == nil {
					continue
				}
				val := v.NewValue(memberValue)
				if val != nil {
					vals = append(vals, val.getBottomUses(actx, opt...)...)
				}
			}
		}
		if len(vals) > 0 {
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
		if len(vals) == 0 {
			vals = append(vals, v.NewValue(call.getValue()).getBottomUses(actx, opt...)...)
		}
		return vals
	}
	return v.visitUserFallback(actx, opt...)
}
