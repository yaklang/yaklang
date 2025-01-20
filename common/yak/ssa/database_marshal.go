package ssa

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func marshalExtraInformation(raw Instruction) map[string]any {
	params := make(map[string]any)
	switch ret := raw.(type) {
	case *LazyInstruction:
		return marshalExtraInformation(ret.Instruction)
	case *Function:
		params["params"] = ret.Params
		params["param_length"] = ret.ParamLength
		freeValues := make(map[int64]int64)
		for k, v := range ret.FreeValues {
			freeValues[k.GetId()] = v
		}
		params["is_method"] = ret.isMethod
		params["method_name"] = ret.methodName
		params["free_values"] = freeValues
		params["parameter_members"] = ret.ParameterMembers
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
				if se.MemberCallKey > 0 {
					element["member_call_key"] = se.MemberCallKey
				}
			}
			sideEffects = append(sideEffects, element)
		}
		params["side_effect"] = sideEffects
		if p := ret.GetParent(); p != nil {
			params["parent"] = p.GetId()
		}
		params["child_funcs"] = ret.ChildFuncs
		params["return"] = ret.Return
		params["blocks"] = ret.Blocks
		params["enter_block"] = ret.EnterBlock
		params["exit_block"] = ret.ExitBlock
		params["defer_block"] = ret.DeferBlock
		var files [][2]string
		params["reference_files"] = files
		params["has_ellipsis"] = ret.hasEllipsis
	case *Assert:
		params["assert_condition_id"] = ret.Cond
		if ret.MsgValue > 0 {
			params["assert_message_id"] = ret.MsgValue
		}
		params["assert_message_string"] = ret.Msg
	case *BasicBlock:
		params["block_id"] = ret.GetId()
		params["block_name"] = ret.GetName()
		params["block_preds"] = ret.Preds
		params["block_succs"] = ret.Succs
		params["block_can_be_reached"] = ret.canBeReached
		if ret.Condition > 0 {
			if ret.Condition > 0 {
				params["block_condition"] = ret.Condition
			} else {
				log.Warnf("strange things happening when marshal BasicBlock: invalid condition(%T: %v) ", ret.Condition, ret.GetValueById(ret.Condition).String())
			}
		}
		params["block_insts"] = ret.Insts
		params["block_phis"] = ret.Phis
		params["block_finish"] = ret.finish
		if ret.ScopeTable != nil {
			// params["block_scope_table"] = ret.ScopeTable.GetPersistentId()
		}
	case *BinOp:
		params["binop_op"] = ret.Op
		if ret.X > 0 {
			params["binop_x"] = ret.X
		}
		if ret.Y > 0 {
			params["binop_y"] = ret.Y
		}
	case *Call:
		params["call_method"] = ret.Method
		params["call_args"] = ret.Args
		params["call_binding"] = ret.Binding
		params["call_arg_member"] = ret.ArgMember
		params["call_async"] = ret.Async
		params["call_unpack"] = ret.Unpack
		params["call_drop_error"] = ret.IsDropError
		params["call_ellipsis"] = ret.IsEllipsis
	case *ErrorHandler:
		// try-catch-finally-done
		params["errorhandler_try"] = ret.try
		params["errorhandler_catch"] = ret.catchs
		params["errorhandler_finally"] = ret.final
		params["errorhandler_done"] = ret.done
	case *ExternLib:
		log.Warnf("TBD: marshal ExternLib: %v", ret)
		// return nil, utils.Errorf("BUG: ConstInst should not be marshaled")
	case *If:
		if ret.Cond > 0 {
			params["if_cond"] = ret.Cond
		}
		if ret.True > 0 {
			params["if_true"] = ret.True
		}
		if ret.False > 0 {
			params["if_false"] = ret.False
		}
	case *Jump:
		params["jump_to"] = ret.To
	case *Loop:
		params["loop_body"] = ret.Body
		if ret.Exit > 0 {
			params["loop_exit"] = ret.Exit
		}
		if ret.Init > 0 {
			params["loop_init"] = ret.Init
		}
		if ret.Cond > 0 {
			params["loop_cond"] = ret.Cond
		}
		if ret.Step > 0 {
			params["loop_step"] = ret.Step
		}
		if ret.Key > 0 {
			params["loop_key"] = ret.Key
		}
	case *Make:
		if ret.low > 0 {
			params["make_low"] = ret.low
		}
		if ret.high > 0 {
			params["make_high"] = ret.high
		}
		if ret.step > 0 {
			params["make_step"] = ret.step
		}
		if ret.Len > 0 {
			params["make_len"] = ret.Len
		}
		if ret.Cap > 0 {
			params["make_cap"] = ret.Cap
		}
	case *Next:
		if ret.Iter > 0 {
			params["next_iter"] = ret.Iter
		}
		params["next_in_next"] = ret.InNext
	case *Panic:
		if ret.Info > 0 {
			params["panic_value"] = ret.Info
		}
	case *Parameter:
		params["formalParam_is_freevalue"] = ret.IsFreeValue
		if ret.defaultValue > 0 {
			params["formalParam_default"] = ret.defaultValue
		}
		params["formalParam_index"] = ret.FormalParameterIndex
	case *ParameterMember:
		params["formalParamMember_index"] = ret.FormalParameterIndex
		params["member_call_kind"] = ret.MemberCallKind
		params["member_call_index"] = ret.MemberCallObjectIndex
		params["member_call_name"] = ret.MemberCallObjectName
		if ret.MemberCallKey > 0 {
			params["member_call_key"] = ret.MemberCallKey
		}
		// params["member_call_obj"] = ret.GetObject()
	case *Phi:
		params["phi_edges"] = ret.Edge
		if ret.CFGEntryBasicBlock > 0 {
			params["cfg_entry"] = ret.CFGEntryBasicBlock
		}
	case *Return:
		params["return_results"] = ret.Results
	case *SideEffect:
		params["sideEffect_call"] = ret.CallSite
		params["sideEffect_value"] = ret.Value
	case *Switch:
		params["switch_cond"] = ret.Cond
		data, _ := json.Marshal(ret.Label)
		params["switch_label"] = string(data)
	case *TypeCast:
		if ret.Value > 0 {
			params["typecast_value"] = ret.Value
		}
	case *TypeValue:
		// nothing to do
	case *UnOp:
		params["unop_op"] = ret.Op
		if ret.X > 0 {
			params["unop_x"] = ret.X
		}
	case *Undefined:
		params["undefined_kind"] = ret.Kind
	case *ConstInst:
		if ret.Const != nil {
			params["const_value"] = ret.Const.GetRawValue()
		}
		if ret.Origin > 0 {
			params["const_origin"] = ret.Origin
		}
	default:
		log.Warnf("marshalExtraInformation: unknown type: %v", reflect.TypeOf(raw).String())
	}
	return params
}

