package ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

func fixupUseChain(node Node) {
	if u, ok := ToUser(node); ok {
		for _, v := range GetValues(u) {
			v.AddUser(u)
		}
	}
}
func DeleteInst(i Instruction) {
	b := i.GetBlock()
	if phi, ok := ToPhi(i); ok {
		b.Phis = utils.RemoveSliceItem(b.Phis, phi)
	} else {
		b.Insts = utils.RemoveSliceItem(b.Insts, Instruction(i))
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

func (f *FunctionBuilder) SetCurrent(i Instruction) func() {
	currentBlock := f.CurrentBlock
	Range := f.CurrentRange
	// scope := f.CurrentScope

	// f.CurrentScope = i.GetScope()
	f.CurrentRange = i.GetRange()
	f.CurrentBlock = i.GetBlock()

	return func() {
		f.CurrentBlock = currentBlock
		f.CurrentRange = Range
		// f.CurrentScope = scope
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
	recoverBuilder := f.SetCurrent(other)
	defer recoverBuilder()

	f.emitEx(i, insert)
}

func (f *FunctionBuilder) emit(i Instruction) {
	if f.CurrentBlock.finish || utils.IsNil(i) {
		log.Errorf("this block [%s] is finish, instruction[%s] can't insert!", f.CurrentBlock, i)
	}
	f.emitEx(i, func(i Instruction) {
		f.CurrentBlock.Insts = append(f.CurrentBlock.Insts, i)
	})
}

func (f *FunctionBuilder) SetInstructionPosition(i Instruction) {
	f.emitEx(i, func(i Instruction) {})
}

func (f *FunctionBuilder) EmitOnly(i Instruction) {
	f.CurrentBlock.Insts = append(f.CurrentBlock.Insts, i)
}

func (f *FunctionBuilder) emitEx(i Instruction, insert func(Instruction)) {
	if n, ok := ToNode(i); ok {
		fixupUseChain(n)
	}
	// i.SetScope(f.CurrentScope)
	i.SetRange(f.CurrentRange)
	i.SetBlock(f.CurrentBlock)
	i.SetFunc(f.Function)
	f.GetProgram().SetVirtualRegister(i)
	insert(i)
}

func (f *FunctionBuilder) EmitUndefine(name string) *Undefined {
	if f.CurrentBlock.finish {
		return nil
	}
	u := NewUndefined(name)
	f.emit(u)
	return u
}

func (f *FunctionBuilder) EmitUnOp(op UnaryOpcode, v Value) Value {
	if f.CurrentBlock.finish {
		return nil
	}
	u := NewUnOp(op, v)
	f.emit(u)
	return u
}

func (f *FunctionBuilder) EmitBinOp(op BinaryOpcode, x, y Value) Value {
	if f.CurrentBlock.finish {
		return nil
	}
	v := NewBinOp(op, x, y)
	f.emit(v)
	return v
}

func (f *FunctionBuilder) EmitIf(cond Value) *If {
	if f.CurrentBlock.finish {
		return nil
	}
	ifSSA := NewIf(cond)
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

func (f *FunctionBuilder) EmitReturn(vs []Value) *Return {
	if f.CurrentBlock.finish {
		return nil
	}
	r := NewReturn(vs)
	f.emit(r)
	f.CurrentBlock.finish = true
	f.Return = append(f.Return, r)
	return r
}

func (f *FunctionBuilder) EmitCall(c *Call) *Call {
	if f.CurrentBlock.finish {
		return nil
	}
	f.emit(c)
	return c
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
	return f.emitMake(nil, nil, nil, nil, nil, Len, Cap)
}
func (f *FunctionBuilder) EmitMakeSlice(i Value, low, high, max Value) *Make {
	return f.emitMake(i, i.GetType(), low, high, max, nil, nil)
}

func (f *FunctionBuilder) EmitConstInstAny() *ConstInst {
	return f.EmitConstInst(struct{}{})
}
func (f *FunctionBuilder) EmitConstInstNil() *ConstInst {
	return f.EmitConstInst(nil)
}
func (f *FunctionBuilder) EmitConstInstWithUnary(i any, un int) *ConstInst {
	ci := f.EmitConstInst(i)
	ci.Unary = un
	return ci
}

func (f *FunctionBuilder) EmitConstInst(i any) *ConstInst {
	if f.CurrentBlock.finish {
		return nil
	}
	ci := NewConst(i)
	f.emitEx(ci, func(i Instruction) {
		// pass
	})
	return ci
}

func (f *FunctionBuilder) EmitField(i, key Value) Value {
	if f.CurrentBlock.finish {
		return nil
	}
	return f.getFieldWithCreate(i, key, false)
}
func (f *FunctionBuilder) EmitFieldMust(i, key Value) *Field {
	if f.CurrentBlock.finish {
		return nil
	}
	if field, ok := ToField(f.getFieldWithCreate(i, key, true)); ok {
		return field
	}
	return nil
}

func (f *FunctionBuilder) EmitUpdate(address, v Value) *Update {
	if f.CurrentBlock.finish {
		return nil
	}
	// CheckUpdateType(address.GetType(), v.GetType())
	s := NewUpdate(address, v)
	f.emit(s)
	return s
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
	key = f.EmitField(n, NewConst("key"))
	field = f.EmitField(n, NewConst("field"))
	ok = f.EmitField(n, NewConst("ok"))
	return
}

func (f *FunctionBuilder) EmitErrorHandler(try, catch *BasicBlock) *ErrorHandler {
	if f.CurrentBlock.finish {
		return nil
	}
	e := NewErrorHandler(try, catch)
	block := f.CurrentBlock
	block.AddSucc(try)
	block.AddSucc(catch)
	try.AddSucc(catch)
	f.emit(e)
	return e
}

func (f *FunctionBuilder) EmitPanic(info Value) *Panic {
	if f.CurrentBlock.finish {
		return nil
	}
	p := &Panic{
		anInstruction: NewInstruction(),
		anValue:       NewValue(),
		Info:          info,
	}
	f.emit(p)
	return p
}

func (f *FunctionBuilder) EmitRecover() *Recover {
	if f.CurrentBlock.finish {
		return nil
	}
	r := &Recover{
		anInstruction: NewInstruction(),
		anValue:       NewValue(),
	}
	r.SetType(BasicTypes[Any])
	f.emit(r)
	return r
}
