package ssaapi

import (
	"fmt"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// GetTopDefs desc all of 'Defs' is not used by any other value
func (i *Value) GetTopDefs(opt ...OperationOption) (ret Values) {
	defer func() {
		if r := recover(); r != nil {
			if r == recursiveError {
				log.Warnf("Value GetTopDefs recursive call too deep, stop it: %s", i.String())
				ret = nil
				return
			}
			log.Errorf("Value GetTopDefs panic: %v", r)
			utils.PrintCurrentGoroutineRuntimeStack()
			ret = nil
		}
	}()
	actx := NewAnalyzeContext(opt...)
	actx.Self = i
	actx.direct = TopDefAnalysis
	ret = i.getTopDefs(actx, opt...)
	ret = MergeValues(ret)
	return
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
		if ret := i.NewValue(def).getTopDefs(actx, opt...); len(ret) > 0 {
			vals = append(vals, ret...)
		}
	}
	if len(vals) == 0 {
		vals = append(vals, i)
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
			shadow = last.NewValue(i.innerValue)
		} else {
			shadow = i.NewValue(i.innerValue)
		}
		for _, def := range maskable.GetMask() {
			if ret := shadow.NewValue(def).getTopDefs(actx, opt...); len(ret) > 0 {
				vals = append(vals, ret...)
			}
		}
	}
	return vals
}

