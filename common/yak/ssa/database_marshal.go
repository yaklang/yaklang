package ssa

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func fetchIds(origin any) any {
	var ids []int64
	switch ret := origin.(type) {
	case []Instruction:
		ids = make([]int64, len(ret))
		for i := 0; i < len(ret); i++ {
			ids[i] = ret[i].GetId()
		}
		return ids
	case map[string]Value:
		params := make(map[string]any)
		for k, v := range ret {
			params[k] = v.GetId()
		}
		return params
	case []SwitchLabel:
		results := make([]map[string]int64, len(ret))
		for i := 0; i < len(ret); i++ {
			results[i] = map[string]int64{
				"value": ret[i].Value.GetId(),
				"dest":  ret[i].Dest.GetId(),
			}
		}
		return results
	case []*Parameter:
		ids = make([]int64, len(ret))
		for i := 0; i < len(ret); i++ {
			ids[i] = ret[i].GetId()
		}
		return ids
	case []Value:
		ids = make([]int64, len(ret))
		for i := 0; i < len(ret); i++ {
			ids[i] = ret[i].GetId()
		}
		return ids
	default:
		t := reflect.TypeOf(origin).Kind()
		if t == reflect.Array || t == reflect.Slice {
			ids := make([]int64, 0, reflect.ValueOf(origin).Len())
			for i := 0; i < len(ids); i++ {
				ins, ok := reflect.ValueOf(origin).Index(i).Interface().(Instruction)
				if ok {
					ids[i] = ins.GetId()
				} else {
					ids[i] = 0
				}
			}
			return ids
		}
		log.Warnf("fetchIds: unknown type: %v", reflect.TypeOf(origin).String())
	}
	return ids
}

