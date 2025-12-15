package ssa

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
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
		if inst, ok := i.GetInstructionById(i.GetId()); ok && inst != nil {
			i = inst
		}
	}

	b := i.GetBlock()
	if b == nil {
		log.Debugf("void block!! %s:%s", i, i.GetRange())
		return
	}
	if phi, ok := ToPhi(i); ok {
		b.Phis = lo.Filter(b.Phis, func(item int64, index int) bool {
			return item != phi.GetId()
		})
	} else {
		// b.Insts = utils.RemoveSliceItem(b.Insts, Instruction(i))
		b.Insts = lo.Filter(b.Insts, func(id int64, index int) bool {
			return id != i.GetId()
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
	if i == nil || i.GetFunc() == nil {
		log.Errorf("BUG: instruction[%s] func is nil", i)
		return func() {}
	}
	builder := i.GetFunc().builder
	parentScope := f.parentScope

	f.CurrentBlock = i.GetBlock()
	f.Function = i.GetFunc()
	if builder != nil { // check nil skip replace scope
		f.parentScope = builder.parentScope
	}
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
	if index := slices.Index(b.Insts, i.GetId()); index == -1 {
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

func (b *FunctionBuilder) EmitFirst(i Instruction, blocks ...*BasicBlock) {
	block := b.CurrentBlock
	if len(blocks) > 0 {
		block = blocks[0]
	}
	if len(block.Insts) == 0 {
		b.emit(i)
	} else {
		if inst, ok := b.GetInstructionById(block.Insts[0]); ok && inst != nil {
			b.EmitInstructionBefore(i, inst)
		}
	}
}

func (f *FunctionBuilder) EmitInstructionBefore(i, before Instruction) {
	f.emitAroundInstruction(i, before, func(i Instruction) {
		insts := f.CurrentBlock.Insts
		if index := slices.Index(insts, before.GetId()); index > -1 {
			// Extend the slice
			insts = append(insts, 0)
			// Move elements to create a new space
			copy(insts[index+1:], insts[index:])
			// Insert new element
			insts[index] = i.GetId()
			f.CurrentBlock.Insts = insts
		}
	})
}

func (f *FunctionBuilder) EmitInstructionAfter(i, after Instruction) {
	f.emitAroundInstruction(i, after, func(i Instruction) {
		insts := f.CurrentBlock.Insts
		if index := slices.Index(insts, after.GetId()); index > -1 {
			// Extend the slice
			insts = append(insts, 0)
			// Move elements to create a new space
			copy(insts[index+2:], insts[index+1:])
			// Insert new element
			insts[index+1] = i.GetId()
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
	if utils.IsNil(i) {
		log.Errorf("this block [%s] is finish, instruction[%s] can't insert!", f.CurrentBlock, i)
	}
	f.emitEx(i, f.EmitOnly)
}

func (f *FunctionBuilder) SetInstructionPosition(i Instruction) {
	f.emitEx(i, func(i Instruction) {})
}

func (f *FunctionBuilder) EmitOnly(i Instruction) {
	f.CurrentBlock.Insts = append(f.CurrentBlock.Insts, i.GetId())
}

func (f *FunctionBuilder) emitEx(i Instruction, insert func(Instruction)) {
	// i.SetScope(f.CurrentScope)
	if i.GetRange() == nil {
		i.SetRange(f.CurrentRange)
	}
	i.SetBlock(f.CurrentBlock)
	i.SetFunc(f.Function)
	f.GetProgram().SetVirtualRegister(i)
	insert(i)
	fixupUseChain(i) // this function should after set Program
}

// EmitUndefined emit undefined value
// the current block is finished.
// NOTE: the object/membercall will create vars in finished blocks
func (f *FunctionBuilder) EmitUndefined(name string) *Undefined {
	u := NewUndefined(name)
	f.EmitFirst(u)
	return u
}

func (f *FunctionBuilder) EmitUnOp(op UnaryOpcode, v Value) Value {
	if f.CurrentBlock.finish {
		return nil
	}
	u := NewUnOp(op, v)
	f.emit(u)
	if c, ok := ToConstInst(HandlerUnOp(u)); ok {
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
	if c, ok := ToConstInst(HandlerBinOp(binOp)); ok {
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
	f.emit(l)
	l.Body = body.GetId()
	l.Exit = exit.GetId()
	f.CurrentBlock.AddSucc(body)
	f.CurrentBlock.AddSucc(exit)
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
	s.SetType(value.GetType())
	value.AddOccultation(s)
	return s
}

func (f *FunctionBuilder) EmitReturn(vs []Value) *Return {
	if f.CurrentBlock.finish {
		return nil
	}
	r := NewReturn(vs)
	f.emit(r)
	f.CurrentBlock.finish = true
	f.IsReturn = true
	f.Return = append(f.Return, r.GetId())

	f.builder.SetReturnSideEffects()

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
		// return nil
	}
	i := NewMake(parentI, typ, low, high, max, Len, Cap)
	f.emit(i)
	saveTypeWithValue(i, typ)
	return i
}

func (f *FunctionBuilder) EmitMakeBuildWithType(typ Type, Len, Cap Value) *Make {
	i := f.emitMake(nil, typ, nil, nil, nil, Len, Cap)
	return i
}

func (f *FunctionBuilder) EmitMakeWithoutType(Len, Cap Value) *Make {
	return f.emitMake(nil, CreateAnyType(), nil, nil, nil, Len, Cap)
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

func (f *FunctionBuilder) EmitConstPointer(o *Variable) Value {
	value := o.GetValue()
	if value == nil {
		value = f.ReadValue(o.GetName())
	}
	verboseName := value.GetVerboseName()
	defer value.SetVerboseName(verboseName)

	keys := []Value{f.EmitConstInstPlaceholder("@pointer"), f.EmitConstInstPlaceholder("@value")}
	values := []Value{f.EmitConstInstPlaceholder(fmt.Sprintf("%s#%d", o.GetName(), o.GetGlobalIndex())), value}

	obj := f.InterfaceAddFieldBuild(2, func(i int) Value {
		return keys[i]
	}, func(i int) Value {
		return values[i]
	})
	if !utils.IsNil(obj) {
		p := f.CreateMemberCallVariable(obj, f.EmitConstInstPlaceholder("@pointer"))
		p.SetKind(ssautil.PointerVariable)

		t := NewPointerType()
		t.SetName("Pointer")
		obj.SetType(t)
	}
	return obj
}

func (f *FunctionBuilder) GetAndCreateOriginPointer(obj Value) *Variable {
	o := f.CreateMemberCallVariable(obj, f.EmitConstInstPlaceholder("@pointer"))
	if o.GetValue() == nil {
		p := f.ReadMemberCallValue(obj, f.EmitConstInstPlaceholder("@pointer"))
		f.AssignVariable(o, p)
	}

	o.SetKind(ssautil.PointerVariable)

	return o
}

func (f *FunctionBuilder) GetOriginPointerName(obj Value) string {
	p := f.ReadMemberCallValue(obj, f.EmitConstInstPlaceholder("@pointer"))

	n := strings.TrimPrefix(p.String(), "&")
	originName, _ := SplitName(n)
	return originName
}

func (f *FunctionBuilder) GetOriginValue(obj Value) Value {
	objectValue := f.ReadMemberCallValue(obj, f.EmitConstInstPlaceholder("@value"))
	p := f.ReadMemberCallValue(obj, f.EmitConstInstPlaceholder("@pointer"))

	n := strings.TrimPrefix(p.String(), "&")
	originName, originGlobalId := SplitName(n)

	scope := f.CurrentBlock.ScopeTable
	if variable := GetFristLocalVariableFromScope(scope, originName); variable != nil {
		if variable.GetGlobalIndex() != originGlobalId {
			return objectValue
		}
	}

	if variable := GetFristVariableFromScopeAndParent(scope, originName); variable != nil {
		if variable.GetCaptured().GetGlobalIndex() != originGlobalId {
			return objectValue
		}
		if originValue := variable.GetValue(); originValue != nil {
			return originValue
		}
	}

	return objectValue
}

func (f *FunctionBuilder) EmitConstInstWithUnary(i any, un int) *ConstInst {
	ci := f.EmitConstInst(i)
	ci.Unary = un
	f.emit(ci)
	return ci
}

func (f *FunctionBuilder) EmitConstInstPlaceholder(i any) *ConstInst {
	ret := f.emitConstInst(i, true)
	// ret.SetType(CreateStringType())
	return ret
}

func (f *FunctionBuilder) EmitConstInst(i any) *ConstInst {
	return f.emitConstInst(i, false)
}

func (f *FunctionBuilder) emitConstInst(i any, isPlaceholder bool) *ConstInst {
	ci := NewConst(i, isPlaceholder)
	f.emit(ci)
	if ci.IsNormalConst() {
		f.GetProgram().AddConstInstruction(ci)
	}
	saveTypeWithValue(ci, ci.GetType())
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
	key = f.ReadMemberCallValue(n, f.EmitConstInstPlaceholder(NextKey.String()))
	field = f.ReadMemberCallValue(n, f.EmitConstInstPlaceholder(NextField.String()))
	ok = f.ReadMemberCallValue(n, f.EmitConstInstPlaceholder(NextOk.String()))
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
	block.Handler = e.GetId()
	return e
}

func (f *FunctionBuilder) EmitErrorCatch(try *ErrorHandler, catchBody *BasicBlock, exception Value) *ErrorCatch {
	e := NewErrorCatch(try, catchBody, exception)
	f.EmitFirst(e)
	try.Catch = append(try.Catch, e.GetId())
	catchBody.Handler = try.GetId()
	return e
}

func (f *FunctionBuilder) EmitPanic(info Value) *Panic {
	if f.CurrentBlock.finish {
		return nil
	}
	p := &Panic{
		anValue: NewValue(),
		Info:    info.GetId(),
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
	r.SetType(CreateAnyType())
	f.emit(r)
	return r
}

func (f *FunctionBuilder) EmitPhi(name string, vs Values) *Phi {
	p := &Phi{
		anValue: NewValue(),
		Edge:    vs.GetIds(),
	}
	p.SetName(name)
	f.emitEx(p, func(i Instruction) {
		f.CurrentBlock.Phis = append(f.CurrentBlock.Phis, p.GetId())
	})
	for _, v := range vs {
		v.AddOccultation(p)
	}
	return p
}

func (f *FunctionBuilder) SetReturnSideEffects() {
	SideEffectsReturn := make(map[*Variable]*FunctionSideEffect)
	var value Value
	scope := f.CurrentBlock.ScopeTable

	for _, se := range f.SideEffects {
		ser := &FunctionSideEffect{
			Name:                 se.Name,
			VerboseName:          se.VerboseName,
			Modify:               se.Modify,
			forceCreate:          se.forceCreate,
			Variable:             se.Variable,
			Kind:                 se.Kind,
			parameterMemberInner: se.parameterMemberInner,
		}

		if variable := scope.ReadVariable(se.Name); variable != nil {
			if find, bind := scope.ReadVariableFromLinkSideEffect(se.Name); find != nil && bind == se.Variable {
				value = find.GetValue()
			} else {
				value = variable.GetValue()
			}
			if _, ok := value.(*SideEffect); ok {
			} else if p, ok := value.(*Parameter); ok && p.IsFreeValue {
			} else {
				ser.Modify = value.GetId()
			}
		}
		variable := se.Variable
		if variable == nil {
			variable = value.GetLastVariable()
		}

		SideEffectsReturn[variable] = ser
	}
	f.SideEffects = nil
	f.SideEffectsReturn = append(f.SideEffectsReturn, SideEffectsReturn)
}

func (f *FunctionBuilder) SwitchFreevalueInSideEffect(name string, se *SideEffect, scopeif ...ScopeIF) *SideEffect {
	var bindVariableId, findVariableId int64
	var bindVariable func(*SideEffect)
	var findVariable func()
	var scope ScopeIF
	if len(scopeif) == 0 {
		scope = f.CurrentBlock.ScopeTable
	} else {
		scope = scopeif[0]
	}

	if variable := ReadVariableFromScopeAndParent(scope, name); variable != nil {
		bindVariable = func(se *SideEffect) {
			if se.CallSite > 0 {
				callSide, ok := f.GetValueById(se.CallSite)
				if !ok || callSide == nil {
					return
				}
				if bindId, ok := callSide.(*Call).Binding[name]; ok {
					bind, ok := f.GetValueById(bindId)
					if !ok || bind == nil {
						return
					}
					bindVariableId = bind.GetLastVariable().GetCaptured().GetId()
					_ = bindVariableId
				}
			}
		}
		findVariable = func() {
			if capture := variable.GetCaptured(); capture != nil {
				findVariableId = capture.GetId()
				_ = findVariableId
			}
		}

		bindVariable(se)
		findVariable()

		edge := []Value{}
		if seValue, ok := f.GetValueById(se.Value); ok && seValue != nil {
			if phi, ok := ToPhi(seValue); ok {
				for _, e := range phi.GetValues() {
					if se, ok := e.(*SideEffect); ok {
						bindVariable(se)
					}
				}
				edge = append(edge, phi.GetValues()...)
				phit := f.EmitPhi(name, edge)

				for i, e := range phit.GetValues() {
					if p, ok := ToParameter(e); ok && p.IsFreeValue {
						newParam := NewParam(name, true, f)
						if bindVariableId == findVariableId {
							value := variable.GetValue()
							newParam.defaultValue = value.GetId()
							phit.Edge[i] = newParam.GetId()
						}
					}
				}
				se.Value = phit.GetId()
			}
		}

	}

	return se
}

func (f *FunctionBuilder) CopyValue(v Value) Value {
	ret := v
	switch v := v.(type) {
	case *ConstInst:
		ret = f.EmitConstInst(v.value)
	case *Make:
		var keys []Value
		var members []Value
		for key, member := range v.GetAllMember() {
			keys = append(keys, key)
			members = append(members, member)
		}
		ret = f.InterfaceAddFieldBuild(len(keys),
			func(i int) Value {
				return keys[i]
			},
			func(i int) Value {
				return members[i]
			})
	case *Phi:
		edgeValues := make(Values, 0, len(v.Edge))
		for _, id := range v.Edge {
			if value, ok := f.GetValueById(id); ok && value != nil {
				edgeValues = append(edgeValues, value)
			}
		}
		phi := f.EmitPhi(v.name, edgeValues)
		phi.CFGEntryBasicBlock = v.CFGEntryBasicBlock
		ret = phi
	}
	ret.SetVerboseName(v.GetVerboseName())
	return ret
}

func SplitName(originName string) (string, int) {
	originGlobalId := 0
	if i := strings.Index(originName, "#"); i > 0 {
		originGlobalId, _ = strconv.Atoi(originName[i+1:])
		originName = originName[:i]
	}
	return originName, originGlobalId
}