func (i *Value) getTopDefs(actx *AnalyzeContext, opt ...OperationOption) (result Values) {

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
	var shouldExit bool
	var recoverStack func()
	shouldExit, recoverStack = actx.check(i)
	defer recoverStack()
	defer func() {
		actx.SavePath(result)
	}()

	if shouldExit {
		return Values{i}
	}
	var err error
	err = actx.hook(i)
	if err != nil {
		return Values{i}
	}

	checkObject := func() Values {
		var ret Values
		obj, key, member := actx.getCurrentObject()
		if obj != nil && i.IsObject() && i.GetId() != obj.GetId() {
			for i, m := range i.GetMember(key) {
				if i == 0 {
					actx.popObject()
				}
				if m != nil && !ValueCompare(m, member) {
					ret = append(ret, m.getTopDefs(actx, opt...)...)
				}
			}
		}
		return ret
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
			if utils.IsNil(obj) || utils.IsNil(key) {
				return nil
			}
			if err := actx.pushObject(obj, key, i); err != nil {
				return i.visitedDefs(actx, opt...)
			}

			results := obj.getTopDefs(actx, opt...)
			if len(results) == 0 && !ValueCompare(i, actx.Self) {
				results = append(results, i)
			}
			return results
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
			ret := i.NewValue(cond).getTopDefs(actx, opt...)
			result = append(result, ret...)
		}
		return result
	case *ssa.Call:
		calleeId := inst.Method
		if calleeId <= 0 {
			return Values{i} // return self
		}
		calleeInst, ok := inst.GetValueById(calleeId)
		if !ok {
			return Values{i} // return self
		}

		fun, isFunc := ssa.ToFunction(calleeInst)
		if !isFunc && calleeInst.GetReference() != nil {
			fun, isFunc = ssa.ToFunction(calleeInst.GetReference())
		}

		switch {
		case isFunc && !fun.IsExtern():
			callee := i.NewValue(fun)
			callee.SetContextValue(ANALYZE_RUNTIME_CTX_TOPDEF_CALL_ENTRY, i)
			if objectContext := actx.CurrentObjectStack(); objectContext != nil && ValueCompare(objectContext.object, i) {
				callee.SetContextValue(ANALYZE_RUNTIME_CTX_TOPDEF_CALL_ENTRY_TRACE_INDEX, objectContext.key)
			}
			return callee.getTopDefs(actx, opt...)
		default:
			callee := i.NewValue(calleeInst)
			nodes := Values{callee}
			for _, val := range inst.Args {
				val, ok := inst.GetValueById(val)
				if ok && val != nil {
					arg := i.NewValue(val)
					if arg != nil {
						nodes = append(nodes, arg)
					}
				}
			}
			for _, value := range inst.Binding {
				value, ok := inst.GetValueById(value)
				if ok && value != nil {
					arg := i.NewValue(value)
					if arg != nil {
						nodes = append(nodes, arg)
					}
				}
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
				for _, retId := range inst.Return {
					retInst, ok := inst.GetValueById(retId)
					if !ok {
						continue
					}
					retIns, ok := ssa.ToReturn(retInst)
					if !ok {
						log.Warnf("BUG: %T is not *Return", retInst)
						continue
					}
					for idx, traceId := range retIns.Results {
						if idx == targetIdx {
							traceVal, ok := inst.GetValueById(traceId)
							if ok && traceVal != nil {
								topDefValue := i.NewValue(traceVal)
								if topDefValue != nil {
									traceRets = append(traceRets, topDefValue)
								}
							}
						}
					}
				}
				return lo.FlatMap(traceRets, func(item *Value, index int) []*Value {
					return item.getTopDefs(actx, opt...)
				})
			} else {
				// string literal member
				var traceRets Values
				for _, retId := range inst.Return {
					retInst, ok := inst.GetValueById(retId)
					if !ok {
						continue
					}
					retIns, ok := ssa.ToReturn(retInst)
					if !ok {
						log.Warnf("BUG: %T is not *Return", retInst)
						continue
					}
					for _, traceId := range retIns.Results {
						traceValue, ok := inst.GetValueById(traceId)
						if !ok {
							continue
						}
						val, ok := traceValue.GetStringMember(retIndexRawStr)
						if ok && val != nil {
							topDefValue := i.NewValue(val)
							if topDefValue != nil {
								traceRets = append(traceRets, topDefValue)
							}
							// trace mask ?
							// TODO: use scope when scope can load from database
							// if len(inst.Blocks) > 0 {
							// 	name, ok := ssa.CombineMemberCallVariableName(traceValue, ssa.NewConst(retIndexRawStr))
							// 	if ok {
							// 		lastBlockRaw, _ := lo.Last(inst.Blocks)
							// 		lastBlock, ok := inst.GetBasicBlockByID(lastBlockRaw)
							// 		if ok && lastBlock != nil {
							// 			variableInstance := lastBlock.ScopeTable.ReadVariable(name)
							// 			_ = variableInstance.String()
							// 		}
							// 	}
							// }
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
			for _, retId := range fun.Return {
				retInst, ok := fun.GetValueById(retId)
				if !ok {
					continue
				}
				for _, subVal := range retInst.GetValues() {
					if ret := value.NewValue(subVal).getTopDefs(actx, opt...); len(ret) > 0 {
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
		return vals
	case *ssa.ParameterMember:
		var vals Values
		getParameter := func() Values {
			fun := i.GetFunction()
			if fun == nil {
				return Values{i}
			}
			switch inst.MemberCallKind {
			case ssa.MoreParameterMember:
				if para := fun.GetParameter(inst.MemberCallObjectIndex); para != nil {
					memberKey, ok := inst.GetValueById(inst.MemberCallKey)
					if !ok {
						memberKey = nil
					}
					actx.pushObject(para, i.NewValue(memberKey), i.NewValue(ssa.NewConst("")))
					return para.getTopDefs(actx, opt...)
				}
			case ssa.ParameterMemberCall:
				if para := fun.GetParameter(inst.MemberCallObjectIndex); para != nil {
					memberKey, ok := inst.GetValueById(inst.MemberCallKey)
					if !ok {
						memberKey = nil
					}
					actx.pushObject(para, i.NewValue(memberKey), i.NewValue(ssa.NewConst("")))
					return para.getTopDefs(actx, opt...)
				}
			case ssa.FreeValueMemberCall:
				if fv := fun.GetFreeValue(inst.MemberCallObjectName); fv != nil {
					memberKey, ok := inst.GetValueById(inst.MemberCallKey)
					if !ok {
						memberKey = nil
					}
					actx.pushObject(fv, i.NewValue(memberKey), i.NewValue(ssa.NewConst("")))
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
			if inst.FormalParameterIndex >= len(calledInstance.ArgMember) {
				return getParameter()
			}
			actualParam, ok := inst.GetValueById(calledInstance.ArgMember[inst.FormalParameterIndex])
			if !ok {
				actualParam = nil
			}
			traced := i.NewValue(actualParam)
			if !actx.needCrossProcess(i, traced) {
				return Values{}
			}
			ret := traced.getTopDefs(actx, opt...)
			if !actx.needCrossProcess(i, traced) {
				ret = append(ret, i)
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
				log.Debugf("BUG: Parameter getCalledByValue called is not callInstruction %s", called.GetOpcode())
				return Values{}
			}

			var actualParam ssa.Value
			if inst.IsFreeValue {
				// free value
				if tmp := inst.GetDefault(); tmp != nil && !isInner {
					actualParam = tmp
				} else if binding, ok := calledInstance.Binding[inst.GetName()]; ok && isInner {
					actualParam, ok = inst.GetValueById(binding)
					if !ok {
						actualParam = nil
					}
				} else {
					log.Errorf("free value: %v is not found in binding", inst.GetName())
					return getMemberCall(i, i.innerValue, actx)
				}
			} else {
				// parameter
				if inst.FormalParameterIndex >= len(calledInstance.Args) {
					log.Debugf("formal parameter index: %d is out of range", inst.FormalParameterIndex)
					return getMemberCall(i, i.innerValue, actx)
				}
				actualParam, ok = inst.GetValueById(calledInstance.Args[inst.FormalParameterIndex])
				if !ok {
					actualParam = nil
				}
			}
			traced := i.NewValue(actualParam)
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
				call2fun.ForEach(func(call *Value) {
					val := getCalledByValue(call, true)
					vals = append(vals, val...)
				})
			}
		}

		if len(vals) == 0 {
			if i.IsFreeValue() && inst.GetDefault() != nil {
				vals = append(vals, i.NewValue(inst.GetDefault()))
			} else {
				vals = append(vals, i)
			}
		}
		return vals
	case *ssa.SideEffect:
		callIns := inst.CallSite
		if callIns >= 0 {
			v, ok := inst.GetValueById(inst.Value)
			if !ok {
				v = nil
			}
			topDefValue := i.NewValue(v)
			return topDefValue.getTopDefs(actx, opt...)
		} else {
			log.Errorf("side effect: %v is not created from call instruction", i.String())
		}
	case *ssa.Make:
		var values Values
		values = append(values, i)
		var allmember map[ssa.Value]ssa.Value
		allmember = inst.GetAllMember()
		for key, member := range allmember {
			value := i.NewValue(member)
			if err := actx.pushObject(i, i.NewValue(key), value); err != nil {
				//log.Errorf("push object failed: %v", err)
				// continue
			} else {
				var vs Values
				vs = value.getTopDefs(actx, opt...)
				values = append(values, vs...)
				actx.popObject()
			}
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
