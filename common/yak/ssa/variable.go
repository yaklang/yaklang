package ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// // --------------- for assign
// type LeftValue interface {
// 	Assign(Value, *FunctionBuilder)
// 	GetRange() *Range
// 	GetValue(*FunctionBuilder) Value
// }

// // --------------- only point variable to value with `f.currentDef`
// // --------------- is SSA value
// type IdentifierLV struct {
// 	name         string
// 	pos          *Range
// 	isSideEffect bool
// }

// func (i *IdentifierLV) Assign(v Value, f *FunctionBuilder) {
// 	beforeSSAValue := f.PeekLexicalVariableByName(i.name)
// 	if beforeSSAValue != nil {
// 		if freeParam, ok := beforeSSAValue.(*Parameter); ok && freeParam.IsFreeValue {
// 			// freevalue shoule connect to parent lexical name!
// 			if f.parentBuilder != nil {
// 				beforeSSAValue = f.parentBuilder.PeekLexicalVariableByName(i.name)
// 			}
// 		}
// 	}
// 	// if !v.IsExtern() {
// 	// f.CurrentScope.AddVariable(NewVariable(i.name, v), i.GetRange())
// 	// }
// 	f.WriteVariable(i.name, v)
// 	if i.isSideEffect {
// 		if beforeSSAValue != nil {
// 			beforeSSAValue.AddMask(v)
// 		} else {
// 			log.Warn("freeValueParameter is nil, conflict, side effect cannot find the relative freevalue! maybe a **BUG**")
// 		}
// 		f.AddSideEffect(i.name, v)
// 	}
// }

// func (i *IdentifierLV) GetValue(f *FunctionBuilder) Value {
// 	v := f.ReadVariable(i.name)
// 	return v
// }

// func (i *IdentifierLV) GetRange() *Range {
// 	return i.pos
// }
// func (i *IdentifierLV) SetIsSideEffect(b bool) {
// 	i.isSideEffect = b
// }

// func NewIdentifierLV(variable string, pos *Range) *IdentifierLV {
// 	return &IdentifierLV{
// 		name: variable,
// 		pos:  pos,
// 	}
// }

// var _ LeftValue = (*IdentifierLV)(nil)

// // --------------- point variable to value `f.symbol[variable]value`
// // --------------- it's memory address, not SSA value
// func (field *Field) Assign(v Value, f *FunctionBuilder) {
// 	f.EmitUpdate(field, v)
// }

// func (f *Field) GetValue(_ *FunctionBuilder) Value {
// 	return f
// }

// var _ LeftValue = (*Field)(nil)

// --------------- `f.currentDef` handler, read && write
// func (f *FunctionBuilder) WriteVariable(variable string, value Value) {
// 	variable = f.GetScopeLocalVariable(variable)
// 	f.writeVariableByBlock(variable, value, f.CurrentBlock)
// }

func (f *Function) ReplaceVariable(variable string, v, to Value) {
	for _, block := range f.Blocks {
		if vs, ok := block.symbolTable[variable]; ok {
			vs = utils.ReplaceSliceItem(vs, v, to)
			block.symbolTable[variable] = vs
		}
	}
}

func (b *Function) writeVariableByBlock(variable string, value Value, block *BasicBlock) {
	vs := block.GetValuesByVariable(variable)
	if vs == nil {
		vs = make([]Value, 0, 1)
	}
	vs = append(vs, value)
	block.symbolTable[variable] = vs
}

// PeekVariable just same like `ReadVariable` , but `PeekVariable` don't create `Variable`
// if your syntax read variable, please use `ReadVariable`
// if you just want see what Value this variable, just use `PeekVariable`
func (b *FunctionBuilder) PeekVariable(variable string, create bool) Value {
	var ret Value
	b.ReadVariableEx(variable, create, func(vs []Value) {
		if len(vs) > 0 {
			ret = vs[len(vs)-1]
		} else {
			ret = nil
		}
	})

	return ret
}

// PeekLexicalVariableByName find the static variable in lexical scope
func (b *FunctionBuilder) PeekLexicalVariableByName(variable string) Value {
	i := b.PeekVariable(variable, false)
	if i != nil {
		return i
	}
	if b.parentBuilder != nil {
		i := b.parentBuilder.PeekVariable(variable, false)
		if i != nil {
			return i
		}
	}
	return nil
}

// get value by variable and block
//
//	return : undefined \ value \ phi