func marshalExtraInformation(raw Instruction) map[string]any {
	marshalValues := func(vs []Value) []int64 {
		ids := make([]int64, len(vs))
		for index, v := range vs {
			if v == nil {
				log.Errorf("BUG: marshalValues[%s: %s]: nil value in slice", raw, raw.GetRange())
				continue
			}
			ids[index] = v.GetId()
		}
		return ids
	}

	params := make(map[string]any)
	switch ret := raw.(type) {
	case *Function:
		params["params"] = fetchIds(ret.Params)
		params["param_length"] = ret.ParamLength
		freeValues := make(map[string]int64)
		for k, v := range ret.FreeValues {
			freeValues[k] = v.GetId()
		}
		params["is_method"] = ret.isMethod
		params["method_name"] = ret.methodName
		params["free_values"] = freeValues
		params["parameter_members"] = fetchIds(ret.ParameterMembers)
		var sideEffects []map[string]any
		for _, se := range ret.SideEffects {
			element := map[string]any{
				"name":         se.Name,
				"verbose_name": se.VerboseName,
				"modify":       se.Modify.GetId(),
				"forceCreate":  se.forceCreate,
			}
			if se.parameterMemberInner != nil {
				element["object_name"] = se.ObjectName
				element["member_call_kind"] = se.MemberCallKind
				element["member_call_object_index"] = se.MemberCallObjectIndex
				element["member_call_name"] = se.MemberCallObjectName
				if se.MemberCallKey != nil {
					element["member_call_key"] = se.MemberCallKey.GetId()
				}
			}
			sideEffects = append(sideEffects, element)
		}
		params["side_effect"] = sideEffects
		if p := ret.GetParent(); p != nil {
			params["parent"] = p.GetId()
		}
		params["child_funcs"] = fetchIds(ret.ChildFuncs)
		params["return"] = fetchIds(ret.Return)
		params["blocks"] = fetchIds(ret.Blocks)
		if ret.EnterBlock != nil {
			params["enter_block"] = ret.EnterBlock.GetId()
		}
		if ret.ExitBlock != nil {
			params["exit_block"] = ret.ExitBlock.GetId()
		}
		if ret.DeferBlock != nil {
			params["defer_block"] = ret.DeferBlock.GetId()
		}
		var files [][2]string
		params["reference_files"] = files
		params["has_ellipsis"] = ret.hasEllipsis
	case *Assert:
		params["assert_condition_id"] = ret.Cond.GetId()
		if ret.MsgValue != nil {
			params["assert_message_id"] = ret.MsgValue.GetId()
		}
		params["assert_message_string"] = ret.Msg
	case *BasicBlock:
		params["block_id"] = ret.GetId()
		params["block_name"] = ret.GetName()
		params["block_preds"] = fetchIds(ret.Preds)
		params["block_succs"] = fetchIds(ret.Succs)
		params["block_set_reachable"] = ret.setReachable
		params["block_can_be_reached"] = ret.canBeReached
		if ret.Condition != nil {
			if id := ret.Condition.GetId(); id > 0 {
				params["block_condition"] = id
			} else {
				log.Warnf("strange things happening when marshal BasicBlock: invalid condition(%T: %v) id: %v", ret.Condition, ret.Condition.String(), id)
			}
		}
		params["block_insts"] = fetchIds(ret.Insts)
		params["block_phis"] = fetchIds(ret.Phis)
		params["block_finish"] = ret.finish
		if ret.ScopeTable != nil {
			params["block_scope_table"] = ret.ScopeTable.GetPersistentId()
		}
	case *BinOp:
		params["binop_op"] = ret.Op
		if ret.X != nil {
			params["binop_x"] = ret.X.GetId()
		}
		if ret.Y != nil {
			params["binop_y"] = ret.Y.GetId()
		}
	case *Call:
		params["call_method"] = ret.Method.GetId()
		params["call_args"] = marshalValues(ret.Args)
		params["call_binding"] = fetchIds(ret.Binding)
		params["call_arg_member"] = marshalValues(ret.ArgMember)
		params["call_async"] = ret.Async
		params["call_unpack"] = ret.Unpack
		params["call_drop_error"] = ret.IsDropError
		params["call_ellipsis"] = ret.IsEllipsis
	case *ErrorHandler:
		// try-catch-finally-done
		if ret.try != nil {
			params["errorhandler_try"] = ret.try.GetId()
		}
		if len(ret.catchs) != 0 {
			params["errorhandler_catch"] = fetchIds(ret.catchs)
		}
		if ret.final != nil {
			params["errorhandler_finally"] = ret.final.GetId()
		}
		if ret.done != nil {
			params["errorhandler_done"] = ret.done.GetId()
		}
	case *ExternLib:
		log.Warnf("TBD: marshal ExternLib: %v", ret)
		// return nil, utils.Errorf("BUG: ConstInst should not be marshaled")
	case *If:
		if ret.Cond != nil {
			params["if_cond"] = ret.Cond.GetId()
		}
		if ret.True != nil {
			params["if_true"] = ret.True.GetId()
		}
		if ret.False != nil {
			params["if_false"] = ret.False.GetId()
		}
	case *Jump:
		params["jump_to"] = ret.To.GetId()
	case *Loop:
		params["loop_body"] = ret.Body.GetId()
		if ret.Exit != nil {
			params["loop_exit"] = ret.Exit.GetId()
		}
		if ret.Init != nil {
			params["loop_init"] = ret.Init.GetId()
		}
		if ret.Cond != nil {
			params["loop_cond"] = ret.Cond.GetId()
		}
		if ret.Step != nil {
			params["loop_step"] = ret.Step.GetId()
		}
		if ret.Key != nil {
			params["loop_key"] = ret.Key.GetId()
		}
	case *Make:
		if ret.low != nil {
			params["make_low"] = ret.low.GetId()
		}
		if ret.high != nil {
			params["make_high"] = ret.high.GetId()
		}
		if ret.step != nil {
			params["make_step"] = ret.step.GetId()
		}
		if ret.Len != nil {
			params["make_len"] = ret.Len.GetId()
		}
		if ret.Cap != nil {
			params["make_cap"] = ret.Cap.GetId()
		}
	case *Next:
		if ret.Iter != nil {
			params["next_iter"] = ret.Iter.GetId()
		}
		params["next_in_next"] = ret.InNext
	case *Panic:
		if ret.Info != nil {
			params["panic_value"] = ret.Info.GetId()
		}
	case *Parameter:
		params["formalParam_is_freevalue"] = ret.IsFreeValue
		if ret.defaultValue != nil {
			params["formalParam_default"] = ret.defaultValue.GetId()
		}
		params["formalParam_index"] = ret.FormalParameterIndex
	case *ParameterMember:
		params["formalParamMember_index"] = ret.FormalParameterIndex
		params["member_call_kind"] = ret.MemberCallKind
		params["member_call_index"] = ret.MemberCallObjectIndex
		params["member_call_name"] = ret.MemberCallObjectName
		if ret.MemberCallKey != nil {
			params["member_call_key"] = ret.MemberCallKey.GetId()
		}
		// params["member_call_obj"] = ret.GetObject().GetId()
	case *Phi:
		params["phi_edges"] = marshalValues(ret.Edge)
		if ret.CFGEntryBasicBlock != nil {
			params["cfg_entry"] = ret.CFGEntryBasicBlock.GetId()
		}
	case *Return:
		params["return_results"] = marshalValues(ret.Results)
	case *SideEffect:
		params["sideEffect_call"] = ret.CallSite.GetId()
		params["sideEffect_value"] = ret.Value.GetId()
	case *Switch:
		if ret.Cond != nil {
			params["switch_cond"] = ret.Cond.GetId()
		}
		params["switch_label"] = fetchIds(ret.Label)
	case *TypeCast:
		if ret.Value != nil {
			params["typecast_value"] = ret.Value.GetId()
		}
	case *TypeValue:
		// nothing to do
	case *UnOp:
		params["unop_op"] = ret.Op
		if ret.X != nil {
			params["unop_x"] = ret.X.GetId()
		}
	case *Undefined:
		params["undefined_kind"] = ret.Kind
	case *ConstInst:
		params["const_value"] = ret.Const.GetRawValue()
		if ret.Origin != nil {
			params["const_origin"] = ret.Origin.GetId()
		}
	default:
		log.Warnf("marshalExtraInformation: unknown type: %v", reflect.TypeOf(raw).String())
	}
	return params
}

