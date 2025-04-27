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
	return MergeValues(ret)
}

func (v Values) GetTopDefs(opts ...OperationOption) Values {
	ret := make(Values, 0)
	for _, sub := range v {
		ret = append(ret, sub.GetTopDefs(opts...)...)
	}
	return MergeValues(ret)
}

func (i *Value) visitedDefs(actx *AnalyzeContext, opt ...OperationOption) (result Values) {
	var vals Values
	if i.innerValue == nil {
		return vals
	}
	for _, def := range i.innerValue.GetValues() {
		if utils.IsNil(def) {
			continue
		}
		if ret := i.NewTopDefValue(def).getTopDefs(actx, opt...); len(ret) > 0 {
			vals = append(vals, ret...)
		}
	}
	if len(vals) == 0 {
		vals = i.AddSelfToTopDefResult(vals)
	}

	if maskable, ok := i.innerValue.(ssa.Maskable); ok {
		if len(maskable.GetMask()) == 0 {
			return vals
		}
		// 拿到上次递归的节点
		last := actx.getLastRecursiveNode()
		var shadow *Value
		// 新建个ssa.Value和i一样的ssaapi.Value,
		// 用以作为下个topdef的effecton的边
		// 而不影响i作为结果result有多出来的边
		if last != nil {
			shadow = last.NewTopDefValue(i.innerValue)
		} else {
			shadow = i.NewValue(i.innerValue)
		}
		for _, def := range maskable.GetMask() {
			if ret := shadow.NewTopDefValue(def).getTopDefs(actx, opt...); len(ret) > 0 {
				vals = append(vals, ret...)
			}
		}
	}
	return vals
}

