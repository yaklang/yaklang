package ssaapi

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

const objectAnalyzeLevel = 50

// memberEntry is one (key, member) pair of an object's member table.
type memberEntry struct {
	key    ssa.Value
	member ssa.Value
	keyID  int64
}

// sortedMemberPairs flattens a member table into a stable order. See the call
// site in the *ssa.Make case: traversal order is observable through the shared
// recursion budget and result cap, so it must not depend on map iteration.
func sortedMemberPairs(members map[ssa.Value]ssa.Value) []memberEntry {
	pairs := make([]memberEntry, 0, len(members))
	for key, member := range members {
		if utils.IsNil(key) || utils.IsNil(member) {
			continue
		}
		pairs = append(pairs, memberEntry{key: key, member: member, keyID: key.GetId()})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].keyID != pairs[j].keyID {
			return pairs[i].keyID < pairs[j].keyID
		}
		return pairs[i].member.GetId() < pairs[j].member.GetId()
	})
	return pairs
}

// isBlueprintValue reports whether a value is a class blueprint rather than a
// data instance. Blueprint members are keyed by the class name (plus companion
// keys such as "<Class>-destructor"), not by field name, so they are not
// candidates for field-wise resolution.
func isBlueprintValue(v *Value) bool {
	if utils.IsNil(v) {
		return false
	}
	typ := v.GetType()
	if utils.IsNil(typ) {
		return false
	}
	_, ok := ssa.ToBluePrintType(GetBareType(typ))
	return ok
}

// isCollectionLikeValue reports whether a value is an array/slice/map style
// container whose "members" are positional or keyed elements rather than named
// fields. A member read on such a value (e.g. `data.toString()` on a char[])
// still needs the whole element set followed, so these keep the enumeration
// path.
func isCollectionLikeValue(v *Value) bool {
	if utils.IsNil(v) {
		return false
	}
	typ := v.GetType()
	if utils.IsNil(typ) {
		return false
	}
	raw := GetBareType(typ)
	if utils.IsNil(raw) {
		return false
	}
	switch raw.GetTypeKind() {
	case ssa.SliceTypeKind, ssa.MapTypeKind, ssa.TupleTypeKind:
		return true
	}
	return false
}

// constKeyText returns the plain string content of a constant key. Member keys
// are rendered with surrounding quotes by the generic key accessors, which
// would defeat prefix comparison, so the constant content is read directly.
func constKeyText(key *Value) (string, bool) {
	if utils.IsNil(key) {
		return "", false
	}
	if raw, ok := key.GetConstValue().(string); ok {
		return raw, true
	}
	text := ssa.GetKeyString(key.getValue())
	if text == "" {
		return "", false
	}
	if unquoted, err := strconv.Unquote(text); err == nil {
		return unquoted, true
	}
	return text, true
}

