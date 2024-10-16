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

func (i *Value) visitedDefs(actx *AnalyzeContext, opt ...OperationOption) Values {
	var vals Values
	if i.node == nil {
		return vals
	}

	for _, def := range i.node.GetValues() {

		if ret := i.NewValue(def).AppendEffectOn(i).getTopDefs(actx, opt...); len(ret) > 0 {
			vals = append(vals, ret...)
		}
	}

	if len(vals) <= 0 {
		vals = append(vals, i)
	}
	if maskable, ok := i.node.(ssa.Maskable); ok {
		for _, def := range maskable.GetMask() {
			if ret := i.NewValue(def).AppendEffectOn(i).getTopDefs(actx, opt...); len(ret) > 0 {
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

	reachDepthLimit := actx.check(opt...)
	if reachDepthLimit {
		return Values{i}
	}
	err := actx.hook(i)
	if err != nil {
		return Values{i}
	}
	if inst, ok := ssa.ToLazyInstruction(i.node); ok {
		var ok bool
		i.node, ok = inst.Self().(ssa.Value)
		if !ok {
			log.Errorf("BUG: %T is not ssa.Value", inst.Self())
			return Values{}
		}
		return i.getTopDefs(actx, opt...)
	}
	if !actx.TheValueShouldBeVisited(i) {
		return Values{i}
	}
	{
		obj, key, member := actx.GetCurrentObject()
		_ = obj
		_ = key
		_ = member
		if obj != nil && i.IsObject() && i != obj {
			if m := i.GetMember(key); m != nil && !ValueCompare(m, member) {
				actx.PopObject()
				return m.getTopDefs(actx, opt...)
			}
		}
	}

	switch inst := i.node.(type) {
	case *ssa.Undefined:
		// ret[n]
		return i.getMemberCall(inst, actx, opt...)
	case *ssa.ConstInst:
		return i.visitedDefs(actx, opt...)
	case *ssa.Phi:
		conds := inst.GetControlFlowConditions()
		result := i.getMemberCall(inst, actx, opt...)
		for _, cond := range conds {
			v := i.NewValue(cond)
			ret := v.AppendEffectOn(i).getTopDefs(actx, opt...)
			result = append(result, ret...)
		}
		return result
	case *ssa.Call:
		calleeInst := inst.Method
		if calleeInst == nil {
			return Values{i} // return self
		}

		fun, isFunc := ssa.ToFunction(calleeInst)
		// callee := i.NewValue(fun)
		if !isFunc && calleeInst.GetReference() != nil {
			fun, isFunc = ssa.ToFunction(calleeInst.GetReference())
			// callee = i.NewValue(fun)
		}

		switch {
		case isFunc && !fun.IsExtern():
			callee := i.NewValue(fun)
			callee.SetContextValue(ANALYZE_RUNTIME_CTX_TOPDEF_CALL_ENTRY, i)
			callee.AppendEffectOn(i)
			actx.PushCrossProcess(i, callee, i)
			defer actx.PopCrossProcess()
			// inherit return index
			val, ok := i.GetContextValue(ANALYZE_RUNTIME_CTX_TOPDEF_CALL_ENTRY_TRACE_INDEX)
			if ok {
				callee.SetContextValue(ANALYZE_RUNTIME_CTX_TOPDEF_CALL_ENTRY_TRACE_INDEX, val)
			}
			return callee.getTopDefs(actx, opt...).AppendEffectOn(callee)
		default:
			callee := i.NewValue(calleeInst)
			i.AppendDependOn(callee)
			nodes := Values{callee}
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
				vals := subNode.getTopDefs(actx, opt...).AppendEffectOn(subNode)
				results = append(results, vals...)
			}
			return results
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

		handlerReturn := func(value *Value) {
			fun, ok := ssa.ToFunction(value.node)
			if !ok {
				return
			}
			for _, r := range fun.Return {
				for _, subVal := range r.GetValues() {
					if ret := value.NewValue(subVal).AppendEffectOn(value).getTopDefs(actx, opt...); len(ret) > 0 {
						vals = append(vals, ret...)
					}
				}
			}
		}

		handlerReturn(i)
		if len(vals) == 0 {
			vals = append(vals, i)
		}
		// handler child-class function
		for _, child := range inst.GetPointer() {
			handlerReturn(i.NewValue(child))
		}
		return vals.AppendEffectOn(i)
	case *ssa.ParameterMember:
		vals := i.getParamTopDef(actx, opt...)
		if len(vals) == 0 {
			return i.getParamForParameterMember(actx, opt...)
		}
		return vals.AppendEffectOn(i)
	case *ssa.Parameter:
		vals := i.getParamTopDef(actx, opt...)
		if len(vals) == 0 {
			if i.IsFreeValue() && inst.GetDefault() != nil {
				vals = append(vals, i.NewValue(inst.GetDefault()))
			} else {
				vals = append(vals, i)
			}
		}
		return vals.AppendEffectOn(i)
	case *ssa.SideEffect:
		callIns := inst.CallSite
		if callIns != nil {
			call := i.NewValue(callIns).AppendEffectOn(i)
			v := i.NewValue(inst.Value).AppendEffectOn(i)
			actx.PushCrossProcess(call, v, call)
			defer actx.PopCrossProcess()
			return v.getTopDefs(actx, opt...)
		} else {
			log.Errorf("side effect: %v is not created from call instruction", i.String())
		}
	case *ssa.Make:
		var values Values
		values = append(values, i)
		for key, member := range inst.GetAllMember() {
			value := i.NewValue(member)
			if err := actx.PushObject(i, i.NewValue(key), value); err != nil {
				log.Errorf("push object failed: %v", err)
				continue
			}
			values = append(values, value.getTopDefs(actx, opt...)...)
			actx.PopObject()
		}
		return values
	}
	return i.getMemberCall(i.node, actx, opt...)
}

func (i *Value) getParamTopDef(actx *AnalyzeContext, opt ...OperationOption) Values {
	var vals Values
	called := actx.GetLastCallStackCall()
	if called != nil {
		if !called.IsCall() {
			log.Infof("parent function is not called by any other function, skip (%T)", called)
			return Values{i}
		}
		recoverProcess := actx.PopCrossProcess()
		calledByValue := i.getCalledByValue(actx, called, opt...)
		recoverProcess()
		vals = append(vals, calledByValue...)
	}
	// if not found in call stack, then find in called-by
	if actx.config.AllowIgnoreCallStack && len(vals) == 0 {
		fun := i.GetFunction()
		if fun != nil {
			call2fun := fun.GetCalledBy()
			call2fun.ForEach(func(call *Value) {
				ok := actx.PushCrossProcess(i, call, nil)
				if !ok {
					return
				}
				val := i.getCalledByValue(actx, call, opt...)
				actx.PopCrossProcess()
				vals = append(vals, val...)
			})
		}
	}
	return vals
}

func (i *Value) getCalledByValue(actx *AnalyzeContext, called *Value, opt ...OperationOption) Values {
	if called == nil {
		return nil
	}
	calledInstance, ok := ssa.ToCall(called.node)
	if !ok {
		log.Infof("BUG: Parameter/ParameterMember getCalledByValue called is not callInstruction %s", called.GetOpcode())
		return Values{}
	}

	var actualParam ssa.Value
	switch inst := i.node.(type) {
	case *ssa.ParameterMember:
		if inst.FormalParameterIndex >= len(calledInstance.ArgMember) {
			return i.getParamForParameterMember(actx, opt...)
		}
		actualParam = calledInstance.ArgMember[inst.FormalParameterIndex]
	case *ssa.Parameter:
		thisFunc := i.GetFunction()
		if !ValueCompare(i.NewValue(calledInstance.Method), thisFunc) {
			log.Errorf("call stack function %s(%d) not same with Parameter function %s(%d)",
				calledInstance.Method.GetName(), calledInstance.Method.GetId(),
				thisFunc.GetName(), thisFunc.GetId(),
			)
			return Values{}
		}
		if inst.IsFreeValue {
			// free value
			if tmp, ok := calledInstance.Binding[inst.GetName()]; ok {
				actualParam = tmp
			} else {
				log.Errorf("free value: %v is not found in binding", inst.GetName())
				return i.getMemberCall(i.node, actx, opt...)
			}
		} else {
			// parameter
			if inst.FormalParameterIndex >= len(calledInstance.Args) {
				log.Infof("formal parameter index: %d is out of range", inst.FormalParameterIndex)
				return i.getMemberCall(i.node, actx, opt...)
			}
			actualParam = calledInstance.Args[inst.FormalParameterIndex]
		}
	default:
		log.Warnf("BUG: %T is not *Parameter or *ParameterMember", i.node)
		return Values{}
	}
	traced := i.NewValue(actualParam).AppendEffectOn(called)
	ret := traced.getTopDefs(actx, opt...)
	if len(ret) > 0 {
		return ret
	} else {
		return Values{traced}
	}
}

func (i *Value) getMemberCall(value ssa.Value, actx *AnalyzeContext, opt ...OperationOption) Values {
	if value.HasValues() {
		return i.visitedDefs(actx, opt...)
	}
	if value.IsMember() {
		obj := i.NewValue(value.GetObject())
		key := i.NewValue(value.GetKey())
		if err := actx.PushObject(obj, key, i); err != nil {
			log.Errorf("%v", err)
			return i.visitedDefs(actx, opt...)
		}
		obj.AppendDependOn(i)
		actx.PushCrossProcess(i, obj, nil)
		ret := obj.getTopDefs(actx, opt...)
		defer actx.PopCrossProcess()
		if !ValueCompare(i, actx.Self) {
			ret = append(ret, i)
		}
		return ret
	}
	return i.visitedDefs(actx, opt...)
}

func (i *Value) getParamForParameterMember(actx *AnalyzeContext, opt ...OperationOption) Values {
	inst, ok := ssa.ToParameterMember(i.node)
	if !ok {
		return Values{}
	}
	fun := i.GetFunction()
	switch inst.MemberCallKind {
	case ssa.ParameterMemberCall:
		if para := fun.GetParameter(inst.MemberCallObjectIndex); para != nil {
			return para.getTopDefs(actx, opt...)
		}
	case ssa.FreeValueMemberCall:
		if fv := fun.GetFreeValue(inst.MemberCallObjectName); fv != nil {
			return fv.getTopDefs(actx, opt...)
		}
	}
	return Values{i}
}