func (i *Value) getTopDefs(actx *AnalyzeContext, opt ...OperationOption) (result Values) {
	defer func() {
		var finalResult Values
		for _, ret := range result {
			if ret.GetDependOn() == nil {
				finalResult = append(finalResult, ret)
			} else {
				log.Errorf("BUG:(topdef's result is not a tree node:%s have depend on %s)", ret.String(), ret.GetDependOn().String())
				log.Errorf("BUG:(topdef's result is not a tree node:%s have depend on %s)", ret.String(), ret.GetDependOn().String())
				log.Errorf("BUG:(topdef's result is not a tree node:%s have depend on %s)", ret.String(), ret.GetDependOn().String())
			}
		}
		result = finalResult
	}()
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

	if inst, ok := ssa.ToLazyInstruction(i.innerValue); ok {
		var ok bool
		i.innerValue, ok = inst.Self().(ssa.Value)
		if !ok {
			log.Errorf("BUG: %T is not ssa.Value", inst.Self())
			return Values{}
		}
		return i.getTopDefs(actx, opt...)
	}
	shouldExit, recoverStack := actx.check(i)
	defer recoverStack()
	if shouldExit {
		return Values{i}
	}
	err := actx.hook(i)
	if err != nil {
		return Values{i}
	}

	checkObject := func() Values {
		obj, key, member := actx.getCurrentObject()
		_ = obj
		_ = key
		_ = member
		if obj != nil && i.IsObject() && i != obj {
			if m := i.GetMember(key); m != nil && !ValueCompare(m, member) {
				actx.popObject()
				return m.getTopDefs(actx, opt...)
			}
		}
		return nil
	}
	vals := checkObject()
	if vals != nil {
		return vals
	}
	getMemberCall := func(apiValue *Value, value ssa.Value, actx *AnalyzeContext) Values {
		if utils.IsNil(value) {
			return nil
		}
		if value.HasValues() {
			return i.visitedDefs(actx, opt...)
		}
		if value.IsMember() {
			obj := i.NewValue(value.GetObject())
			key := i.NewValue(value.GetKey())
			if err := actx.pushObject(obj, key, i); err != nil {
				log.Errorf("%v", err)
				return i.visitedDefs(actx, opt...)
			}
			// obj.AppendDependOn(apiValue)
			apiValue.AppendDependOn(obj)
			ret := obj.getTopDefs(actx, opt...)
			if len(ret) == 0 && !ValueCompare(i, actx.Self) {
				vals = i.AddSelfToTopDefResult(vals)
			}
			return ret
		}
		return i.visitedDefs(actx, opt...)
	}

	switch inst := i.innerValue.(type) {
	case *ssa.Undefined:
		if inst.Kind == ssa.UndefinedValueReturn {
			return Values{}
		}
		return getMemberCall(i, inst, actx)
	case *ssa.ConstInst:
		return i.visitedDefs(actx, opt...)
	case *ssa.Phi:
		conds := inst.GetControlFlowConditions()
		result := getMemberCall(i, inst, actx)
		for _, cond := range conds {
			ret := i.NewTopDefValue(cond).getTopDefs(actx, opt...)
			result = append(result, ret...)
		}
		return result
	case *ssa.Call:
		calleeInst := inst.Method
		if calleeInst == nil {
			return Values{i} // return self
		}

		fun, isFunc := ssa.ToFunction(calleeInst)
		if !isFunc && calleeInst.GetReference() != nil {
			fun, isFunc = ssa.ToFunction(calleeInst.GetReference())
		}

		switch {
		case isFunc && !fun.IsExtern():
			callee := i.NewTopDefValue(fun)
			callee.SetContextValue(ANALYZE_RUNTIME_CTX_TOPDEF_CALL_ENTRY, i)
			// inherit return index
			val, ok := i.GetContextValue(ANALYZE_RUNTIME_CTX_TOPDEF_CALL_ENTRY_TRACE_INDEX)
			if ok {
				callee.SetContextValue(ANALYZE_RUNTIME_CTX_TOPDEF_CALL_ENTRY_TRACE_INDEX, val)
			}
			return callee.getTopDefs(actx, opt...)
		default:
			callee := i.NewTopDefValue(calleeInst)
			nodes := Values{callee}
			for _, val := range inst.Args {
				arg := i.NewTopDefValue(val)
				nodes = append(nodes, arg)
			}
			for _, value := range inst.Binding {
				arg := i.NewTopDefValue(value)
				nodes = append(nodes, arg)
			}
			var results Values
			for _, subNode := range nodes {
				if subNode == nil {
					continue
				}
				vals := subNode.getTopDefs(actx, opt...)
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
							traceRets = append(traceRets, i.NewTopDefValue(traceVal))
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
							traceRets = append(traceRets, i.NewTopDefValue(val))
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
			fun, ok := ssa.ToFunction(value.innerValue)
			if !ok {
				return
			}
			for _, r := range fun.Return {
				for _, subVal := range r.GetValues() {
					if ret := value.NewTopDefValue(subVal).getTopDefs(actx, opt...); len(ret) > 0 {
						vals = append(vals, ret...)
					}
				}
			}
		}

		handlerReturn(i)
		if len(vals) == 0 {
			vals = i.AddSelfToTopDefResult(vals)
		}
		// handler child-class function
		for _, child := range inst.GetPointer() {
			handlerReturn(i.NewTopDefValue(child))
		}
		return vals
	case *ssa.ParameterMember:
		var vals Values
		getParameter := func() Values {
			fun := i.GetFunction()
			if fun == nil {
				return Values{i}
			}
			fun.AppendEffectOn(i)
			switch inst.MemberCallKind {
			case ssa.ParameterMemberCall:
				if para := fun.GetParameter(inst.MemberCallObjectIndex); para != nil {
					actx.pushObject(para, i.NewValue(inst.MemberCallKey), i.NewValue(ssa.NewConst("")))
					return para.AppendEffectOn(fun).getTopDefs(actx, opt...)
				}
			case ssa.FreeValueMemberCall:
				if fv := fun.GetFreeValue(inst.MemberCallObjectName); fv != nil {
					actx.pushObject(fv, i.NewValue(inst.MemberCallKey), i.NewValue(ssa.NewConst("")))
					return fv.getTopDefs(actx, opt...)
				}
			}
			return Values{i}
		}
		getCalledByValue := func(called *Value) Values {
			if called == nil {
				return nil
			}
			calledInstance, ok := ssa.ToCall(called.innerValue)
			if !ok {
				log.Warnf("BUG: Parameter getCalledByValue called is not callInstruction %s", called.GetOpcode())
				return Values{}
			}
			var actualParam ssa.Value
			if inst.FormalParameterIndex >= len(calledInstance.ArgMember) {
				return getParameter()
			}
			actualParam = calledInstance.ArgMember[inst.FormalParameterIndex]
			traced := i.NewTopDefValue(actualParam)
			if !actx.needCrossProcess(i, traced) {
				return Values{}
			}
			ret := traced.getTopDefs(actx, opt...)
			if !actx.needCrossProcess(i, traced) {
				vals = i.AddSelfToTopDefResult(vals)
			}
			if len(ret) > 0 {
				return ret
			} else {
				return Values{traced}
			}
		}
		called := actx.getLastCauseCall(TopDefAnalysis)
		if called != nil {
			actx.setRollBack()
			calledByValue := getCalledByValue(called)
			vals = append(vals, calledByValue...)
		}
		if actx.config.AllowIgnoreCallStack && len(vals) == 0 {
			if fun := i.GetFunction(); fun != nil {
				call2fun := fun.GetCalledBy()
				call2fun.AppendEffectOn(fun)
				call2fun.ForEach(func(call *Value) {
					val := getCalledByValue(call)
					vals = append(vals, val...)
				})
			}
		}
		if len(vals) == 0 {
			return getParameter()
		}
		return vals
	case *ssa.Parameter:
		getCalledByValue := func(called *Value, isInners ...bool) Values {
			if called == nil {
				return nil
			}
			isInner := true
			if len(isInners) > 0 {
				isInner = isInners[0]
			}
			calledInstance, ok := ssa.ToCall(called.innerValue)
			if !ok {
				log.Infof("BUG: Parameter getCalledByValue called is not callInstruction %s", called.GetOpcode())
				return Values{}
			}
			var actualParam ssa.Value
			if inst.IsFreeValue {
				// free value
				if tmp := inst.GetDefault(); tmp != nil && !isInner {
					actualParam = tmp
				} else if tmp, ok := calledInstance.Binding[inst.GetName()]; ok && isInner {
					actualParam = tmp
				} else {
					log.Errorf("free value: %v is not found in binding", inst.GetName())
					return getMemberCall(i, i.innerValue, actx)
				}
			} else {
				// parameter
				if inst.FormalParameterIndex >= len(calledInstance.Args) {
					log.Infof("formal parameter index: %d is out of range", inst.FormalParameterIndex)
					return getMemberCall(i, i.innerValue, actx)
				}
				actualParam = calledInstance.Args[inst.FormalParameterIndex]
			}
			traced := i.NewTopDefValue(actualParam)
			if !actx.needCrossProcess(i, traced) {
				return Values{}
			}
			ret := traced.getTopDefs(actx, opt...)
			if len(ret) > 0 {
				return ret
			} else {
				return Values{traced}
			}
		}
		var vals Values
		// Retrieve the case value. And it is required that the value must be a Call.
		called := actx.getLastCauseCall(TopDefAnalysis)
		if called != nil {
			actx.setRollBack()
			calledByValue := getCalledByValue(called)
			vals = append(vals, calledByValue...)
		}
		// if not found in call stack, then find in called-by
		if actx.config.AllowIgnoreCallStack && len(vals) == 0 {
			if fun := i.GetFunction(); fun != nil {
				call2fun := fun.GetCalledBy()
				call2fun.AppendEffectOn(fun)
				call2fun.ForEach(func(call *Value) {
					val := getCalledByValue(call, true)
					vals = append(vals, val...)
				})
			}
		}

		if len(vals) == 0 {
			if i.IsFreeValue() && inst.GetDefault() != nil {
				vals = append(vals, i.NewTopDefValue(inst.GetDefault()))
			} else {
				// 如果要将自身作为叶子节点加入到结果，需要清除DependOn这条边
				// 这条边是在traced := i.NewTopDefValue(actualParam)的时候产生的
				vals = i.AddSelfToTopDefResult(vals)
			}
		}
		return vals
	case *ssa.SideEffect:
		callIns := inst.CallSite
		if callIns != nil {
			v := i.NewTopDefValue(inst.Value)
			return v.getTopDefs(actx, opt...)
		} else {
			log.Errorf("side effect: %v is not created from call instruction", i.String())
		}
	case *ssa.Make:
		var values Values
		values = i.AddSelfToTopDefResult(values)
		for key, member := range inst.GetAllMember() {
			value := i.NewValue(member)
			if err := actx.pushObject(i, i.NewValue(key), value); err != nil {
				log.Errorf("push object failed: %v", err)
				continue
			}
			values = append(values, value.getTopDefs(actx, opt...)...)
			actx.popObject()
		}
		return values
	}
	// if if/loop/... control instruction, this innerValue is nil
	if i.innerValue != nil {
		return getMemberCall(i, i.innerValue, actx)
	} else {
		return Values{i}
	}
}
