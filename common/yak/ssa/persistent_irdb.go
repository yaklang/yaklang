package ssa

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func FitIRCode(c *ssadb.IrCode, r Instruction) error {
	originId := c.ID

	// basic info
	c.Name = r.GetName()
	c.VerboseName = r.GetVerboseName()
	c.ShortVerboseName = r.GetShortVerboseName()

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

	// source code
	if r := r.GetRange(); r != nil {
		c.SourceCodeStartLine = r.Start.Line
		c.SourceCodeStartCol = r.Start.Column
		c.SourceCodeEndLine = r.End.Line
		c.SourceCodeEndCol = r.End.Column
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

func UpdateIRCode(r Instruction) error {
	db := consts.GetGormProjectDatabase()
	code := ssadb.GetIrCodeById(db, uint(r.GetId()))
	if code == nil {
		log.Warnf("IrCode not found: %d", r.GetId())
		return nil
	}

	FitIRCode(code, r)
	db.Save(code)
	return nil
}
