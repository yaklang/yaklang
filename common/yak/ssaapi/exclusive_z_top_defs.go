package ssaapi

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

const objectAnalyzeLevel = 50

// GetTopDefs desc all of 'Defs' is not used by any other value
func (i *Value) GetTopDefs(opt ...OperationOption) (ret Values) {
	defer func() {
		if r := recover(); r != nil {
			if r == errRecursiveDepth {
				log.Warnf("Value GetTopDefs recursive call too deep, stop it: %s", i.String())
				ret = nil
				return
			}
			if r == context.Canceled {
				log.Warnf("Value GetTopDefs context canceled, stop it: %s", i.String())
				ret = nil
				return
			}
			log.Errorf("Value GetTopDefs panic: %v", r)
			utils.PrintCurrentGoroutineRuntimeStack()
			ret = nil
		}
	}()
	actx := NewAnalyzeContext(opt...)
	if i.ParentProgram != nil {
		actx.Query = func(sf string) Values {
			return i.ParentProgram.SyntaxFlowChain(sf)
		}
	}
	actx.Self = i
	actx.direct = TopDefAnalysis
	ret = i.getTopDefs(actx, opt...)
	if actx.HasUntilNode() {
		ret = actx.untilMatch
	}
	if ret.Count() > dataflowValueLimit {
		log.Warnf("Value TopDef too many: %d:\n\t%s", ret.Count(), i.StringWithRange())
		return nil
	}
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

func resolveMembersForKey(actx *AnalyzeContext, object, key, exclude *Value, allowTypeFallback bool) Values {
	if object == nil || key == nil {
		return nil
	}
	matches := filterOutMember(object.lookupMembersOnObject(key), exclude)
	if len(matches) == 0 && allowTypeFallback {
		matches = filterOutMember(object.lookupMembersOnType(key), exclude)
	}
	if len(matches) == 0 && allowTypeFallback {
		matches = filterOutMember(object.queryMemberCandidates(actx, key), exclude)
	}
	return matches
}

func isConstructorLikeMemberValue(callee *Value) bool {
	if callee == nil || !callee.IsMember() || callee.getValue() == nil {
		return false
	}
	for _, pair := range ssa.GetObjectKeyPairs(callee.getValue()) {
		calleeObj := callee.NewValue(pair.Object)
		calleeKey := callee.NewValue(pair.Key)
		if calleeObj == nil || calleeKey == nil {
			continue
		}
		// Heuristic: treat `obj.<key>` as "constructor-like" when the member key matches
		// the object's identifier/name after stripping common prefixes used in SSA values.
		// This helps approximate patterns like `ClassName(...)` / `new ClassName(...)`
		// across languages where we don't have an explicit constructor call edge.
		keyName := ssa.GetKeyString(calleeKey.getValue())
		for _, candidate := range []string{calleeObj.GetName(), calleeObj.GetVerboseName(), calleeObj.String()} {
			candidate = strings.TrimSpace(candidate)
			candidate = strings.TrimPrefix(candidate, "Undefined-")
			candidate = strings.TrimPrefix(candidate, "ExternLib-")
			if candidate == keyName {
				return true
			}
		}
	}
	return false
}

func isConstructorLikeObjectCall(value *Value) bool {
	if value == nil || value.getValue() == nil {
		return false
	}
	callInst, ok := ssa.ToCall(value.getValue())
	if !ok || callInst == nil || callInst.Method <= 0 {
		return false
	}
	calleeInst, ok := callInst.GetValueById(callInst.Method)
	if !ok || calleeInst == nil {
		return false
	}
	return isConstructorLikeMemberValue(value.NewValue(calleeInst))
}

func isDestructorLikeValue(value *Value) bool {
	if value == nil {
		return false
	}
	if raw := value.getValue(); raw != nil {
		for _, pair := range ssa.GetObjectKeyPairs(raw) {
			if strings.Contains(strings.ToLower(ssa.GetKeyString(pair.Key)), "destructor") {
				return true
			}
		}
	}
	name := strings.ToLower(value.GetName())
	verboseName := strings.ToLower(value.GetVerboseName())
	return strings.Contains(name, "destructor") || strings.Contains(verboseName, "destructor")
}

func shouldFallbackToObjectTopDefs(value *Value) bool {
	if value == nil {
		return false
	}
	switch value.GetSSAInst().(type) {
	case *ssa.Phi, *ssa.SideEffect:
		return false
	case *ssa.Make:
		rawType := GetBareType(value.GetType())
		if rawType == nil {
			return false
		}
		_, ok := ssa.ToBluePrintType(rawType)
		return ok
	default:
		return true
	}
}

func (i *Value) visitedDefs(actx *AnalyzeContext, opt ...OperationOption) (result Values) {
	var vals Values
	if i.getValue() == nil {
		return vals
	}
	for _, def := range i.getValue().GetValues() {
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

	if maskable, ok := i.getValue().(ssa.Maskable); ok {
		if len(maskable.GetMask()) == 0 {
			if i.IsMember() {
				if filtered := filterOutMember(vals, i); len(filtered) > 0 {
					return filtered
				}
			}
			return vals
		}
		// 拿到上次递归的节点
		last := actx.getLastRecursiveNode()
		var shadow *Value
		// 新建个ssa.Value和i一样的ssaapi.Value,
		// 用以作为下个topdef的effecton的边
		// 而不影响i作为结果result有多出来的边
		if last != nil {
			shadow = last.NewValue(i.getValue())
		} else {
			shadow = i.NewValue(i.getValue())
		}
		for _, def := range maskable.GetMask() {
			if ret := shadow.NewValue(def).getTopDefs(actx, opt...); len(ret) > 0 {
				vals = append(vals, ret...)
			}
		}
	}
	if i.IsMember() {
		if filtered := filterOutMember(vals, i); len(filtered) > 0 {
			return filtered
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

	// if inst, ok := ssa.ToLazyInstruction(i.getValue()); ok {
	// 	var ok bool
	// 	i.innerValue, ok = inst.Self().(ssa.Value)
	// 	if !ok {
	// 		log.Errorf("BUG: %T is not ssa.Value", inst.Self())
	// 		return Values{}
	// 	}
	// 	return i.getTopDefs(actx, opt...)
	// }

	// if not shadow value return i self
	i = actx.CovertShadowValue(i)

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

	if actx.isUntilNode(i) {
		return Values{i}
	}

	checkObject := func() Values {
		var ret Values
		obj, key, member := actx.getCurrentObject()
		if obj != nil && key != nil && i.IsObject() {
			matches := resolveMembersForKey(actx, i, key, member, i.GetId() == obj.GetId())
			for index, m := range matches {
				if index == 0 {
					actx.popObject()
				}
				if m != nil {
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
		if actx._objectStack.Len() < objectAnalyzeLevel {
			if value.IsMember() {
				calledAsMethod := lo.SomeBy(apiValue.GetUsers(), func(user *Value) bool {
					call, ok := ssa.ToCall(user.getValue())
					return ok && call != nil && call.Method == apiValue.GetId()
				})
				resolveMembersFromObjectDefs := func(obj, key *Value) Values {
					if obj == nil || key == nil {
						return nil
					}
					objectDefs := filterOutMember(obj.getTopDefs(actx, opt...), i)
					ret := make(Values, 0)
					for _, objectDef := range objectDefs {
						if objectDef == nil || !objectDef.IsObject() {
							continue
						}
						matched := resolveMembersForKey(actx, objectDef, key, i, true)
						for _, member := range matched {
							ret = append(ret, member.getTopDefs(actx, opt...)...)
						}
					}
					return MergeValues(ret)
				}
				var results Values
				for _, pair := range ssa.GetObjectKeyPairs(value) {
					obj := i.NewValue(pair.Object)
					key := i.NewValue(pair.Key)
					if utils.IsNil(obj) || utils.IsNil(key) {
						continue
					}
					if calledAsMethod && !ValueCompare(obj, i) {
						if obj.IsMember() && !ValueCompare(i, actx.Self) {
							results = append(results, i)
						}
						results = append(results, filterOutMember(obj.getTopDefs(actx, opt...), i)...)
					}
					if obj.IsObject() {
						if !calledAsMethod {
							switch obj.GetSSAInst().(type) {
							case *ssa.Phi, *ssa.SideEffect:
								results = append(results, resolveMembersFromObjectDefs(obj, key)...)
							}
						}
						if len(results) == 0 {
							results = append(results, actx.withObject(obj, key, i, func() Values {
								return obj.getTopDefs(actx, opt...)
							})...)
						}
					}
					if len(results) == 0 {
						results = append(results, filterOutMember(obj.lookupMembersOnType(key), i)...)
					}
					if len(results) == 0 {
						results = append(results, filterOutMember(obj.queryMemberCandidates(actx, key), i)...)
					}
					if len(results) == 0 && shouldFallbackToObjectTopDefs(obj) {
						results = append(results, filterOutMember(obj.getTopDefs(actx, opt...), i)...)
					}
					if !calledAsMethod && isConstructorLikeObjectCall(obj) {
						for _, user := range filterOutMember(obj.GetUsers(), i) {
							if isDestructorLikeValue(user) {
								continue
							}
							results = append(results, user.getTopDefs(actx, opt...)...)
						}
					}
				}
				results = filterOutDestructor(filterOutMember(MergeValues(results), i))
				if len(results) == 0 {
					for _, pair := range ssa.GetObjectKeyPairs(value) {
						obj := i.NewValue(pair.Object)
						if utils.IsNil(obj) || ValueCompare(obj, i) || !shouldFallbackToObjectTopDefs(obj) {
							continue
						}
						results = append(results, filterOutMember(obj.getTopDefs(actx, opt...), i)...)
					}
					results = filterOutDestructor(filterOutMember(MergeValues(results), i))
				}
				if len(results) == 0 && !ValueCompare(i, actx.Self) {
					results = append(results, i)
				}
				return results
			}
		}
		return i.visitedDefs(actx, opt...)
	}
	switch inst := i.getValue().(type) {
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
		// For FreeValue Parameter, check GetDefault() which may contain the actual function
		// This handles cases like mutual recursion where the method is a captured variable
		// But skip this for self-recursion (same function calling itself via FreeValue)
		// because we want to track the arguments, not just the return values
		if !isFunc {
			if param, ok := ssa.ToParameter(calleeInst); ok && param.IsFreeValue {
				if defVal := param.GetDefault(); defVal != nil {
					if defFunc, ok := ssa.ToFunction(defVal); ok {
						// Check if this is self-recursion by comparing the function
						// where this call is made with the function being called
						currentFunc := inst.GetFunc()
						if currentFunc != nil && currentFunc.GetId() == defFunc.GetId() {
							// Self-recursion: don't enter the function, let default branch handle it
							// This ensures we track arguments like Undefined-a
						} else {
							// Mutual recursion or other cases: enter the function
							fun = defFunc
							isFunc = true
						}
					}
				}
			}
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
			constructorLikeMember := isConstructorLikeMemberValue(callee)
			hasMeaningfulArg := false
			calleeObject := (*Value)(nil)
			if callee != nil {
				calleeObject = callee.GetObject()
			}
			for _, argID := range inst.Args {
				argValue, ok := inst.GetValueById(argID)
				if !ok || argValue == nil {
					continue
				}
				arg := i.NewValue(argValue)
				if arg == nil {
					continue
				}
				if calleeObject == nil || !ValueCompare(arg, calleeObject) {
					hasMeaningfulArg = true
					break
				}
			}
			keepSelfAsTop := i.IsObject() && callee != nil && callee.IsMember() && !constructorLikeMember && hasMeaningfulArg
			var results Values
			if keepSelfAsTop {
				results = append(results, i)
			}
			if callee != nil && callee.IsMember() {
				if calleeObject := callee.GetObject(); calleeObject != nil && calleeObject.IsMember() && !ValueCompare(callee, i) {
					results = append(results, callee)
				}
			}
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
			shouldKeepSelfDepend := func(def *Value) bool {
				if def == nil {
					return false
				}
				if def.IsConstInst() {
					return false
				}
				if def.IsUndefined() && def.IsMember() {
					return false
				}
				return true
			}
			shouldKeepDirectArg := func(node *Value) bool {
				if node == nil {
					return false
				}
				if callee != nil && callee.IsFunction() {
					return false
				}
				if node.IsConstInst() {
					return false
				}
				return node.IsMember()
			}
			for index, subNode := range nodes {
				if subNode == nil {
					continue
				}
				if index > 0 && shouldKeepDirectArg(subNode) {
					results = append(results, subNode)
				}
				vals := subNode.getTopDefs(actx, opt...)
				if keepSelfAsTop {
					for _, def := range vals {
						if shouldKeepSelfDepend(def) {
							i.AppendDependOn(def)
						}
					}
				}
				results = append(results, vals...)
			}
			if keepSelfAsTop {
				var toRemove []*Value
				for _, dep := range i.GetDependOn() {
					if !shouldKeepSelfDepend(dep) {
						toRemove = append(toRemove, dep)
					}
				}
				i.RemoveDependOn(toRemove...)
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
						val, ok := ssa.GetLatestMemberByKeyString(traceValue, retIndexRawStr)
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
			fun, ok := ssa.ToFunction(value.getValue())
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
		funVal := i.GetFunction()
		if funVal == nil {
			return Values{i}
		}

		fun, ok := ssa.ToFunction(funVal.getInstruction())
		if !ok || fun == nil {
			return Values{i}
		}

		paraValue, ok := inst.GetFormalParam(fun)
		if !ok || paraValue == nil {
			return Values{i}
		}
		para, ok := ssa.ToParameter(paraValue)
		if !ok || para == nil {
			return Values{i}
		}

		memberKey, ok := inst.GetValueById(inst.MemberCallKey)
		if !ok {
			memberKey = nil
		}
		memberKeyValue := i.NewValue(memberKey)

		getParameter := func() Values {
			paraValue := i.NewValue(para)
			if paraValue == nil {
				return Values{}
			}
			if memberKeyValue != nil {
				if ret := actx.withObject(paraValue, memberKeyValue, i, func() Values {
					return filterOutMember(paraValue.getTopDefs(actx, opt...), i)
				}); len(ret) > 0 {
					return ret
				}
				matched := filterOutMember(paraValue.lookupMembersOnType(memberKeyValue), i)
				if len(matched) == 0 {
					matched = filterOutMember(paraValue.queryMemberCandidates(actx, memberKeyValue), i)
				}
				if len(matched) > 0 {
					return lo.FlatMap(matched, func(item *Value, _ int) []*Value {
						return item.getTopDefs(actx, opt...)
					})
				}
			}
			return paraValue.getTopDefs(actx, opt...)
		}
		resolveActualObjectMember := func(actualParam ssa.Value) Values {
			if utils.IsNil(actualParam) || memberKeyValue == nil {
				return nil
			}
			actualObj := i.NewValue(actualParam)
			if actualObj == nil {
				return nil
			}
			matched := resolveMembersForKey(actx, actualObj, memberKeyValue, i, true)
			ret := make(Values, 0, len(matched))
			for _, member := range matched {
				ret = append(ret, member.getTopDefs(actx, opt...)...)
			}
			return MergeValues(ret)
		}
		getActualValueByCall := func(called *Value) Values {
			if called == nil {
				return nil
			}
			calledInstance, ok := ssa.ToCall(called.getValue())
			if !ok {
				log.Warnf("BUG: Parameter getActualValueByCall called is not callInstruction %s", called.GetOpcode())
				return Values{}
			}

			if actualMember, ok := inst.GetActualCallParam(calledInstance); ok {
				traced := i.NewValue(actualMember)
				if traced != nil && actx.needCrossProcess(i, traced) {
					if ret := traced.getTopDefs(actx, opt...); len(ret) > 0 {
						return ret
					}
				}
			}

			if para.FormalParameterIndex >= len(calledInstance.Args) {
				return Values{}
			}
			actualParam, ok := calledInstance.GetValueById(calledInstance.Args[para.FormalParameterIndex])
			if !ok || utils.IsNil(actualParam) {
				return Values{}
			}
			if ret := resolveActualObjectMember(actualParam); len(ret) > 0 {
				return ret
			}
			traced := i.NewValue(actualParam)
			if traced != nil && actx.needCrossProcess(i, traced) {
				if memberKeyValue != nil {
					if ret := actx.withObject(traced, memberKeyValue, i, func() Values {
						return traced.getTopDefs(actx, opt...)
					}); len(ret) > 0 {
						return ret
					}
				}
				if ret := filterOutMember(traced.getTopDefs(actx, opt...), i); len(ret) > 0 {
					return ret
				}
			}
			return Values{}
		}

		getLastCall := func() *Value {
			called := actx.getLastCauseCall(TopDefAnalysis)
			if called != nil {
				actx.setRollBack()
			}
			return called
		}

		called := getLastCall()
		result = append(result, getActualValueByCall(called)...)

		if actx.AllowIgnoreCallStack() && len(result) == 0 {
			call2fun := funVal.GetCalledBy()
			for index, call := range call2fun {
				if index > dataflowValueLimit {
					log.Warnf("Function %s CalledBy too many: %d", funVal.StringWithRange(), len(call2fun))
					break
				}
				val := getActualValueByCall(call)
				result = append(result, val...)
			}
		}

		if len(result) == 0 {
			return getParameter()
		}
		return MergeValues(result)
	case *ssa.Parameter:
		getCalledByValue := func(called *Value, isInners ...bool) Values {
			if called == nil {
				return nil
			}
			isInner := true
			if len(isInners) > 0 {
				isInner = isInners[0]
			}
			calledInstance, ok := ssa.ToCall(called.getValue())
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
					return getMemberCall(i, i.getValue(), actx)
				}
			} else {
				// parameter
				if inst.FormalParameterIndex >= len(calledInstance.Args) {
					log.Debugf("formal parameter index: %d is out of range", inst.FormalParameterIndex)
					return getMemberCall(i, i.getValue(), actx)
				}
				argID := calledInstance.Args[inst.FormalParameterIndex]
				// Prefer resolving actual argument in the call-site scope first.
				actualParam, ok = calledInstance.GetValueById(argID)
				if !ok {
					// Fallback to current instruction scope for compatibility.
					actualParam, ok = inst.GetValueById(argID)
					if !ok {
						actualParam = nil
					}
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
				for index, call := range call2fun {
					if index > dataflowValueLimit {
						log.Warnf("Function %s CalledBy too many: %d", fun.StringWithRange(), len(call2fun))
						break
					}
					val := getCalledByValue(call, true)
					vals = append(vals, val...)
				}
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
		if currentObject, currentKey, currentMember := actx.getCurrentObject(); currentObject != nil && currentKey != nil && currentObject.GetId() == i.GetId() {
			members := resolveMembersForKey(actx, i, currentKey, currentMember, true)
			return lo.FlatMap(members, func(item *Value, _ int) []*Value {
				return item.getTopDefs(actx, opt...)
			})
		}
		var values Values
		values = append(values, i)
		for _, pair := range ssa.GetMemberPairs(inst) {
			value := i.NewValue(pair.Member)
			keyValue := i.NewValue(pair.Key)
			if value == nil || keyValue == nil {
				continue
			}
			values = append(values, actx.withObject(i, keyValue, value, func() Values {
				return value.getTopDefs(actx, opt...)
			})...)
		}
		return values
	case *ssa.ExternLib:
		// ExternLib represents external library references, which don't support dataflow analysis
		// Return the current value itself
		return Values{i}
	case *ssa.BasicBlock:
		// BasicBlock is a control flow structure, not a value for dataflow analysis
		// Return the current value itself
		return Values{i}
	case *ssa.BinOp:
		// Binary operations: track the operands X and Y
		var results Values
		if x, ok := inst.GetValueById(inst.X); ok && x != nil {
			if xVal := i.NewValue(x); xVal != nil {
				results = append(results, xVal.getTopDefs(actx, opt...)...)
			}
		}
		if y, ok := inst.GetValueById(inst.Y); ok && y != nil {
			if yVal := i.NewValue(y); yVal != nil {
				results = append(results, yVal.getTopDefs(actx, opt...)...)
			}
		}
		if len(results) == 0 {
			return Values{i}
		}
		return results
	case *ssa.UnOp:
		// Unary operations: track the operand X
		if x, ok := inst.GetValueById(inst.X); ok && x != nil {
			if xVal := i.NewValue(x); xVal != nil {
				return xVal.getTopDefs(actx, opt...)
			}
		}
		return Values{i}
	case *ssa.Next:
		// Next operations: track the iterator
		if iter, ok := inst.GetValueById(inst.Iter); ok && iter != nil {
			if iterVal := i.NewValue(iter); iterVal != nil {
				return iterVal.getTopDefs(actx, opt...)
			}
		}
		return Values{i}
	default:
		log.Debugf("BUG: %T is not supported in getTopDefs, using fallback", inst)
	}
	// if if/loop/... control instruction, this innerValue is nil
	if i.getValue() != nil {
		return getMemberCall(i, i.getValue(), actx)
	} else {
		return Values{i}
	}
}