func unmarshalExtraInformation(inst Instruction, ir *ssadb.IrCode) {
	newLazyInstruction := func(input any) Value {
		var id int64
		switch result := input.(type) {
		case int64:
			id = result
		case float64:
			id = int64(result)
		default:
			id = codec.Atoi64(fmt.Sprint(input))
		}

		if id <= 0 {
			log.Infof("unmarshalExtraInformation: invalid id: %v if u want to check why? enable DEBUG=1", id)
			utils.Debug(func() {
				utils.PrintCurrentGoroutineRuntimeStack()
			})
			return nil
		}

		lz, err := NewLazyInstruction(id)
		if err != nil {
			log.Errorf("BUG: unmatched instruction create lazyInstruction: %v", err)
		}
		return lz
	}
	unmarshalValues := func(p any) []Value {
		vs := make([]Value, 0)
		switch ret := p.(type) {
		case []any:
			for _, id := range ret {
				vs = append(vs, newLazyInstruction(id))
			}

		default:
		}
		return vs
	}
	unmarshalInstructions := func(p any) []Instruction {
		vs := make([]Instruction, 0)
		switch ret := p.(type) {
		case []any:
			for _, id := range ret {
				vs = append(vs, newLazyInstruction(id))
			}

		default:
		}
		return vs
	}
	unmarshalMapValues := func(p any) map[string]Value {
		vs := make(map[string]Value)
		switch ret := p.(type) {
		case map[string]any:
			for k, id := range ret {
				vs[k] = newLazyInstruction(id)
			}
		default:
		}
		return vs
	}

	toInt := func(i any) int {
		switch ret := i.(type) {
		case float64:
			return int(ret)
		case int64:
			return int(ret)
		default:
			return codec.Atoi(fmt.Sprint(i))
		}
	}

	toBool := func(i any) bool {
		switch ret := i.(type) {
		case bool:
			return ret
		default:
			res, _ := strconv.ParseBool(fmt.Sprint(i))
			return res
		}
	}

	toString := func(i any) string {
		return codec.AnyToString(i)
	}

	//toInt64 := func(i any) int64 {
	//	switch ret := i.(type) {
	//	case float64:
	//		return int64(ret)
	//	case int64:
	//		return ret
	//	default:
	//		return codec.Atoi64(fmt.Sprint(i))
	//	}
	//}

	params := ir.GetExtraInfo()
	switch ret := inst.(type) {
	case *Assert:
		ret.Cond = newLazyInstruction(params["assert_condition_id"])
		if msg, ok := params["assert_message_id"]; ok {
			ret.MsgValue = newLazyInstruction(msg)
		}
		ret.Msg = params["assert_message_string"].(string)
	case *BasicBlock:
		ret.Preds = unmarshalValues(params["block_preds"])
		ret.Succs = unmarshalValues(params["block_succs"])
		if cond, ok := params["block_condition"]; ok {
			ret.Condition = newLazyInstruction(cond)
		}
		ret.setReachable = codec.Atob(fmt.Sprint(params["block_set_reachable"]))
		ret.canBeReached = codec.Atoi(fmt.Sprint(params["block_can_be_reached"]))
		ret.Insts = unmarshalInstructions(params["block_insts"])
		ret.Phis = unmarshalValues(params["block_phis"])
		ret.finish = toBool(params["block_finish"])
		if scopeTable, ok := params["block_scope_table"]; ok {
			ret.ScopeTable = GetLazyScopeFromIrScopeId(int64(toInt(scopeTable)))
		}
	case *BinOp:
		ret.Op = BinaryOpcode(params["binop_op"].(string))
		if x, ok := params["binop_x"]; ok {
			ret.X = newLazyInstruction(x)
		}
		if y, ok := params["binop_y"]; ok {
			ret.Y = newLazyInstruction(y)
		}
	case *Call:
		ret.Method = newLazyInstruction(params["call_method"])
		ret.Args = unmarshalValues(params["call_args"])
		ret.ArgMember = unmarshalValues(params["call_arg_member"])
		ret.Binding = unmarshalMapValues(params["call_binding"])
		ret.Async = toBool(params["call_async"])
		ret.Unpack = toBool(params["call_unpack"])
		ret.IsDropError = toBool(params["call_drop_error"])
		ret.IsEllipsis = toBool(params["call_ellipsis"])
	case *Next:
		ret.InNext = toBool(params["next_in_next"])
		ret.Iter = newLazyInstruction(params["next_iter"])
	case *Parameter:
		ret.IsFreeValue = params["formalParam_is_freevalue"].(bool)
		if defaultValue, ok := params["formalParam_default"]; ok {
			ret.defaultValue = newLazyInstruction(defaultValue)
		}
		ret.FormalParameterIndex = int(params["formalParam_index"].(float64))
	case *ParameterMember:
		ret.FormalParameterIndex = int(params["formalParamMember_index"].(float64))
		ret.MemberCallKind = ParameterMemberCallKind(params["member_call_kind"].(float64))
		ret.MemberCallObjectIndex = int(params["member_call_index"].(float64))
		ret.MemberCallObjectName = params["member_call_name"].(string)
		if key, ok := params["member_call_key"]; ok {
			ret.MemberCallKey = newLazyInstruction(key)
		}
	case *Phi:
		ret.Edge = unmarshalValues(params["phi_edges"])
		if cfgEntry, ok := params["cfg_entry"]; ok {
			ret.CFGEntryBasicBlock = newLazyInstruction(cfgEntry)
		}
	case *Return:
		ret.Results = unmarshalValues(params["return_results"])
	case *SideEffect:
		ret.CallSite = newLazyInstruction(params["sideEffect_call"])
		ret.Value = newLazyInstruction(params["sideEffect_value"])
	case *UnOp:
		ret.Op = UnaryOpcode(params["unop_op"].(string))
		ret.X = newLazyInstruction(params["unop_x"])
	case *Undefined:
		ret.Kind = UndefinedKind(params["undefined_kind"].(float64))
	case *Jump:
		if to, ok := params["jump_to"]; ok {
			ret.To = newLazyInstruction(to)
		}
	case *ConstInst:
		i := params["const_value"]
		c := newConstByMap(i)
		if c == nil {
			c = newConstCreate(i)
		}
		ret.Const = c
		if origin, ok := params["const_origin"]; ok {
			id := int64(origin.(float64))
			if lz, err := NewInstructionFromLazy(id, ToUser); err == nil {
				ret.Origin = lz
			} else {
				log.Errorf("BUG: unmatched instruction create lazyInstruction: %v", err)
			}
		}
	case *If:
		if cond, ok := params["if_cond"]; ok {
			ret.Cond = newLazyInstruction(cond)
		}
		if trueBlock, ok := params["if_true"]; ok {
			ret.True = newLazyInstruction(trueBlock)
		}
		if falseBlock, ok := params["if_false"]; ok {
			ret.False = newLazyInstruction(falseBlock)
		}
	case *Make:
		if low, ok := params["make_low"]; ok {
			ret.low = newLazyInstruction(low)
		}
		if high, ok := params["make_high"]; ok {
			ret.high = newLazyInstruction(high)
		}
		if step, ok := params["make_step"]; ok {
			ret.step = newLazyInstruction(step)
		}
		if l, ok := params["make_len"]; ok {
			ret.Len = newLazyInstruction(l)
		}
		if c, ok := params["make_cap"]; ok {
			ret.Cap = newLazyInstruction(c)
		}
	case *Function:
		ret.Params = unmarshalValues(params["params"])
		ret.ParamLength = toInt(params["param_length"])
		ret.isMethod = toBool(params["is_method"])
		ret.methodName = toString(params["method_name"])
		ret.FreeValues = unmarshalMapValues(params["free_values"])
		ret.ParameterMembers = unmarshalValues(params["parameter_members"])
		if ses := params["side_effect"]; ses != nil && funk.IsIteratee(ses) {
			var se []*FunctionSideEffect
			funk.ForEach(params["side_effect"], func(a any) {
				ins := &FunctionSideEffect{parameterMemberInner: &parameterMemberInner{}}
				extra := utils.InterfaceToGeneralMap(a)
				// name / verbose_name / modified / forcecreate
				ins.Name = utils.MapGetString(extra, "name")
				ins.VerboseName = utils.MapGetString(extra, "verbose_name")
				ins.Modify = newLazyInstruction(extra["modify"])
				ins.forceCreate = utils.MapGetBool(extra, "forceCreate")
				ins.ObjectName = utils.MapGetString(extra, "object_name")
				ins.MemberCallKind = ParameterMemberCallKind(utils.MapGetInt(extra, "member_call_kind"))
				ins.MemberCallObjectIndex = utils.MapGetInt(extra, "member_call_object_index")
				ins.MemberCallObjectName = utils.MapGetString(extra, "member_call_name")
				if extra["member_call_key"] != nil {
					ins.MemberCallKey = newLazyInstruction(extra["member_call_key"])
				}
				se = append(se, ins)
			})
			ret.SideEffects = se
		}
		if parent, ok := params["parent"]; ok {
			ret.parent = newLazyInstruction(parent)
		}
		ret.ChildFuncs = unmarshalValues(params["child_funcs"])
		ret.Return = unmarshalValues(params["return"])
		ret.Blocks = unmarshalInstructions(params["blocks"])
		if enter, ok := params["enter_block"]; ok {
			ret.EnterBlock = newLazyInstruction(enter)
		}
		if exit, ok := params["exit_block"]; ok {
			ret.ExitBlock = newLazyInstruction(exit)
		}
		if deferBlock, ok := params["defer_block"]; ok {
			ret.DeferBlock = newLazyInstruction(deferBlock)
		}

		if hasEllipsis, ok := params["has_ellipsis"].(bool); ok {
			ret.hasEllipsis = hasEllipsis
		}
	default:
		log.Warnf("unmarshalExtraInformation: unknown type: %v", reflect.TypeOf(inst).String())
	}
}
