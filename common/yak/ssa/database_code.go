package ssa

import (
	"encoding/json"
	"github.com/davecgh/go-spew/spew"
	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"reflect"
)

func fitRange(c *ssadb.IrCode, rangeIns *Range) {
	if rangeIns == nil {
		log.Warnf("(BUG or in DEBUG MODE) Range not found for %s", c.Name)
		return
	}
	c.SourceCodeHash = rangeIns.GetEditor().SourceCodeMd5()
	start, end := rangeIns.GetOffsetRange()
	c.SourceCodeStartOffset = int64(start)
	c.SourceCodeEndOffset = int64(end)
}

type cacheRecoveryContext struct {
	cache *omap.OrderedMap[string, *ssadb.IrCode]
	db    *gorm.DB
}

func (c *cacheRecoveryContext) LazyInstruction(id int64) *LazyInstruction {
	return NewLazyInstruction(c.db, id)
}

func newCacheRecoverContext() *cacheRecoveryContext {
	return &cacheRecoveryContext{
		cache: omap.NewOrderedMap(make(map[string]*ssadb.IrCode)),
		db:    consts.GetGormProjectDatabase(),
	}
}

func irCodeToInstruction(ctx *cacheRecoveryContext, c *ssadb.IrCode) (Instruction, error) {
	if ctx == nil {
		ctx = newCacheRecoverContext()
	}
	switch c.Opcode {
	case int64(SSAOpcodeUnKnow):
		return nil, utils.Errorf("BUG here: ir.OpCode cannot be the unknown opcode: %v", c.VerboseString())
	case int64(SSAOpcodeAssert):
		// assert should set cond as member
		return nil, nil
	case int64(SSAOpcodeBasicBlock):
		return nil, utils.Errorf("unhandled opcode: %v", c.VerboseString())
	case int64(SSAOpcodeBinOp):
		return nil, nil
	case int64(SSAOpcodeCall):
		return nil, nil
	case int64(SSAOpcodeConstInst):
		return nil, nil
	case int64(SSAOpcodeErrorHandler):
		return nil, nil
	case int64(SSAOpcodeExternLib):
		return nil, nil
	case int64(SSAOpcodeIf):
		return nil, nil
	case int64(SSAOpcodeJump):
		return nil, nil
	case int64(SSAOpcodeLoop):
		return nil, nil
	case int64(SSAOpcodeMake):
		return nil, nil
	case int64(SSAOpcodeNext):
		return nil, nil
	case int64(SSAOpcodePanic):
		return nil, nil
	case int64(SSAOpcodeParameter):
		return nil, nil
	case int64(SSAOpcodeFreeValue):
		return nil, nil
	case int64(SSAOpcodeParameterMember):
		return nil, nil
	case int64(SSAOpcodePhi):
		return nil, nil
	case int64(SSAOpcodeRecover):
		return nil, nil
	case int64(SSAOpcodeReturn):
		return nil, nil
	case int64(SSAOpcodeSideEffect):
		return nil, nil
	case int64(SSAOpcodeSwitch):
		return nil, nil
	case int64(SSAOpcodeTypeCast):
		return nil, nil
	case int64(SSAOpcodeTypeValue):
		return nil, nil
	case int64(SSAOpcodeUnOp):
		return nil, nil
	case int64(SSAOpcodeUndefined):
		return nil, nil
	case int64(SSAOpcodeFunction):
		return nil, nil
	default:
		return nil, utils.Errorf("unknown opcode: %v", c.VerboseString())
	}
}