// * first check builder.currentDef
//
// * if block sealed; just create a phi
// * if len(block.preds) == 0: undefined
// * if len(block.preds) == 1: just recursive
// * if len(block.preds) >  1: create phi and builder
// func (b *FunctionBuilder) ReadVariable(variable string, create bool) Value {
// 	var ret Value
// 	b.ReadVariableEx(variable, create, func(vs []Value) {
// 		if len(vs) > 0 {
// 			ret = vs[len(vs)-1]
// 		} else {
// 			ret = nil
// 		}
// 	})

// 	if ret == nil {
// 		return ret
// 	}

// 	if ret.IsExtern() {
// 		return ret
// 	}

// 	if v := ret.GetVariable(variable); v != nil {
// 		v.AddRange(b.CurrentRange, false)
// 		b.CurrentScope.InsertByRange(v, b.CurrentRange)
// 	} else {
// 		b.CurrentScope.AddVariable(NewVariable(variable, ret), b.CurrentRange)
// 	}
// 	return ret
// }

func (b *FunctionBuilder) ReadVariableBefore(variable string, create bool, before Instruction) Value {
	var ret Value
	b.ReadVariableEx(variable, create, func(vs []Value) {
		for i := len(vs) - 1; i >= 0; i-- {
			if vs[i] == nil {
				continue
			}
			vpos := vs[i].GetRange()
			bpos := before.GetRange()
			if vpos == nil || bpos == nil {
				continue
			}
			if vpos.CompareStart(bpos) <= 0 {
				ret = vs[i]
				return
			}
		}
	})
	return ret
}

func (b *FunctionBuilder) ReadVariableEx(variable string, create bool, fun func([]Value)) {
	// variable = b.GetScopeLocalVariable(variable)
	var ret []Value
	block := b.CurrentBlock
	if block == nil {
		block = b.ExitBlock
	}
	ret = b.readVariableByBlockEx(variable, block, create)
	fun(ret)
}

func (b *FunctionBuilder) deleteVariableByBlock(variable string, block *BasicBlock) {
	delete(block.symbolTable, variable)
}

func (b *FunctionBuilder) readVariableByBlock(variable string, block *BasicBlock, create bool) Value {
	ret := b.readVariableByBlockEx(variable, block, create)
	if len(ret) > 0 {
		return ret[len(ret)-1]
	} else {
		return nil
	}
}

func (block *BasicBlock) GetValuesByVariable(name string) []Value {
	if vs, ok := block.symbolTable[name]; ok && (len(vs) > 0 && vs[0] != nil) {
		return vs
	}
	return nil
}

func (b *FunctionBuilder) readVariableByBlockEx(name string, block *BasicBlock, create bool) []Value {
	if vs := block.GetValuesByVariable(name); vs != nil {
		return vs
	}

	if block.Skip {
		return nil
	}

	getValue := func() Value {
		if !block.isSealed {
			if !create {
				return nil
			}
			phi := NewPhi(block, name, create)
			phi.SetRange(b.CurrentRange)
			block.inCompletePhi = append(block.inCompletePhi, phi)
			return phi
		}
		switch len(block.Preds) {
		case 0:
			{
				// if can capture parent value, just use it
				if value, ok := b.CaptureParentValue(name); ok {
					_ = value
					/* TODO: only handle side-effect,
					but this problem is not solved in here, need handle in Closure-Function.Finish
					example1:
						i = 1
						f = () => {println(i)} // println(1)
					example2:
						i = 1
						f = () => {i++; println(i)} // `println(i+1)` not `println(2)`
					*/
					// return value
					return b.BuildFreeValue(name)
				}

				// if can build extern value, just use it
				if value := b.TryBuildExternValue(name); value != nil {
					return value
				}

				if !create {
					return nil
				}

				// if can't capture parent value,  but has parent function (in closure),
				if b.parentBuilder != nil {
					// build free value
					return b.BuildFreeValue(name)
				}

				// if not parent (in global main function)
				// create undefine value
				un := NewUndefined(name)
				b.EmitToBlock(un, block)
				un.SetRange(b.CurrentRange)
				return un
			}
		case 1:
			{
				// just recursive read pred block
				vs := b.readVariableByBlockEx(name, block.Preds[0], create)
				if len(vs) == 0 {
					return nil
				}
				return vs[len(vs)-1]
			}
		default:
			{
				phi := NewPhi(block, name, create)
				phi.SetRange(b.CurrentRange)
				return phi.Build()
			}
		}
	}

	v := getValue()
	b.writeVariableByBlock(name, v, block) // NOTE: why write when the v is nil?
	if v != nil {
		return []Value{v}
	}
	return nil
}

