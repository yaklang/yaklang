package ssaapi

import (
	"fmt"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// GetTopDefs desc all of 'Defs' is not used by any other value
func (i *Value) GetTopDefs(opt ...OperationOption) Values {
	actx := NewAnalyzeContext(opt...)
	actx.Self = i
	ret := i.getTopDefs(actx, opt...)
	ret = lo.UniqBy(ret, func(item *Value) int64 {
		return item.GetId()
	})
	return ret
}

func (v Values) GetTopDefs(opts ...OperationOption) Values {
	ret := make(Values, 0)
	for _, sub := range v {
		ret = append(ret, sub.GetTopDefs(opts...)...)
	}
	return ret
}

func (v Values) GetBottomUses(opts ...OperationOption) Values {
	ret := make(Values, 0)
	for _, sub := range v {
		ret = append(ret, sub.GetBottomUses(opts...)...)
	}
	return ret
}

func (i Values) AppendEffectOn(v *Value) Values {
	for _, node := range i {
		node.AppendEffectOn(v)
	}
	return i
}

func (i Values) AppendDependOn(v *Value) Values {
	for _, node := range i {
		node.AppendDependOn(v)
	}
	return i
}

func (i *Value) visitedDefsDefault(actx *AnalyzeContext) Values {
	var vals Values
	if i.node == nil {
		return vals
	}
	if !actx.TheDefaultShouldBeVisited(i) {
		return vals
	}
	for _, def := range i.node.GetValues() {
		if ret := i.NewValue(def).AppendEffectOn(i).getTopDefs(actx); len(ret) > 0 {
			vals = append(vals, ret...)
		}
	}

	if len(vals) <= 0 {
		vals = append(vals, i)
	}
	if maskable, ok := i.node.(ssa.Maskable); ok {
		for _, def := range maskable.GetMask() {
			if ret := i.NewValue(def).AppendEffectOn(i).getTopDefs(actx); len(ret) > 0 {
				vals = append(vals, ret...)
			}
		}
	}
	return vals
}

func (i *Value) getTopDefs(actx *AnalyzeContext, opt ...OperationOption) Values {
	if i == nil {
		return nil
	}

	if actx == nil {
		actx = NewAnalyzeContext(opt...)
	}

	actx.depth--
	defer func() {
		actx.depth++
	}()
	i.SetDepth(actx.depth)
	if actx.depth > 0 && actx.config.MaxDepth > 0 && actx.depth > actx.config.MaxDepth {
		return i.DependOn
	}
	if actx.depth < 0 && actx.config.MinDepth < 0 && actx.depth < actx.config.MinDepth {
		return i.EffectOn
	}

	// hook everynode
	if len(actx.config.HookEveryNode) > 0 {
		for _, hook := range actx.config.HookEveryNode {
			if err := hook(i); err != nil {
				log.Errorf("hook-every-node error: %v", err)
				return Values{}
			}
		}
	}

	{
		obj, key, member := actx.GetCurrentObject()
		_ = obj
		_ = key
		_ = member
		if obj != nil && i.IsObject() && i != obj {
			if m := i.GetMember(key); m != nil && m != member {
				actx.PopObject()
				return m.getTopDefs(actx, opt...)
			}
		}
	}
	if i.IsMember() && !actx.TheMemberShouldBeVisited(i) {
		return Values{}
	}

	getMemberCall := func(value ssa.Value, actx *AnalyzeContext) Values {
		if value.HasValues() {
			return i.visitedDefsDefault(actx)
		}
		if value.IsMember() {
			obj := i.NewValue(value.GetObject())
			key := i.NewValue(value.GetKey())
			if err := actx.PushObject(obj, key, i); err != nil {
				log.Errorf("%v", err)
				return i.visitedDefsDefault(actx)
			}

			ret := obj.getTopDefs(actx, opt...)
			if !ValueCompare(i, actx.Self) {
				ret = append(ret, i)
			}
			return ret
		}
		return i.visitedDefsDefault(actx)
	}

	switch inst := i.node.(type) {
	case *ssa.LazyInstruction:
		var ok bool
		i.node, ok = inst.Self().(ssa.Value)
		if !ok {
			log.Errorf("BUG: %T is not ssa.Value", inst.Self())
			return Values{}
		}
		return i.getTopDefs(actx, opt...)
	case *ssa.Undefined:
		// ret[n]
		return getMemberCall(inst, actx)
	case *ssa.Phi:
		if !actx.ThePhiShouldBeVisited(i) {
			// phi is visited...
			return Values{}
		}
		actx.VisitPhi(i)

		conds := inst.GetControlFlowConditions()
		result := getMemberCall(inst, actx)
		for _, cond := range conds {
			v := i.NewValue(cond)
			ret := v.getTopDefs(actx, opt...)
			result = append(result, v)
			result = append(result, ret...)
		}
		_ = conds
		return result
	case *ssa.Call:
		caller := inst.Method
		if caller == nil {
			return Values{i} // return self
		}

		// TODO: trace the specific return-values
		callerValue := i.NewValue(caller)
		_, isFunc := ssa.ToFunction(caller)
		funcType, isFuncTyp := ssa.ToFunctionType(caller.GetType())
		if callerValue.IsExtern() {
			i.AppendDependOn(callerValue)
			nodes := Values{callerValue}
			for _, val := range inst.Args {
				arg := i.NewValue(val)
				i.AppendDependOn(arg)
				nodes = append(nodes, arg)
			}
			for _, value := range inst.Binding {
				arg := i.NewValue(value)
				i.AppendDependOn(arg)
				nodes = append(nodes, arg)
			}
			var results Values
			for _, subNode := range nodes {
				if subNode == nil {
					continue
				}
				// getTopDefs(nil,opt...)第一个参数指定为nil
				// 提供一个新的上下文，避免指向新的actx.self导致多余的结果
				vals := subNode.GetTopDefs(opt...).AppendEffectOn(subNode)
				//vals := subNode.getTopDefs(nil, opt...).AppendEffectOn(subNode)
				results = append(results, vals...)
			}
			return results
		}
		switch {
		case isFunc:
			callerValue.SetContextValue(ANALYZE_RUNTIME_CTX_TOPDEF_CALL_ENTRY, i)
			callerValue.AppendEffectOn(i)
			err := actx.PushCall(i)
			if err != nil {
				log.Warnf("push call failed, if the current path in side-effect, ignore it: %v", err)
				return Values{i}
			}
			defer actx.PopCall()
			// inherit return index
			val, ok := i.GetContextValue(ANALYZE_RUNTIME_CTX_TOPDEF_CALL_ENTRY_TRACE_INDEX)
			if ok {
				callerValue.SetContextValue(ANALYZE_RUNTIME_CTX_TOPDEF_CALL_ENTRY_TRACE_INDEX, val)
			}
			return callerValue.getTopDefs(actx, opt...).AppendEffectOn(callerValue)
		case isFuncTyp:
			// funcType.ReturnType
			// string literal member
			err := actx.PushCall(i)
			if err != nil {
				log.Warnf("push call failed, if the current path in side-effect, ignore it: %v", err)
				return Values{i}
			}
			defer actx.PopCall()

			var res Values
			for _, retIns := range funcType.ReturnValue {
				for _, traceVal := range retIns.Results {
					// val, ok := traceVal.GetStringMember(retIndexRawStr)
					// if ok {
					res = append(res,
						i.NewValue(traceVal).AppendEffectOn(i).getTopDefs(actx, opt...)...,
					)
				}
			}
			if len(res) == 0 {
				// the result from the return value is empty,
				// get the topDef by the callee
				res = append(res,
					callerValue.AppendDependOn(i).getTopDefs(actx, opt...)...,
				)
			}
			return res
		}
	case *ssa.Function:
		var vals Values
		// handle return
		returnIndex, traceIndexedReturn := i.GetContextValue(ANALYZE_RUNTIME_CTX_TOPDEF_CALL_ENTRY_TRACE_INDEX)
		if traceIndexedReturn {
			retIndexRaw := returnIndex.GetConstValue()
			retIndexRawStr := fmt.Sprint(retIndexRaw)
			if utils.IsValidInteger(retIndexRawStr) {
				targetIdx := codec.Atoi(retIndexRawStr)
				var traceRets Values
				for _, retInsRaw := range inst.Return {
					retIns, ok := ssa.ToReturn(retInsRaw)
					if !ok {
						log.Warnf("BUG: %T is not *Return", retInsRaw)
						continue
					}
					for idx, traceVal := range retIns.Results {
						if idx == targetIdx {
							traceRets = append(traceRets, i.NewValue(traceVal).AppendEffectOn(i))
						}
					}
				}
				return lo.FlatMap(traceRets, func(item *Value, index int) []*Value {
					return item.getTopDefs(actx, opt...)
				})
			} else {
				// string literal member
				var traceRets Values
				for _, retInsRaw := range inst.Return {
					retIns, ok := ssa.ToReturn(retInsRaw)
					if !ok {
						log.Warnf("BUG: %T is not *Return", retInsRaw)
						continue
					}
					for _, traceVal := range retIns.Results {
						val, ok := traceVal.GetStringMember(retIndexRawStr)
						if ok {
							traceRets = append(traceRets, i.NewValue(val).AppendEffectOn(i))
							// trace mask ?
							if len(inst.Blocks) > 0 {
								name, ok := ssa.CombineMemberCallVariableName(traceVal, ssa.NewConst(retIndexRawStr))
								if ok {
									lastBlockRaw, _ := lo.Last(inst.Blocks)
									lastBlock, ok := ssa.ToBasicBlock(lastBlockRaw)
									if ok {
										variableInstance := lastBlock.ScopeTable.ReadVariable(name)
										_ = variableInstance.String()
									}
								}
							}
						}
					}
				}
				return lo.FlatMap(traceRets, func(item *Value, index int) []*Value {
					return item.getTopDefs(actx, opt...)
				})
			}
		}

		for _, r := range inst.Return {
			for _, subVal := range r.GetValues() {
				if ret := i.NewValue(subVal).AppendEffectOn(i).getTopDefs(actx); len(ret) > 0 {
					vals = append(vals, ret...)
				}
			}
		}
		if len(vals) == 0 {
			return Values{i} // no return, use undefined
		}
		return vals.AppendEffectOn(i)
	case *ssa.ParameterMember:
		// log.Info("ParameterMember")
		called := actx.GetCurrentCall()
		if called == nil {
			// log.Info("parent function is not called by any other function, skip")
			var vals Values
			vals = append(vals, i)
			// 获取ParameterMember的形参定义
			obj := inst.GetObject()
			if obj != nil {
				if inst.MemberCallKind == ssa.ParameterMemberCall {
					objValue := i.NewValue(obj)
					val := objValue.GetFunction().GetParameter(inst.MemberCallObjectIndex)
					if val != nil {
						vals = append(vals, val)
					}
				} else if inst.MemberCallKind == ssa.FreeValueMemberCall {
					param := inst.GetFunc().FreeValues[obj.GetName()]
					val := i.NewValue(param)
					vals = append(vals, val)
				}
			}
			return vals
		}
		calledInstance, ok := ssa.ToCall(called.node)
		if !ok {
			log.Infof("parent function is not called by any other function, skip (%T)", called)
			return Values{i}
		}

		// parameter
		if inst.FormalParameterIndex >= len(calledInstance.ArgMember) {
			log.Infof("formal parameter member index: %d is out of range", inst.FormalParameterIndex)
			return getMemberCall(i.node, actx)
		}
		actualParam := calledInstance.ArgMember[inst.FormalParameterIndex]
		traced := i.NewValue(actualParam).AppendEffectOn(called)
		if ret := traced.getTopDefs(actx); len(ret) > 0 {
			return ret
		} else {
			return Values{traced}
		}

	case *ssa.Parameter:
		// 查找被调用函数的TopDef
		getCalledByValue := func(called *Value) Values {
			calledInstance, ok := ssa.ToCall(called.node)
			if !ok {
				log.Infof("BUG: Parameter getCalledByValue called is not callInstruction %s", called.GetOpcode())
				return Values{}
			}

			// fun := i.GetFunction()
			// if !ValueCompare(fun, i.NewValue(calledInstance.Method)) {
			// 	return Values{}
			// }

			var actualParam ssa.Value
			if inst.IsFreeValue {
				// free value
				if tmp, ok := calledInstance.Binding[inst.GetName()]; ok {
					actualParam = tmp
				} else {
					log.Errorf("free value: %v is not found in binding", inst.GetName())
					return getMemberCall(i.node, actx)
				}
			} else {
				// parameter
				if inst.FormalParameterIndex >= len(calledInstance.Args) {
					log.Infof("formal parameter index: %d is out of range", inst.FormalParameterIndex)
					return getMemberCall(i.node, actx)
				}
				actualParam = calledInstance.Args[inst.FormalParameterIndex]
			}
			traced := i.NewValue(actualParam).AppendEffectOn(called)
			// todo: 解决exclusive_callstack_top_test.go测试不受出入栈影响
			call := actx.PopCall()

			ret := traced.getTopDefs(actx)
			if call != nil {
				actx.PushCall(call)
			}
			if len(ret) > 0 {
				return ret
			} else {
				return Values{traced}
			}
		}

		if inst.GetDefault() != nil {
			return i.NewValue(inst.GetDefault()).getTopDefs(actx, opt...)
		}
		var vals Values
		called := actx.GetCurrentCall()
		if called != nil {
			if !called.IsCall() {
				log.Infof("parent function is not called by any other function, skip (%T)", called)
				return Values{i}
			}
			calledByValue := getCalledByValue(called)
			vals = append(vals, calledByValue...)
		}

		if actx.config.AllowIgnoreCallStack {
			fun := i.GetFunction()
			if fun != nil {
				call2fun := fun.GetCalledBy()
				call2fun.ForEach(func(call *Value) {
					val := getCalledByValue(call)
					vals = append(vals, val...)
				})
			}
		}

		if len(vals) == 0 {
			// return Values{i} // no return, use undefined
			vals = append(vals, i)
		}
		return vals.AppendEffectOn(i)
	case *ssa.SideEffect:
		callIns := inst.CallSite
		if callIns != nil {
			err := actx.PushCall(i.NewValue(callIns).AppendEffectOn(i))
			if err != nil {
				log.Errorf("push call error: %v", err)
			} else {
				defer actx.PopCall()

				v := i.NewValue(inst.Value).AppendEffectOn(i)
				return v.getTopDefs(actx)
			}
		} else {
			log.Errorf("side effect: %v is not created from call instruction", i.String())
		}
	case *ssa.Make:
		// 根据make的参数查看TopDef
		// 比如:new String(data)
		// 会继续往上找data的TopDef
		getMakeParamTopDef := func() Values {
			var vals Values
			params := i.GetFunction().GetParameters()
			for _, param := range params {
				if param.GetName() == "this" || param.GetName() == "$this" {
					continue
				}
				vals = append(vals, param.getTopDefs(actx, opt...)...)
			}
			return vals
		}

		var values Values
		values = append(values, i)
		for _, member := range inst.GetAllMember() {
			value := i.NewValue(member)
			values = append(values, value.getTopDefs(actx, opt...)...)
		}
		paramVals := getMakeParamTopDef()
		values = append(values, paramVals...)
		return values
	}
	return getMemberCall(i.node, actx)
}
