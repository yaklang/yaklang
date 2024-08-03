package ssa

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

func fixupUseChain(node Instruction) {
	if u, ok := ToUser(node); ok {
		for _, v := range u.GetValues() {
			if v == nil {
				log.Warnf("BUG: value[%s: %s] def is nil", u, u.GetRange())
				continue
			}
			v.AddUser(u)
		}
	}
}

func DeleteInst(i Instruction) {
	_, ok := i.(*anInstruction)
	if ok {
		i = i.GetProgram().GetInstructionById(i.GetId())
	}

	b := i.GetBlock()
	if b == nil {
		log.Infof("void block!! %s:%s", i, i.GetRange())
		return
	}
	if phi, ok := ToPhi(i); ok {
		b.Phis = lo.Filter(b.Phis, func(item Value, index int) bool {
			return item.GetId() != phi.GetId()
		})
	} else {
		//b.Insts = utils.RemoveSliceItem(b.Insts, Instruction(i))
		b.Insts = lo.Filter(b.Insts, func(item Instruction, index int) bool {
			return item.GetId() != i.GetId()
		})
	}
	if user, ok := ToUser(i); ok {
		for _, value := range user.GetValues() {
			value.RemoveUser(user)
		}
	}
	i.GetProgram().DeleteInstruction(i)
}

// func EmitInst(i Instruction) {
// 	block := i.GetBlock()
// 	if block == nil {
// 		// println("void block!! %s")
// 		return
// 	}
// 	if index := slices.Index(block.Insts, i); index != -1 {
// 		return
// 	}
// 	if len(block.Insts) == 0 {
// 		b := block.Parent.builder
// 		current := b.CurrentBlock
// 		b.CurrentBlock = block
// 		b.emit(i)
// 		b.CurrentBlock = current
// 	} else {
// 		EmitBefore(block.LastInst(), i)
// 	}
// }

// func EmitBefore(before, inst Instruction) {
// 	block := before.GetBlock()
// 	insts := block.Insts
// 	if index := slices.Index(insts, before); index > -1 {
// 		// Extend the slice
// 		insts = append(insts, nil)
// 		// Move elements to create a new space
// 		copy(insts[index+1:], insts[index:])
// 		// Insert new element
// 		insts[index] = inst
// 		block.Insts = insts
// 		block.Parent.SetReg(inst)
// 	}
// }

//	func EmitAfter(after, inst Instruction) {
//		block := after.GetBlock()
//		insts := block.Insts
//		if index := slices.Index(insts, after); index > -1 {
//			// Extend the slice
//			insts = append(insts, nil)
//			// Move elements to create a new space
//			copy(insts[index+2:], insts[index+1:])
//			// Insert new element
//			insts[index+1] = inst
//			block.Insts = insts
//			block.Parent.SetReg(inst)
//		}
//	}

func (f *FunctionBuilder) SetCurrent(i Instruction, noChangeRanges ...bool) func() {
	if i.IsExtern() {
		return func() {}
	}
	noChangeRange := false
	if len(noChangeRanges) > 0 {
		noChangeRange = noChangeRanges[0]
	}

	currentBlock := f.CurrentBlock
	Range := f.CurrentRange
	fun := f.Function
	builder := i.GetFunc().builder
	parentScope := f.parentScope

	f.CurrentBlock = i.GetBlock()
	f.Function = i.GetFunc()
	f.parentScope = builder.parentScope
	if !noChangeRange {
		f.CurrentRange = i.GetRange()
	}

	return func() {
		f.CurrentBlock = currentBlock
		f.Function = fun
		f.parentScope = parentScope
		if !noChangeRange {
			f.CurrentRange = Range
		}
	}
}

func (b *BasicBlock) EmitInst(i Instruction) {
	if index := slices.Index(b.Insts, i); index == -1 {
		b.GetFunc().builder.EmitToBlock(i, b)
	}
}

func (f *FunctionBuilder) EmitToBlock(i Instruction, block *BasicBlock) {
	if len(block.Insts) == 0 {
		f.emit(i)
	} else {
		f.EmitInstructionBefore(i, block.LastInst())
	}
}

func (b *FunctionBuilder) EmitFirst(i Instruction, block *BasicBlock) {
	if len(block.Insts) == 0 {
		b.emit(i)
	} else {
		b.EmitInstructionBefore(i, block.Insts[0])
	}
}

func (f *FunctionBuilder) EmitInstructionBefore(i, before Instruction) {
	f.emitAroundInstruction(i, before, func(i Instruction) {
		insts := f.CurrentBlock.Insts
		if index := slices.Index(insts, before); index > -1 {
			// Extend the slice
			insts = append(insts, nil)
			// Move elements to create a new space
			copy(insts[index+1:], insts[index:])
			// Insert new element
			insts[index] = i
			f.CurrentBlock.Insts = insts
		}
	})
}

