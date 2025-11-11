package ssa

import "github.com/yaklang/yaklang/common/utils"

// for DataFlowNode cover
func ToNode(a any) (Node, bool) { u, ok := a.(Node); return u, ok }

func ToValue(n Instruction) (Value, bool) {
	if utils.IsNil(n) {
		return nil, false
	}
	if IsValueInstruction(n) {
		return n.(Value), true
	}
	return nil, false
}
func ToUser(n Instruction) (User, bool) {
	if utils.IsNil(n) {
		return nil, false
	}
	if IsUserInstruction(n) {
		return n.(User), true
	}
	return nil, false
}

func ToFunction(n Instruction) (*Function, bool) {
	if utils.IsNil(n) {
		return nil, false
	}
	if lz, isLZ := ToLazyInstruction(n); isLZ {
		return ToFunction(lz.Self())
	}
	u, ok := n.(*Function)
	return u, ok
}
func ToBasicBlock(n Instruction) (*BasicBlock, bool) {
	if lz, isLZ := ToLazyInstruction(n); isLZ {
		return ToBasicBlock(lz.Self())
	}
	u, ok := n.(*BasicBlock)
	return u, ok
}

func ToIfInstruction(n Instruction) (*If, bool) {
	if lz, isLZ := ToLazyInstruction(n); isLZ {
		return ToIfInstruction(lz.Self())
	}
	u, ok := n.(*If)
	return u, ok
}

func ToFreeValue(n Node) (*Parameter, bool) {
	u, ok := n.(*Parameter)
	if ok && u.IsFreeValue {
		return u, ok
	}
	return u, ok
}

func ToLazyInstruction(n any) (*LazyInstruction, bool) { u, ok := n.(*LazyInstruction); return u, ok }

// value
func IsConstInst(v Instruction) bool { _, ok := ToConstInst(v); return ok }
func ToConstInst(v Instruction) (*ConstInst, bool) {
	if lz, isLZ := ToLazyInstruction(v); isLZ {
		return ToConstInst(lz.Self())
	}
	c, ok := v.(*ConstInst)
	return c, ok
}

func IsPhi(v Instruction) bool { _, ok := ToPhi(v); return ok }
func ToPhi(v Instruction) (*Phi, bool) {
	if lz, isLZ := ToLazyInstruction(v); isLZ {
		return ToPhi(lz.Self())
	}
	p, ok := v.(*Phi)
	return p, ok
}

func ToExternLib(v Instruction) (*ExternLib, bool) {
	if lz, isLZ := ToLazyInstruction(v); isLZ {
		return ToExternLib(lz.Self())
	}
	p, ok := v.(*ExternLib)
	return p, ok
}

func ToParameter(v Instruction) (*Parameter, bool) {
	if lz, isLZ := ToLazyInstruction(v); isLZ {
		return ToParameter(lz.Self())
	}
	p, ok := v.(*Parameter)
	return p, ok
}

func ToParameterMember(v Instruction) (*ParameterMember, bool) {
	if lz, isLZ := ToLazyInstruction(v); isLZ {
		return ToParameterMember(lz.Self())
	}
	p, ok := v.(*ParameterMember)
	return p, ok
}

func ToReturn(v Instruction) (*Return, bool) {
	if lz, isLZ := ToLazyInstruction(v); isLZ {
		return ToReturn(lz.Self())
	}
	ret, ok := v.(*Return)
	return ret, ok
}

func ToUndefined(v Instruction) (*Undefined, bool) {
	if lz, isLZ := ToLazyInstruction(v); isLZ {
		return ToUndefined(lz.Self())
	}
	p, ok := v.(*Undefined)
	return p, ok
}

func ToBinOp(v Instruction) (*BinOp, bool) {
	if lz, isLZ := ToLazyInstruction(v); isLZ {
		return ToBinOp(lz.Self())
	}
	c, ok := v.(*BinOp)
	return c, ok
}

func ToUnOp(v Instruction) (*UnOp, bool) {
	if lz, isLZ := ToLazyInstruction(v); isLZ {
		return ToUnOp(lz.Self())
	}
	c, ok := v.(*UnOp)
	return c, ok
}

func ToCall(v Instruction) (*Call, bool) {
	if lz, isLZ := ToLazyInstruction(v); isLZ {
		return ToCall(lz.Self())
	}
	p, ok := v.(*Call)
	return p, ok
}
func ToSideEffect(instruction Instruction) (*SideEffect, bool) {
	if lz, isLZ := ToLazyInstruction(instruction); isLZ {
		return ToSideEffect(lz.Self())
	}
	p, ok := instruction.(*SideEffect)
	return p, ok
}

func ToMake(v Instruction) (*Make, bool) {
	if lz, isLZ := ToLazyInstruction(v); isLZ {
		return ToMake(lz.Self())
	}
	p, ok := v.(*Make)
	return p, ok
}

func ToJump(v Instruction) (*Jump, bool) {
	if lz, isLZ := ToLazyInstruction(v); isLZ {
		return ToJump(lz.Self())
	}
	j, ok := v.(*Jump)
	return j, ok
}

func ToTypeValue(v Instruction) (*TypeValue, bool) {
	if lz, isLZ := ToLazyInstruction(v); isLZ {
		return ToTypeValue(lz.Self())
	}
	t, ok := v.(*TypeValue)
	return t, ok
}

// type cover

func ToObjectType(t Type) (*ObjectType, bool)        { o, ok := t.(*ObjectType); return o, ok }
func ToAliasType(t Type) (*AliasType, bool)          { a, ok := t.(*AliasType); return a, ok }
func ToFunctionType(t Type) (*FunctionType, bool)    { f, ok := t.(*FunctionType); return f, ok }
func ToBasicType(t Type) (*BasicType, bool)          { b, ok := t.(*BasicType); return b, ok }
func ToClassBluePrintType(t Type) (*Blueprint, bool) { c, ok := t.(*Blueprint); return c, ok }
func ToOrType(t Type) (*OrType, bool)                { o, ok := t.(*OrType); return o, ok }