// --------------- `f.freeValue`

func (b *FunctionBuilder) BuildFreeValue(variable string) Value {
	freeValue := NewParam(variable, true, b)
	b.FreeValues[variable] = freeValue
	// b.CurrentScope.AddVariable(NewVariable(variable, freeValue), b.CurrentRange)
	b.WriteVariable(variable, freeValue)
	return freeValue
}

func (b *FunctionBuilder) CaptureParentValue(name string) (Value, bool) {
	// parent := b.parentBuilder
	// scope := b.parentScope
	// block := b.parentCurrentBlock
	// for parent != nil {
	// 	name = scope.GetLocalVariable(name)
	// 	v := parent.readVariableByBlock(name, block, false)
	// 	if v != nil {
	// 		// if v not extern instance
	// 		// or value assign by extern instance (extern instance but name not equal)
	// 		if (!v.IsExtern()) || (v.IsExtern() && v.GetName() != name) {
	// 			return v, true
	// 		}
	// 	}

	// 	// parent symbol and block
	// 	scope = parent.parentScope
	// 	block = parent.parentCurrentBlock
	// 	// next parent
	// 	parent = parent.parentBuilder
	// }
	return nil, false
}

func (b *FunctionBuilder) CanCaptureParentValue(name string) bool {
	_, ok := b.CaptureParentValue(name)
	return ok
}
func (b *FunctionBuilder) CanBuildFreeValue(variable string) bool {
	// parent := b.parentBuilder
	// scope := b.parentScope
	// block := b.parentCurrentBlock
	// for parent != nil {
	// 	variable = scope.GetLocalVariable(variable)
	// 	v := parent.readVariableByBlock(variable, block, false)
	// 	if v != nil && !v.IsExtern() {
	// 		return true
	// 	}

	// 	// parent symbol and block
	// 	scope = parent.parentScope
	// 	block = parent.parentCurrentBlock
	// 	// next parent
	// 	parent = parent.parentBuilder
	// }
	return false
}

func (v *Variable) assign(value Value) {
	if v.value != nil {
		log.Errorf("variable %s already assigned", v.Name)
		panic("variable already assigned")
	}

	value.GetProgram().SetInstructionWithName(v.Name, value)
	v.value = value
	value.AddVariable(v)
}

func (v *Variable) readValue() Value {
	return v.value
}

// --------------- Read

// ReadValue get value by name
func (b *FunctionBuilder) ReadValue(name string) Value {
	scope := b.CurrentBlock.ScopeTable
	if v := scope.GetLatestVersion(name); v != nil {
		ret := v.Value
		ret.AddRange(b.CurrentRange, false)
		if ret.value != nil {
			return ret.value
		}
	}
	undefine := b.EmitUndefine(name)
	b.WriteVariable(name, undefine)
	return undefine
}

// ReadValueByVariable get value by variable
func (b *FunctionBuilder) ReadValueByVariable(v *Variable) Value {
	return v.readValue()
}

// ----------------- Write

// WriteVariable write value to variable
// will create Variable  and assign value
func (b *FunctionBuilder) WriteVariable(name string, value Value) {
	v := b.CreateVariable(name)
	v.assign(value)
}

// WriteLocalVariable write value to local variable
func (b *FunctionBuilder) WriteLocalVariable(name string, value Value) {
	v := b.CreateLocalVariable(name)
	v.assign(value)
}

// ------------------- Assign

// AssignVariable  assign value to variable
func (b *FunctionBuilder) AssignVariable(variable *Variable, value Value) {
	variable.assign(value)
}

// ------------------- Create

// CreateVariable create variable
func (b *FunctionBuilder) CreateVariable(name string) *Variable {
	return b.createVariableEx(name, false)
}

// CreateLocalVariable create local variable
func (b *FunctionBuilder) CreateLocalVariable(name string) *Variable {
	return b.createVariableEx(name, true)
}
func (b *FunctionBuilder) createVariableEx(name string, local bool) *Variable {
	scope := b.CurrentBlock.ScopeTable
	v := NewVariable(name, b.CurrentRange, scope)
	if local {
		scope.CreateLexicalLocalVariable(name, v)
	} else {
		scope.CreateLexicalVariable(name, v)
	}
	return v
}