func FitIRCode(c *ssadb.IrCode, r Instruction) error {
	originId := c.ID

	// basic info
	c.Name = r.GetName()
	c.VerboseName = r.GetVerboseName()
	c.ShortVerboseName = r.GetShortVerboseName()

	extraInfo, err := marshalInformation(r)
	if err != nil {
		log.Warnf("BUG: cannot fetch instruction's extra info: %v", err)
	}
	c.ExtraInformation = string(extraInfo)

	if rangeIns := r.GetRange(); rangeIns != nil {
		// set range from code
		fitRange(c, rangeIns)
	} else if f := r.GetFunc(); f != nil {
		fitRange(c, f.GetRange())
	} else {
		log.Warnf("Range not found for %s", c.Name)
	}

	if ret := r.GetFunc(); ret != nil {
		c.CurrentFunction = int64(ret.GetId())
	}
	if ret := r.GetBlock(); ret != nil {
		c.CurrentBlock = int64(ret.GetId())
	}

	// handle func
	if f, ok := r.(*Function); ok {
		c.IsFunction = true
		c.IsVariadic = f.hasEllipsis
		for _, formArg := range f.Param {
			if formArg == nil {
				continue
			}
			c.FormalArgs = append(c.FormalArgs, int64(formArg.GetId()))
		}
		for _, returnIns := range f.Return {
			if returnIns == nil {
				continue
			}
			c.ReturnCodes = append(c.ReturnCodes, int64(returnIns.GetId()))
		}
		for _, sideEffect := range f.SideEffects {
			if sideEffect == nil {
				continue
			}
			log.Warnf("SideEffect is not supported yet: %v", sideEffect.Name)
		}

		for _, b := range f.Blocks {
			if b == nil {
				continue
			}
			c.CodeBlocks = append(c.CodeBlocks, int64(b.GetId()))
		}

		if f.EnterBlock != nil {
			c.EnterBlock = int64(f.EnterBlock.GetId())
		}
		if f.ExitBlock != nil {
			c.ExitBlock = int64(f.ExitBlock.GetId())
		}
		if f.DeferBlock != nil {
			c.DeferBlock = int64(f.DeferBlock.GetId())
		}

		for _, subFunc := range f.ChildFuncs {
			c.ChildrenFunction = append(c.ChildrenFunction, int64(subFunc.GetId()))
		}
	}

	c.IsExternal = r.IsExtern()
	if v, isVal := r.(Value); isVal {
		// ud chain
		for _, user := range v.GetUsers() {
			if _, isCall := user.(*Call); isCall {
				c.IsCalledBy = append(c.IsCalledBy, int64(user.GetId()))
				if !c.IsCalled {
					c.IsCalled = true
				}
			}
			c.Users = append(c.Users, int64(user.GetId()))
		}
		for _, def := range v.GetValues() {
			c.Defs = append(c.Defs, int64(def.GetId()))
		}

		// oop
		if parent := v.GetObject(); parent != nil {
			c.ObjectParent = int64(parent.GetId())
		}
		if c.ObjectMembers == nil {
			c.ObjectMembers = make(ssadb.Int64Map)
		}
		for key, val := range v.GetAllMember() {
			c.ObjectMembers[int64(key.GetId())] = int64(val.GetId())
		}
		c.IsObject = v.IsObject()
		c.IsObjectMember = v.IsMember()

		// masked
		for _, m := range v.GetMask() {
			c.MaskedCodes = append(c.MaskedCodes, int64(m.GetId()))
		}
		c.IsMasked = v.Masked()
	}

	// variable
	c.Variable = lo.Keys(r.GetAllVariables())

	c.Opcode = int64(r.GetOpcode())
	c.OpcodeName = SSAOpcode2Name[r.GetOpcode()]

	switch i := r.(type) {
	case *ConstInst:
		c.ConstantValue = i.str
		if bin, ok := i.Origin.(*BinOp); ok {
			c.OpcodeOperator = BinaryOpcodeName[(*bin).Op]
		} else if un, ok := i.Origin.(*UnOp); ok {
			c.OpcodeOperator = UnaryOpcodeName[(*un).Op]
		}
	case *BasicBlock:
		c.IsBlock = true
		for _, pred := range i.Preds {
			c.PredBlock = append(c.PredBlock, int64(pred.GetId()))
		}
		for _, succ := range i.Succs {
			c.SuccBlock = append(c.SuccBlock, int64(succ.GetId()))
		}
		for _, p := range i.Phis {
			c.Phis = append(c.Phis, int64(p.GetId()))
		}
	case *BinOp:
		c.OpcodeOperator = BinaryOpcodeName[i.Op]
	case *UnOp:
		c.OpcodeOperator = UnaryOpcodeName[i.Op]
	case *Call:
		for _, arg := range i.Args {
			c.ActualArgs = append(c.ActualArgs, int64(arg.GetId()))
		}
		// default:
		// 	return utils.Errorf("BUG: UNRECOGNIZED INSTRUCTION TYPE: %v", reflect.TypeOf(i).String())
	}
	afterId := c.ID
	if originId != afterId {
		return utils.Error("BUG: Fit IRCode failed, must not change code id")
	}
	return nil
}

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
	default:
		log.Warnf("fetchIds: unknown type: %v", reflect.TypeOf(origin).String())
	}
	return ids
}

func marshalInformation(raw Instruction) ([]byte, error) {
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
		params["call_args"] = fetchIds(ret.Args)
		params["call_binding"] = fetchIds(ret.Binding)
		params["call_async"] = ret.Async
		params["call_unpack"] = ret.Unpack
		params["call_drop_error"] = ret.IsDropError
		params["call_ellipsis"] = ret.IsEllipsis
	case *ConstInst:
		return nil, utils.Errorf("BUG: ConstInst should not be marshaled")
	case *ErrorHandler:
		// try-catch-finally-done
		params["errorhandler_try"] = ret.try.GetId()
		params["errorhandler_catch"] = ret.catch.GetId()
		params["errorhandler_finally"] = ret.final.GetId()
		params["errorhandler_done"] = ret.done.GetId()
	case *ExternLib:
		return nil, utils.Errorf("BUG: ConstInst should not be marshaled")
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
		params["phi_edges"] = fetchIds(ret.Edge)
		params["phi_create"] = ret.create
		params["phi_whi1"] = ret.wit1.GetId()
		params["phi_whi2"] = ret.wit2.GetId()
	case *Recover:
		// nothing to do
	case *Return:
		params["return_results"] = fetchIds(ret.Results)
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
	}
	results, err := json.Marshal(params)
	if err != nil {
		spew.Dump(params)
		return nil, utils.Errorf("instruction json.Marshal failed: %v", err)
	}
	return results, nil
}