// resolveKeyedMembers returns the members of an object that a keyed descent
// should follow.
//
// A descent that arrives with an (object, key) context does so because some
// site read `object.key`; the precise answer is that key's member set. Some
// languages additionally synthesise companion members under a key derived from
// it -- PHP registers the destructor as `<Class>-destructor` alongside the
// class member -- and rules rely on those, so derived keys are included too.
//
// Returning nil means the key resolves to nothing here, which tells the caller
// to fall back to walking the object's members.
func resolveKeyedMembers(object, key *Value) []*Value {
	if object == nil || key == nil {
		return nil
	}
	matched := object.GetMember(key)
	if len(matched) == 0 {
		return nil
	}
	raw, ok := constKeyText(key)
	if !ok || raw == "" {
		return matched
	}
	seen := make(map[int64]struct{}, len(matched))
	for _, m := range matched {
		if !utils.IsNil(m) {
			seen[m.GetId()] = struct{}{}
		}
	}
	for _, pair := range ssa.GetMemberPairs(object.getValue()) {
		if utils.IsNil(pair.Key) || utils.IsNil(pair.Member) {
			continue
		}
		pairKey := object.NewValue(pair.Key)
		if utils.IsNil(pairKey) {
			continue
		}
		derived, ok := constKeyText(pairKey)
		if !ok {
			continue
		}
		if derived == raw || !strings.HasPrefix(derived, raw+"-") {
			continue
		}
		member := object.NewValue(pair.Member)
		if utils.IsNil(member) {
			continue
		}
		if _, ok := seen[member.GetId()]; ok {
			continue
		}
		seen[member.GetId()] = struct{}{}
		matched = append(matched, member)
	}
	return matched
}

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
	actx.Self = i
	actx.direct = TopDefAnalysis
	ret = i.getTopDefs(actx, opt...)
	if actx.HasUntilNode() {
		ret = actx.untilMatch
	}
	if ret.Count() > dataflowValueLimit {
		log.Warnf("Value TopDef too many: %d: %s", ret.Count(), i.StringForDataflowWarn())
		if report := actx.widenReport(); report != "" {
			log.Warnf("Value TopDef widening: %s", report)
		}
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
			if utils.IsNil(def) {
				continue
			}
			if ret := shadow.NewValue(def).getTopDefs(actx, opt...); len(ret) > 0 {
				vals = append(vals, ret...)
			}
		}
		// shadow is a pure factory shell used only as a ParentProgram carrier for
		// the mask defs; it is never appended to vals/edges/results itself, so it
		// is unreachable after this loop. Release it.
		releaseValue(shadow)
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
	actx.traceNodeVisit()

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

	if !actx.structAllowValue(i) {
		return Values{}
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

	if actx.isUntilNode(i) {
		return Values{i}
	}

	checkObject := func() Values {
		var ret Values
		obj, key, member := actx.getCurrentObject()
		if obj != nil && i.IsObject() && i.GetId() != obj.GetId() {
			members := i.GetMember(key)
			if len(members) == 0 {
				if raw, ok := key.GetConstValue().(string); ok && strings.HasPrefix(raw, "$") {
					normalizedKey := i.NewValue(ssa.NewConst(strings.TrimPrefix(raw, "$")))
					members = i.GetMember(normalizedKey)
					// normalizedKey is a pure factory shell used only as a lookup
					// key; GetMember never retains it. Release it now that it is
					// unreachable.
					releaseValue(normalizedKey)
				}
			}
			for i, m := range members {
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
		if actx._objectStack.Len() < objectAnalyzeLevel {
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
				isStaticPropertyCarrier := false
				if raw, ok := key.GetConstValue().(string); ok && strings.HasPrefix(raw, "$") {
					isStaticPropertyCarrier = true
				}
				if isStaticPropertyCarrier && len(results) > 1 {
					results = lo.Filter(results, func(item *Value, _ int) bool {
						return !ValueCompare(item, obj)
					})
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
			if utils.IsNil(cond) {
				continue
			}
			ret := i.NewValue(cond).getTopDefs(actx, opt...)
			result = append(result, ret...)
		}
		return result
	case *ssa.Call:
		calleeId := inst.Method
		if calleeId <= 0 {
			return Values{i} // return self
		}
		calleeInst, ok := actx.getResolvedValue(inst, calleeId)
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
			for _, val := range inst.Args {
				val, ok := actx.getResolvedValue(inst, val)
				if ok && val != nil {
					arg := i.NewValue(val)
					if arg != nil {
						nodes = append(nodes, arg)
					}
				}
			}
			for _, value := range inst.Binding {
				value, ok := actx.getResolvedValue(inst, value)
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
					retInst, ok := actx.getResolvedValue(inst, retId)
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
							traceVal, ok := actx.getResolvedValue(inst, traceId)
							if ok && traceVal != nil {
								topDefValue := i.NewValue(traceVal)
								if topDefValue != nil {
									traceRets = append(traceRets, topDefValue)
								}
							}
						}
					}
				}
				// Some frontends represent an out/ref fallback as call[index]. This
				// is especially useful for a forward local-function call, whose body
				// (and therefore pointer side effect) is completed only after the call
				// instruction was emitted. Resolve that indexed member against the
				// finished callee metadata at analysis time; this also survives a DB
				// reload because FunctionType.SideEffects is serialized.
				if functionType, ok := ssa.ToFunctionType(inst.GetType()); ok {
					for _, sideEffect := range functionType.SideEffects {
						if sideEffect == nil || sideEffect.Kind != ssa.PointerSideEffect ||
							sideEffect.MemberCallKind != ssa.ParameterCall || sideEffect.MemberCallObjectIndex != targetIdx {
							continue
						}
						if modified, ok := actx.getResolvedValue(inst, sideEffect.Modify); ok && !utils.IsNil(modified) {
							traceRets = append(traceRets, i.NewValue(modified))
						}
					}
				}
				return lo.FlatMap(traceRets, func(item *Value, index int) []*Value {
					if item == nil {
						return nil
					}
					return item.getTopDefs(actx, opt...)
				})
			} else {
				// string literal member
				var traceRets Values
				for _, retId := range inst.Return {
					retInst, ok := actx.getResolvedValue(inst, retId)
					if !ok {
						continue
					}
					retIns, ok := ssa.ToReturn(retInst)
					if !ok {
						log.Warnf("BUG: %T is not *Return", retInst)
						continue
					}
					for _, traceId := range retIns.Results {
						traceValue, ok := actx.getResolvedValue(inst, traceId)
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
			fun, ok := ssa.ToFunction(value.getValue())
			if !ok {
				return
			}
			for _, retId := range fun.Return {
				retInst, ok := actx.getResolvedValue(fun, retId)
				if !ok {
					continue
				}
				for _, subVal := range retInst.GetValues() {
					if utils.IsNil(subVal) {
						continue
					}
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
		if callEntry, ok := i.GetContextValue(ANALYZE_RUNTIME_CTX_TOPDEF_CALL_ENTRY); ok {
			if call, ok := ssa.ToCall(callEntry.getValue()); ok && call.IsNonVirtual {
				return vals
			}
		}
		// handler child-class function
		for _, child := range inst.GetPointer() {
			if utils.IsNil(child) {
				continue
			}
			handlerReturn(i.NewValue(child))
		}
		return vals
	case *ssa.ParameterMember:
		getParameter := func() Values {
			funVal := i.GetFunction()
			if funVal == nil {
				return Values{i}
			}

			fun, ok := ssa.ToFunction(funVal.getInstruction())
			if !ok || fun == nil {
				return Values{i}
			}

			para, ok := inst.GetFormalParam(fun)
			if !ok || para == nil {
				return Values{i}
			}

			memberKey, ok := actx.getResolvedValue(inst, inst.MemberCallKey)
			if !ok {
				memberKey = nil
			}
			keyVal := i.NewValue(memberKey)
			if keyVal == nil {
				keyVal = i.NewValue(ssa.NewConst(""))
			}
			actx.pushObject(i.NewValue(para), keyVal, i.NewValue(ssa.NewConst("")))
			return i.NewValue(para).getTopDefs(actx, opt...)
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

			// 获取实际传入的参数值
			actualParam, ok := inst.GetActualCallParam(calledInstance)
			if !ok || utils.IsNil(actualParam) {
				return Values{}
			}
			traced := i.NewValue(actualParam)
			if !actx.needCrossProcess(i, traced) {
				return Values{}
			}
			ret := traced.getTopDefs(actx, opt...)
			if len(ret) > 0 {
				return ret
			} else {
				return Values{}
			}
		}

		// 拿上一个调用栈的call
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
			if fun := i.GetFunction(); fun != nil {
				call2fun := fun.GetCalledBy()
				actx.traceCalledByFanout(len(call2fun))
				for index, call := range call2fun {
					if index > dataflowValueLimit {
						log.Warnf("Function %s CalledBy too many: %d", fun.StringForDataflowWarn(), len(call2fun))
						break
					}
					val := getActualValueByCall(call)
					result = append(result, val...)
				}
			}
		}

		if len(result) == 0 {
			return getParameter()
		}
		return result
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
			omittedDefault := false
			if inst.IsFreeValue {
				// free value
				if binding, ok := calledInstance.Binding[inst.GetName()]; ok && isInner {
					// Prefer call-site scope (same as formal parameters): binding id refers to
					// the actual SSA value at the call, which may not resolve on inst alone.
					// TODO(scan-log): binding id may not reload after split-compile flush (GetValueById miss).
					actualParam, ok = actx.getResolvedValue(calledInstance, binding)
					if !ok {
						actualParam, ok = actx.getResolvedValue(inst, binding)
						if !ok {
							actualParam = nil
						}
					}
				}
				// Fallback to GetDefault when the binding is missing or the
				// binding id didn't resolve. This was previously an Errorf +
				// early return that silently dropped taint paths through
				// captured variables (e.g. "template" in tutor, "q" in sonic,
				// "a"/"rd" in GoBlog — thousands of occurrences across projects).
				// GetDefault is the compile-time captured value; using it
				// preserves the dataflow path even when the call-site binding
				// was not populated (known gap in HandleFreeValue / split-compile).
				if utils.IsNil(actualParam) {
					if tmp := inst.GetDefault(); tmp != nil {
						actualParam = tmp
					}
				}
				if utils.IsNil(actualParam) {
					log.Debugf("free value: %v is not found in binding and has no default", inst.GetName())
					return getMemberCall(i, i.getValue(), actx)
				}
			} else {
				// parameter
				if inst.FormalParameterIndex >= len(calledInstance.Args) {
					// Optional parameters keep their declared default on the formal.
					// A call intentionally omits trailing optional arguments, so an
					// out-of-range formal is not necessarily a broken call vector.
					// Resolve the default in the callee program before falling back to
					// the unresolved parameter itself.
					if fallback := inst.GetDefault(); !utils.IsNil(fallback) {
						actualParam = fallback
						omittedDefault = true
					} else {
						log.Debugf("formal parameter index: %d is out of range", inst.FormalParameterIndex)
						return getMemberCall(i, i.getValue(), actx)
					}
				} else {
					argID := calledInstance.Args[inst.FormalParameterIndex]
					// Prefer resolving actual argument in the call-site scope first.
					actualParam, ok = actx.getResolvedValue(calledInstance, argID)
					if !ok {
						// Fallback to current instruction scope for compatibility.
						actualParam, ok = actx.getResolvedValue(inst, argID)
						if !ok {
							actualParam = nil
						}
					}
					// A frontend may need to materialize an omitted default into an
					// earlier call-vector slot so a later named argument stays aligned.
					// The exact declared-default instruction is still callee-local and
					// must not be rejected by the caller/callee crossing guard.
					if fallback := inst.GetDefault(); !utils.IsNil(actualParam) && !utils.IsNil(fallback) && actualParam.GetId() == fallback.GetId() {
						omittedDefault = true
					}
				}
			}
			if utils.IsNil(actualParam) {
				// TODO(scan-log): arg/binding id not in resident cache or DB (split-compile unit boundary).
				return getMemberCall(i, i.getValue(), actx)
			}
			traced := i.NewValue(actualParam)
			// A declared default is local to the callee, so it does not cross a
			// function boundary. Explicit arguments still require the ordinary
			// caller-to-callee guard.
			if !omittedDefault && !actx.needCrossProcess(i, traced) {
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
						log.Warnf("Function %s CalledBy too many: %d", fun.StringForDataflowWarn(), len(call2fun))
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
			v, ok := actx.getResolvedValue(inst, inst.Value)
			if !ok || utils.IsNil(v) {
				return getMemberCall(i, i.getValue(), actx)
			}
			topDefValue := i.NewValue(v)
			return topDefValue.getTopDefs(actx, opt...)
		} else {
			log.Errorf("side effect: %v is not created from call instruction", i.String())
		}
	case *ssa.Make:
		var values Values
		values = append(values, i)

		// Field-sensitive fast path.
		//
		// Reaching a Make with a pushed (object, key) context means some site
		// read `object.key` and the trace followed that field here. Resolving
		// that key directly is both more precise and dramatically cheaper than
		// enumerating every member: with the full walk each sibling field is
		// traced in turn, and each of those traces can re-enter this object.
		//
		// When the key resolves to nothing here, fall through to the original
		// enumeration so nothing that used to resolve is dropped.
		// Only a data object (an instance) can be resolved field-wise. A class
		// blueprint is a namespace: its members live under the class name and
		// under derived keys such as "AA-destructor", and rules rely on those
		// being reached, so blueprints keep the enumeration path.
		if obj, key, member := actx.getCurrentObject(); obj != nil && obj.GetId() == i.GetId() &&
			!isBlueprintValue(i) && !isCollectionLikeValue(i) {
			if matched := resolveKeyedMembers(i, key); len(matched) > 0 {
				for _, m := range matched {
					if utils.IsNil(m) {
						continue
					}
					if ValueCompare(m, member) {
						continue
					}
					if err := actx.pushObject(i, m.GetKey(), m); err != nil {
						continue
					}
					values = append(values, m.getTopDefs(actx, opt...)...)
					actx.popObject()
				}
				return values
			}
		}

		var allmember map[ssa.Value]ssa.Value
		allmember = inst.GetAllMember()
		actx.traceObjectExpansion(len(allmember))
		// Deterministic order: GetAllMember is a map, and Go randomises map
		// iteration. Downstream state (recursion budget, visited sets, the
		// result cap) makes traversal order observable, so an unordered walk
		// makes the same program produce different results run to run. Sorting
		// by key id keeps each member visited exactly once and makes the
		// descent reproducible.
		for _, pair := range sortedMemberPairs(allmember) {
			key, member := pair.key, pair.member
			if utils.IsNil(key) || utils.IsNil(member) {
				continue
			}
			value := i.NewValue(member)
			keyVal := i.NewValue(key)
			if value == nil || keyVal == nil {
				continue
			}
			if err := actx.pushObject(i, keyVal, value); err != nil {
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
		if x, ok := actx.getResolvedValue(inst, inst.X); ok && x != nil {
			if xVal := i.NewValue(x); xVal != nil {
				results = append(results, xVal.getTopDefs(actx, opt...)...)
			}
		}
		if y, ok := actx.getResolvedValue(inst, inst.Y); ok && y != nil {
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
		if x, ok := actx.getResolvedValue(inst, inst.X); ok && x != nil {
			if xVal := i.NewValue(x); xVal != nil {
				return xVal.getTopDefs(actx, opt...)
			}
		}
		return Values{i}
	case *ssa.Next:
		// Next operations: track the iterator
		if iter, ok := actx.getResolvedValue(inst, inst.Iter); ok && iter != nil {
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