func (f *FunctionBuilder) EmitInstructionAfter(i, after Instruction) {
	f.emitAroundInstruction(i, after, func(i Instruction) {
		insts := f.CurrentBlock.Insts
		if index := slices.Index(insts, after); index > -1 {
			// Extend the slice
			insts = append(insts, nil)
			// Move elements to create a new space
			copy(insts[index+2:], insts[index+1:])
			// Insert new element
			insts[index+1] = i
			// block.Insts = insts
			f.CurrentBlock.Insts = insts
		}
	})
}

func (f *FunctionBuilder) emitAroundInstruction(i, other Instruction, insert func(Instruction)) {
	recoverBuilder := f.SetCurrent(other, true)
	defer recoverBuilder()

	f.emitEx(i, insert)
}

func (f *FunctionBuilder) emit(i Instruction) {
	if f.CurrentBlock.finish || utils.IsNil(i) {
		log.Errorf("this block [%s] is finish, instruction[%s] can't insert!", f.CurrentBlock, i)
	}
	f.emitEx(i, f.EmitOnly)
}

func (f *FunctionBuilder) SetInstructionPosition(i Instruction) {
	f.emitEx(i, func(i Instruction) {})
}

func (f *FunctionBuilder) EmitOnly(i Instruction) {
	f.CurrentBlock.Insts = append(f.CurrentBlock.Insts, i)
}

func (f *FunctionBuilder) emitEx(i Instruction, insert func(Instruction)) {
	fixupUseChain(i)
	// i.SetScope(f.CurrentScope)
	i.SetRange(f.CurrentRange)
	i.SetBlock(f.CurrentBlock)
	i.SetFunc(f.Function)
	f.GetProgram().SetVirtualRegister(i)
	insert(i)
}

// EmitUndefined emit undefined value
// the current block is finished.
// NOTE: the object/membercall will create vars in finished blocks
func (f *FunctionBuilder) EmitUndefined(name string) *Undefined {
	u := NewUndefined(name)
	f.EmitFirst(u, f.CurrentBlock)
	return u
}

func (f *FunctionBuilder) EmitUnOp(op UnaryOpcode, v Value) Value {
	if f.CurrentBlock.finish {
		return nil
	}
	u := NewUnOp(op, v)
	f.emit(u)
	if c, ok := ToConst(HandlerUnOp(u)); ok {
		f.emit(c)
		return c
	}
	return u
}

func (f *FunctionBuilder) EmitBinOp(op BinaryOpcode, x, y Value) Value {
	if f.CurrentBlock.finish {
		return nil
	}
	binOp := NewBinOp(op, x, y)
	f.emit(binOp)
	if c, ok := ToConst(HandlerBinOp(binOp)); ok {
		f.emit(c)
		return c
	}
	return binOp
}

func (f *FunctionBuilder) EmitIf() *If {
	if f.CurrentBlock.finish {
		return nil
	}
	ifSSA := NewIf()
	f.emit(ifSSA)
	f.CurrentBlock.finish = true
	return ifSSA
}

func (f *FunctionBuilder) EmitJump(to *BasicBlock) *Jump {
	if f.CurrentBlock.finish {
		return nil
	}
	j := NewJump(to)
	f.emit(j)
	f.CurrentBlock.AddSucc(to)
	f.CurrentBlock.finish = true
	return j
}

func (f *FunctionBuilder) EmitLoop(body, exit *BasicBlock, cond Value) *Loop {
	if f.CurrentBlock.finish {
		return nil
	}
	l := NewLoop(cond)
	l.Body = body
	l.Exit = exit
	f.CurrentBlock.AddSucc(body)
	f.CurrentBlock.AddSucc(exit)
	f.emit(l)
	f.CurrentBlock.finish = true
	return l
}

func (f *FunctionBuilder) EmitSwitch(cond Value, defaultb *BasicBlock, label []SwitchLabel) *Switch {
	if f.CurrentBlock.finish {
		return nil
	}
	sw := NewSwitch(cond, defaultb, label)
	f.emit(sw)
	f.CurrentBlock.finish = true
	return sw
}

func (f *FunctionBuilder) EmitSideEffect(name string, call *Call, value Value) *SideEffect {
	if f.CurrentBlock.finish {
		return nil
	}
	s := NewSideEffect(name, call, value)
	f.emit(s)
	return s
}

func (f *FunctionBuilder) EmitReturn(vs []Value) *Return {
	if f.CurrentBlock.finish {
		return nil
	}
	r := NewReturn(vs)
	f.emit(r)
	f.Return = append(f.Return, r)
	return r
}

