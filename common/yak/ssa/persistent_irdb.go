package ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"reflect"
)

const (
	SSAOpcodeAssert       = 1
	SSAOpcodeBasicBlock   = 2
	SSAOpcodeBinOp        = 3
	SSAOpcodeCall         = 4
	SSAOpcodeConstInst    = 5
	SSAOpcodeErrorHandler = 6
	SSAOpcodeExternLib    = 7
	SSAOpcodeIf           = 8
	SSAOpcodeJump         = 9
	SSAOpcodeLoop         = 10
	SSAOpcodeMake         = 11
	SSAOpcodeNext         = 12
	SSAOpcodePanic        = 13
	SSAOpcodeParameter    = 14
	SSAOpcodePhi          = 15
	SSAOpcodeRecover      = 16
	SSAOpcodeReturn       = 17
	SSAOpcodeSideEffect   = 18
	SSAOpcodeSwitch       = 19
	SSAOpcodeTypeCast     = 20
	SSAOpcodeTypeValue    = 21
	SSAOpcodeUnOp         = 22
	SSAOpcodeUndefined    = 23
)

func FitIRCode(c *ssadb.IrCode, r Instruction) error {
	originId := c.ID

	// basic info
	c.Name = r.GetName()
	c.VerboseName = r.GetVerboseName()
	c.ShortVerboseName = r.GetShortVerboseName()

	if ret := r.GetFunc(); ret != nil {
		c.ParentFunction = uint64(ret.GetId())
	}
	if ret := r.GetBlock(); ret != nil {
		c.CurrentBlock = uint64(ret.GetId())
	}

	// handle func
	if f := r.GetFunc(); f != nil {
		c.IsFunction = true
		c.IsVariadic = f.hasEllipsis
		for _, formArg := range f.Param {
			if formArg == nil {
				continue
			}
			c.FormalArgs = append(c.FormalArgs, uint64(formArg.GetId()))
		}
		for _, returnIns := range f.Return {
			if returnIns == nil {
				continue
			}
			c.ReturnCodes = append(c.ReturnCodes, uint64(returnIns.GetId()))
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
			c.CodeBlocks = append(c.CodeBlocks, uint64(b.GetId()))
		}

		c.EnterBlock = uint64(f.EnterBlock.GetId())
		c.ExitBlock = uint64(f.ExitBlock.GetId())
		c.DeferBlock = uint64(f.DeferBlock.GetId())
		for _, subFunc := range f.ChildFuncs {
			c.ChildrenFunction = append(c.ChildrenFunction, uint64(subFunc.GetId()))
		}
	}

	c.IsExternal = r.IsExtern()
	if v, isVal := r.(Value); isVal {
		// ud chain
		for _, user := range v.GetUsers() {
			if _, isCall := user.(*Call); isCall {
				c.IsCalledBy = append(c.IsCalledBy, uint64(user.GetId()))
				if !c.IsCalled {
					c.IsCalled = true
				}
			}
			c.Users = append(c.Users, uint64(user.GetId()))
		}
		for _, def := range v.GetValues() {
			c.Defs = append(c.Defs, uint64(def.GetId()))
		}

		// oop
		if parent := v.GetObject(); parent != nil {
			c.ObjectParent = uint64(parent.GetId())
		}
		if c.ObjectMembers == nil {
			c.ObjectMembers = make(ssadb.Uint64Map)
		}
		for key, val := range v.GetAllMember() {
			c.ObjectMembers[uint64(key.GetId())] = uint64(val.GetId())
		}
		c.IsObject = v.IsObject()
		c.IsObjectMember = v.IsMember()

		// masked
		for _, m := range v.GetMask() {
			c.MaskedCodes = append(c.MaskedCodes, uint64(m.GetId()))
		}
		c.IsMasked = v.Masked()
	}

	switch i := r.(type) {
	case *Assert:
		c.OpcodeName = "assert"
		c.Opcode = SSAOpcodeAssert
	case *BasicBlock:
		c.OpcodeName = "block"
		c.Opcode = SSAOpcodeBasicBlock
		c.IsBlock = true
		for _, pred := range i.Preds {
			c.PredBlock = append(c.PredBlock, uint64(pred.GetId()))
		}
		for _, succ := range i.Succs {
			c.SuccBlock = append(c.SuccBlock, uint64(succ.GetId()))
		}
		for _, p := range i.Phis {
			c.Phis = append(c.Phis, uint64(p.GetId()))
		}
	case *BinOp:
		c.OpcodeName = "binop"
		c.Opcode = SSAOpcodeBinOp
		c.OpcodeOperator = string(i.GetOpcode())
	case *Call:
		c.OpcodeName = "call"
		c.Opcode = SSAOpcodeCall
		for _, arg := range i.Args {
			c.ActualArgs = append(c.ActualArgs, uint64(arg.GetId()))
		}
	case *ConstInst:
		c.OpcodeName = "const"
		c.Opcode = SSAOpcodeConstInst
	case *ErrorHandler:
		c.OpcodeName = "error"
		c.Opcode = SSAOpcodeErrorHandler
	case *ExternLib:
		c.OpcodeName = "extern"
		c.Opcode = SSAOpcodeExternLib
	case *If:
		c.OpcodeName = "if"
		c.Opcode = SSAOpcodeIf
	case *Jump:
		c.OpcodeName = "jump"
		c.Opcode = SSAOpcodeJump
	case *Loop:
		c.OpcodeName = "loop"
		c.Opcode = SSAOpcodeLoop
	case *Make:
		c.OpcodeName = "make"
		c.Opcode = SSAOpcodeMake
	case *Next:
		c.OpcodeName = "next"
		c.Opcode = SSAOpcodeNext
	case *Panic:
		c.OpcodeName = "panic"
		c.Opcode = SSAOpcodePanic
	case *Parameter:
		c.OpcodeName = "param"
		c.Opcode = SSAOpcodeParameter
	case *Phi:
		c.OpcodeName = "phi"
		c.Opcode = SSAOpcodePhi
	case *Recover:
		c.OpcodeName = "recover"
		c.Opcode = SSAOpcodeRecover
	case *Return:
		c.OpcodeName = "return"
		c.Opcode = SSAOpcodeReturn
	case *SideEffect:
		c.OpcodeName = "sideeffect"
		c.Opcode = SSAOpcodeSideEffect
	case *Switch:
		c.OpcodeName = "switch"
		c.Opcode = SSAOpcodeSwitch
	case *TypeCast:
		c.OpcodeName = "typecast"
		c.Opcode = SSAOpcodeTypeCast
	case *TypeValue:
		c.OpcodeName = "typevalue"
		c.Opcode = SSAOpcodeTypeValue
	case *UnOp:
		c.OpcodeName = "unop"
		c.Opcode = SSAOpcodeUnOp
	case *Undefined:
		c.OpcodeName = "undefined"
		c.Opcode = SSAOpcodeUndefined
	default:
		return utils.Errorf("BUG: UNRECOGNIZED INSTRUCTION TYPE: %v", reflect.TypeOf(i).String())
	}
	afterId := c.ID
	if originId != afterId {
		return utils.Error("BUG: Fit IRCode failed, must not change code id")
	}
	return nil
}
