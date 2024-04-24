package ssa

import (
	"reflect"

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
	case []Value:
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
	default:
		log.Warnf("fetchIds: unknown type: %v", reflect.TypeOf(origin).String())
	}
	return ids
}

func marshalExtraInformation(raw Instruction) map[string]any {
	marshalValues := func(vs []Value) []int64 {
		ids := make([]int64, len(vs))
		for index, v := range vs {
			ids[index] = v.GetId()
		}
		return ids
	}

	params := make(map[string]any)
	switch ret := raw.(type) {
	case *Assert:
		params["assert_condition_id"] = ret.Cond.GetId()
		params["assert_message_id"] = ret.MsgValue.GetId()
		params["assert_message_string"] = ret.MsgValue.String()
	case *BasicBlock:
		params["block_id"] = ret.GetId()
		params["block_name"] = ret.GetName()
		params["block_preds"] = fetchIds(ret.Preds)
		params["block_succs"] = fetchIds(ret.Succs)
		params["block_condition"] = ret.Condition.GetId()
		params["block_insts"] = fetchIds(ret.Insts)
		params["block_phis"] = fetchIds(ret.Phis)
	case *BinOp:
		params["binop_op"] = ret.Op
		params["binop_x"] = ret.X.GetId()
		params["binop_y"] = ret.Y.GetId()
	case *Call:
		params["call_method"] = ret.GetFunc().GetId()
		params["call_args"] = marshalValues(ret.Args)
		params["call_binding"] = fetchIds(ret.Binding)
		params["call_async"] = ret.Async
		params["call_unpack"] = ret.Unpack
		params["call_drop_error"] = ret.IsDropError
		params["call_ellipsis"] = ret.IsEllipsis
	case *ErrorHandler:
		// try-catch-finally-done
		params["errorhandler_try"] = ret.try.GetId()
		params["errorhandler_catch"] = ret.catch.GetId()
		params["errorhandler_finally"] = ret.final.GetId()
		params["errorhandler_done"] = ret.done.GetId()
	case *ExternLib:
		// return nil, utils.Errorf("BUG: ConstInst should not be marshaled")
	case *If:
		params["if_cond"] = ret.Cond.GetId()
		params["if_true"] = ret.True.GetId()
		params["if_false"] = ret.False.GetId()
	case *Jump:
		params["jump_to"] = ret.To.GetId()
	case *Loop:
		params["loop_body"] = ret.Body.GetId()
		params["loop_exit"] = ret.Exit.GetId()
		params["loop_init"] = ret.Init.GetId()
		params["loop_cond"] = ret.Cond.GetId()
		params["loop_step"] = ret.Step.GetId()
		params["loop_key"] = ret.Key.GetId()
	case *Make:
		params["make_low"] = ret.low.GetId()
		params["make_high"] = ret.high.GetId()
		params["make_step"] = ret.step.GetId()
		params["make_len"] = ret.Len.GetId()
		params["make_cap"] = ret.Cap.GetId()
	case *Next:
		params["next_iter"] = ret.Iter.GetId()
		params["next_in_next"] = ret.InNext
	case *Panic:
		params["panic_value"] = ret.Info.GetId()
	case *Parameter:
		params["formalParam_is_freevalue"] = ret.IsFreeValue
		params["formalParam_default"] = ret.defaultValue.GetId()
		params["formalParam_index"] = ret.FormalParameterIndex
	case *Phi:
		params["phi_edges"] = marshalValues(ret.Edge)
		params["phi_create"] = ret.create
		params["phi_whi1"] = ret.wit1.GetId()
		params["phi_whi2"] = ret.wit2.GetId()
	case *Recover:
		// nothing to do
	case *Return:
		params["return_results"] = marshalValues(ret.Results)
	case *SideEffect:
		params["sideEffect_call"] = ret.CallSite.GetId()
		params["sideEffect_value"] = ret.GetId()
	case *Switch:
		params["switch_cond"] = ret.Cond.GetId()
		params["switch_label"] = fetchIds(ret.Label)
	case *TypeCast:
		params["typecast_value"] = ret.Value.GetId()
	case *TypeValue:
		// nothing to do
	case *UnOp:
		params["unop_op"] = ret.Op
		params["unop_x"] = ret.X.GetId()
	case *Undefined:
		params["undefined_kind"] = ret.Kind
	case *Function:
		// fill it later
	case *ConstInst:
		params["const_value"] = ret.Const.GetRawValue()
	}
	return params
}

func unmarshalExtraInformation(inst Instruction, ir *ssadb.IrCode) {
	params := ir.GetExtraInfo()
	switch ret := inst.(type) {
	case *ConstInst:
		i := params["const_value"]
		c := newConstByMap(i)
		if c == nil {
			c = newConstCreate(i)
		}
		ret.Const = c

	}

}