func (f *FunctionBuilder) EmitAssert(cond, msgValue Value, msg string) *Assert {
	if f.CurrentBlock.finish {
		return nil
	}
	a := NewAssert(cond, msgValue, msg)
	f.emit(a)
	return a
}

func (f *FunctionBuilder) emitMake(parentI Value, typ Type, low, high, max, Len, Cap Value) *Make {
	if f.CurrentBlock.finish {
		log.Errorf("BUG: current block is finish, can't emit make")
		log.Errorf("BUG: current block is finish, can't emit make")
		log.Errorf("BUG: current block is finish, can't emit make")
		log.Errorf("BUG: current block is finish, can't emit make")
		return nil
	}
	i := NewMake(parentI, typ, low, high, max, Len, Cap)
	f.emit(i)
	return i
}

func (f *FunctionBuilder) EmitMakeBuildWithType(typ Type, Len, Cap Value) *Make {
	i := f.emitMake(nil, typ, nil, nil, nil, Len, Cap)
	return i
}

func (f *FunctionBuilder) EmitMakeWithoutType(Len, Cap Value) *Make {
	return f.emitMake(nil, BasicTypes[AnyTypeKind], nil, nil, nil, Len, Cap)
}

func (f *FunctionBuilder) EmitEmptyContainer() *Make {
	return f.EmitMakeWithoutType(nil, nil)
}

func (f *FunctionBuilder) EmitMakeSlice(i Value, low, high, max Value) *Make {
	return f.emitMake(i, i.GetType(), low, high, max, nil, nil)
}

func (f *FunctionBuilder) EmitValueOnlyDeclare(name string) *Undefined {
	un := f.EmitUndefined(name)
	un.Kind = UndefinedValueValid
	return un
}

func (f *FunctionBuilder) EmitConstInstNil() *ConstInst {
	return f.EmitConstInst(nil)
}

func (f *FunctionBuilder) EmitConstInstWithUnary(i any, un int) *ConstInst {
	ci := f.EmitConstInst(i)
	ci.Unary = un
	f.emit(ci)
	return ci
}

func (f *FunctionBuilder) EmitConstInst(i any) *ConstInst {
	// if f.CurrentBlock.finish {
	// 	return nil
	// }
	ci := NewConst(i)
	f.emit(ci)
	return ci
}

func (f *FunctionBuilder) EmitTypeCast(v Value, typ Type) *TypeCast {
	if f.CurrentBlock.finish {
		return nil
	}
	t := NewTypeCast(typ, v)
	f.emit(t)
	return t
}

func (f *FunctionBuilder) EmitTypeValue(typ Type) *TypeValue {
	if f.CurrentBlock.finish {
		return nil
	}
	t := NewTypeValue(typ)
	f.emit(t)
	t.SetType(typ)
	return t
}

func (f *FunctionBuilder) EmitNextOnly(iter Value, isIn bool) *Next {
	if f.CurrentBlock.finish {
		return nil
	}
	n := NewNext(iter, isIn)
	f.emit(n)
	return n
}

func (f *FunctionBuilder) EmitNext(iter Value, isIn bool) (key, field, ok Value) {
	if f.CurrentBlock.finish {
		return nil, nil, nil
	}
	n := f.EmitNextOnly(iter, isIn)
	// n iter-type: map[T]U   n-type {key: T, field: U, ok: bool}
	key = f.ReadMemberCallVariable(n, f.EmitConstInst(NextKey.String()))
	field = f.ReadMemberCallVariable(n, f.EmitConstInst(NextField.String()))
	ok = f.ReadMemberCallVariable(n, f.EmitConstInst(NextOk.String()))
	return
}

func (f *FunctionBuilder) EmitErrorHandler(try *BasicBlock) *ErrorHandler {
	if f.CurrentBlock.finish {
		return nil
	}
	e := NewErrorHandler(try)
	block := f.CurrentBlock
	block.AddSucc(try)
	f.emit(e)
	return e
}

func (f *FunctionBuilder) EmitPanic(info Value) *Panic {
	if f.CurrentBlock.finish {
		return nil
	}
	p := &Panic{
		anValue: NewValue(),
		Info:    info,
	}
	f.emit(p)
	return p
}

func (f *FunctionBuilder) EmitRecover() *Recover {
	if f.CurrentBlock.finish {
		return nil
	}
	r := &Recover{
		anValue: NewValue(),
	}
	r.SetType(BasicTypes[AnyTypeKind])
	f.emit(r)
	return r
}

func (f *FunctionBuilder) EmitPhi(name string, vs []Value) *Phi {
	p := &Phi{
		anValue: NewValue(),
		Edge:    vs,
	}
	p.SetName(name)
	f.emitEx(p, func(i Instruction) {
		f.CurrentBlock.Phis = append(f.CurrentBlock.Phis, p)
	})
	return p
}
