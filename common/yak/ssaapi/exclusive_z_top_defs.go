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

func (v Values) GetBottomUses() Values {
	ret := make(Values, 0)
	for _, sub := range v {
		ret = append(ret, sub.GetBottomUses()...)
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
	for _, def := range i.node.GetValues() {
		if ret := NewValue(def).AppendEffectOn(i).getTopDefs(actx); len(ret) > 0 {
			vals = append(vals, ret...)
		}
	}

	if len(vals) <= 0 {
		vals = append(vals, i)
	}
	if maskable, ok := i.node.(ssa.Maskable); ok {
		for _, def := range maskable.GetMask() {
			if ret := NewValue(def).AppendEffectOn(i).getTopDefs(actx); len(ret) > 0 {
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
	if actx.config.HookEveryNode != nil {
		if err := actx.config.HookEveryNode(i); err != nil {
			log.Errorf("hook-every-node error: %v", err)
			return Values{}
		}
	}

	{
		obj, key, member := actx.GetCurrentObject()
		_ = obj
		_ = key
		_ = member
		if obj != nil && i.IsObject() && i != obj {
			if m := i.GetMember(key); m != nil {
				actx.PopObject()
				return m.getTopDefs(actx, opt...)
			}
		}
	}
	if i.IsMember() && !actx.TheMemberShouldBeVisited(i) {
		return Values{}
	}

	getMemberCall := func(value ssa.Value, actx *AnalyzeContext) Values {
		if !value.HasValues() && value.IsMember() {
			obj := NewValue(value.GetObject())
			key := NewValue(value.GetKey())
			actx.PushObject(obj, key, i)

			ret := obj.getTopDefs(actx, opt...)
			if !ValueCompare(i, actx.Self) {
				ret = append(ret, i)
			}
			return ret
		}
		return i.visitedDefsDefault(actx)
	}

	switch inst := i.node.(type) {
	case *ssa.Undefined:
		// ret[n]
		return getMemberCall(inst, actx)
	case *ssa.Phi:
		if !actx.ThePhiShouldBeVisited(i) {
			// phi is visited...
			return Values{}
		}
		actx.VisitPhi(i)
		return getMemberCall(inst, actx)
	case *ssa.Call:
		caller := inst.Method
		if caller == nil {
			return Values{i} // return self
		}

		// TODO: trace the specific return-values
		callerValue := NewValue(caller)
		callerFunc, isFunc := ssa.ToFunction(caller)
		if !isFunc {
			i.AppendDependOn(callerValue)
			var nodes = Values{callerValue}
			for _, val := range inst.Args {
				arg := NewValue(val)
				i.AppendDependOn(arg)
				nodes = append(nodes, arg)
			}
			for _, value := range inst.Binding {
				arg := NewValue(value)
				i.AppendDependOn(arg)
				nodes = append(nodes, arg)
			}
			var results Values
			for _, subNode := range nodes {
				//getTopDefs(nil,opt...)第一个参数指定为nil
				//提供一个新的上下文，避免指向新的actx.self导致多余的结果
				vals := subNode.getTopDefs(nil, opt...).AppendEffectOn(subNode)
				results = append(results, vals...)
			}
			return results
		}
		_ = callerFunc

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
				for _, retIns := range inst.Return {
					for idx, traceVal := range retIns.Results {
						if idx == targetIdx {
							traceRets = append(traceRets, NewValue(traceVal).AppendEffectOn(i))
						}
					}
				}
				return lo.FlatMap(traceRets, func(item *Value, index int) []*Value {
					return item.getTopDefs(actx, opt...)
				})
			} else {
				// string literal member
				var traceRets Values
				for _, retIns := range inst.Return {
					for _, traceVal := range retIns.Results {
						val, ok := traceVal.GetStringMember(retIndexRawStr)
						if ok {
							traceRets = append(traceRets, NewValue(val).AppendEffectOn(i))
							// trace mask ?
							if len(inst.Blocks) > 0 {
								name, ok := ssa.CombineMemberCallVariableName(traceVal, ssa.NewConst(retIndexRawStr))
								if ok {
									lastBlock, _ := lo.Last(inst.Blocks)
									variableInstance := lastBlock.ScopeTable.ReadVariable(name)
									_ = variableInstance.String()
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
				if ret := NewValue(subVal).AppendEffectOn(i).getTopDefs(actx); len(ret) > 0 {
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
			log.Error("parent function is not called by any other function, skip")
			var vals Values
			vals = append(vals, i)
			// 获取ParameterMember的形参定义
			obj := inst.Parameter.GetObject()
			if obj != nil {
				if inst.MemberCallKind == ssa.ParameterMemberCall {
					objValue := NewValue(obj)
					val := objValue.GetFunction().GetParameter(inst.MemberCallObjectIndex)
					vals = append(vals, val)
				} else if inst.MemberCallKind == ssa.FreeValueMemberCall {
					param := inst.GetFunc().FreeValues[obj.GetName()]
					val := NewValue(param)
					vals = append(vals, val)
				}
			}
			return vals
		}
		if !called.IsCall() {
			log.Infof("parent function is not called by any other function, skip (%T)", called)
			return Values{i}
		}
		calledInstance := called.node.(*ssa.Call)

		// parameter
		if inst.FormalParameterIndex >= len(calledInstance.ArgMember) {
			log.Infof("formal parameter member index: %d is out of range", inst.FormalParameterIndex)
			return getMemberCall(i.node, actx)
		}
		actualParam := calledInstance.ArgMember[inst.FormalParameterIndex]
		traced := NewValue(actualParam).AppendEffectOn(called)
		if ret := traced.getTopDefs(actx); len(ret) > 0 {
			return ret
		} else {
			return Values{traced}
		}

	case *ssa.Parameter:
		// 查找被调用函数的TopDef
		getCalledByValue := func(called *Value) Values {
			calledInstance := called.node.(*ssa.Call)

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
			traced := NewValue(actualParam).AppendEffectOn(called)
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
			return Values{NewValue(inst.GetDefault())}
		}
		var vals Values
		called := actx.GetCurrentCall()
		if called == nil {
			// 允许跨类进行TopDef的查找
			if actx.config.AllowCallStack {
				fun := i.GetFunction()
				if fun != nil {
					call2fun := fun.GetCalledBy()
					call2fun.ForEach(func(call *Value) {
						val := getCalledByValue(call)
						vals = append(vals, val...)
					})
					return append(vals, i)
				}
			}
			log.Error("parent function is not called by any other function, skip")
			return Values{i}
		}
		if !called.IsCall() {
			log.Infof("parent function is not called by any other function, skip (%T)", called)
			return Values{i}
		}

		calledByValue := getCalledByValue(called)
		vals = append(vals, calledByValue...)
		if actx.config.AllowCallStack {
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
			return Values{NewValue(ssa.NewUndefined("_")).AppendEffectOn(i)} // no return, use undefined
		}
		return vals.AppendEffectOn(i)
	case *ssa.SideEffect:

		callIns := inst.CallSite
		if callIns != nil {
			err := actx.PushCall(NewValue(callIns).AppendEffectOn(i))
			if err != nil {
				log.Errorf("push call error: %v", err)
			} else {
				defer actx.PopCall()

				v := NewValue(inst.Value).AppendEffectOn(i)
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
			value := NewValue(member)
			values = append(values, value.getTopDefs(actx, opt...)...)
		}
		paramVals := getMakeParamTopDef()
		values = append(values, paramVals...)
		return values
	}
	return getMemberCall(i.node, actx)
}