func unmarshalExtraInformation(inst Instruction, ir *ssadb.IrCode) {
	toInt64 := func(input any) int64 {
		var id int64
		switch result := input.(type) {
		case int64:
			id = result
		case float64:
			id = int64(result)
		default:
			id = codec.Atoi64(fmt.Sprint(input))
		}
		return id
	}
	newLazyInstruction := func(input any) Value {
		id := toInt64(input)
		if id <= 0 {
			return nil
		}

		lz, err := NewLazyEx(id, ToValue)
		if err != nil {
			log.Errorf("BUG: unmatched instruction create lazyInstruction: %v", err)
		}
		return lz
	}

	unmarshalMapVariables := func(p any) map[*Variable]int64 {
		vs := make(map[*Variable]int64)
		switch ret := p.(type) {
		case map[string]any:
			for _, id := range ret {
				value := newLazyInstruction(id)
				vs[value.GetLastVariable()] = toInt64(id)
			}
		default:
		}
		return vs
	}

	params := ir.GetExtraInfo()
	switch ret := inst.(type) {
	case *LazyInstruction:
		unmarshalExtraInformation(ret.Instruction, ir)
	case *Assert:
		ret.Cond = utils.MapGetInt64(params, "assert_condition_id")
		ret.MsgValue = utils.MapGetInt64(params, "assert_message_id")
		ret.Msg = utils.MapGetString(params, "assert_message_string")
	case *BasicBlock:
		ret.Preds = utils.MapGetInt64Slice(params, "block_preds")
		ret.Succs = utils.MapGetInt64Slice(params, "block_succs")
		ret.Condition = utils.MapGetInt64(params, "block_condition")
		ret.canBeReached = BasicBlockReachableKind(utils.MapGetInt(params, "block_can_be_reached"))
		ret.Insts = utils.MapGetInt64Slice(params, "block_insts")
		ret.Phis = utils.MapGetInt64Slice(params, "block_phis")
		ret.finish = utils.MapGetBool(params, "block_finish")
	case *BinOp:
		ret.Op = BinaryOpcode(utils.MapGetString(params, "binop_op"))
		ret.X = utils.MapGetInt64(params, "binop_x")
		ret.Y = utils.MapGetInt64(params, "binop_y")
	case *Call:
		ret.Method = utils.MapGetInt64(params, "call_method")
		ret.Args = utils.MapGetInt64Slice(params, "call_args")
		ret.ArgMember = utils.MapGetInt64Slice(params, "call_arg_member")
		ret.Binding = utils.MapGetStringInt64Map(params, "call_binding")
		ret.Async = utils.MapGetBool(params, "call_async")
		ret.Unpack = utils.MapGetBool(params, "call_unpack")
		ret.IsDropError = utils.MapGetBool(params, "call_drop_error")
		ret.IsEllipsis = utils.MapGetBool(params, "call_ellipsis")
	case *Next:
		ret.InNext = utils.MapGetBool(params, "next_item")
		ret.Iter = utils.MapGetInt64(params, "next_iter")
	case *Parameter:
		ret.IsFreeValue = utils.MapGetBool(params, "formalParam_is_freevalue")
		ret.defaultValue = utils.MapGetInt64(params, "formalParam_default")
		ret.FormalParameterIndex = utils.MapGetInt(params, "formalParam_index")
	case *ParameterMember:
		ret.FormalParameterIndex = utils.MapGetInt(params, "formalParamMember_index")
		ret.MemberCallKind = ParameterMemberCallKind(utils.MapGetInt(params, "member_call_kind"))
		ret.MemberCallObjectIndex = utils.MapGetInt(params, "member_call_index")
		ret.MemberCallObjectName = utils.MapGetString(params, "member_call_name")
		ret.MemberCallKey = utils.MapGetInt64(params, "member_call_key")
	case *Phi:
		ret.Edge = utils.MapGetInt64Slice(params, "phi_edges")
		ret.CFGEntryBasicBlock = utils.MapGetInt64(params, "cfg_entry")
	case *Return:
		ret.Results = utils.MapGetInt64Slice(params, "return_results")
	case *SideEffect:
		ret.CallSite = utils.MapGetInt64(params, "sideEffect_call")
		ret.Value = utils.MapGetInt64(params, "sideEffect_value")
	case *Switch:
		ret.Cond = utils.MapGetInt64(params, "switch_cond")
		json.Unmarshal([]byte(utils.MapGetString(params, "switch_label")), &ret.Label)
	case *UnOp:
		ret.Op = UnaryOpcode(utils.MapGetString(params, "unop_op"))
		ret.X = utils.MapGetInt64(params, "unop_x")
	case *Undefined:
		ret.Kind = UndefinedKind(utils.MapGetInt(params, "undefined_kind"))
	case *Jump:
		ret.To = utils.MapGetInt64(params, "jump_to")
	case *ConstInst:
		if v, ok := params["const_value"]; ok {
			SetConst(v, ret)
		}
		ret.Origin = utils.MapGetInt64(params, "const_origin")
	case *If:
		ret.Cond = utils.MapGetInt64(params, "if_cond")
		ret.True = utils.MapGetInt64(params, "if_true")
		ret.False = utils.MapGetInt64(params, "if_false")
	case *Make:
		ret.low = utils.MapGetInt64(params, "make_low")
		ret.high = utils.MapGetInt64(params, "make_high")
		ret.step = utils.MapGetInt64(params, "make_step")
		ret.Len = utils.MapGetInt64(params, "make_len")
		ret.Cap = utils.MapGetInt64(params, "make_cap")
	case *Function:
		ret.Params = utils.MapGetInt64Slice(params, "params")
		ret.ParamLength = utils.MapGetInt(params, "param_length")
		ret.isMethod = utils.MapGetBool(params, "is_method")
		ret.methodName = utils.MapGetString(params, "method_name")
		ret.FreeValues = unmarshalMapVariables(params["free_values"])
		ret.ParameterMembers = utils.MapGetInt64Slice(params, "parameter_members")

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
					ins.MemberCallKey = utils.MapGetInt64(extra, "member_call_key")
				}
				se = append(se, ins)
			})
			ret.SideEffects = se
		}
		ret.parent = utils.MapGetInt64(params, "parent")
		ret.ChildFuncs = utils.MapGetInt64Slice(params, "child_funcs")
		ret.Return = utils.MapGetInt64Slice(params, "return")
		ret.Blocks = utils.MapGetInt64Slice(params, "blocks")
		ret.EnterBlock = utils.MapGetInt64(params, "enter_block")
		ret.ExitBlock = utils.MapGetInt64(params, "exit_block")
		ret.DeferBlock = utils.MapGetInt64(params, "defer_block")
		ret.hasEllipsis = utils.MapGetBool(params, "has_ellipsis")
	case *ExternLib:

	default:
		log.Warnf("unmarshalExtraInformation: unknown type: %v", reflect.TypeOf(inst).String())
	}
}
