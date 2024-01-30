package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
)

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

func (b *FunctionBuilder) readVariableByBlockEx(variable string, block *BasicBlock, create bool) []Value {
	if vs := block.GetValuesByVariable(variable); vs != nil {
		return vs
	}

	if block.Skip {
		return nil
	}

	var v Value
	// if block in sealedBlock
	if !block.isSealed {
		if create {
			phi := NewPhi(block, variable, create)
			phi.SetRange(b.CurrentRange)
			block.inCompletePhi = append(block.inCompletePhi, phi)
			v = phi
		}
	} else if len(block.Preds) == 0 {
		// v = nil
		if create && b.CanBuildFreeValue(variable) {
			v = b.BuildFreeValue(variable)
		} else if i := b.TryBuildExternValue(variable); i != nil {
			v = i
		} else if create {
			un := NewUndefined(variable)
			// b.emitInstructionBefore(un, block.LastInst())
			b.EmitToBlock(un, block)
			un.SetRange(b.CurrentRange)
			v = un
		} else {
			v = nil
		}
	} else if len(block.Preds) == 1 {
		vs := b.readVariableByBlockEx(variable, block.Preds[0], create)
		if len(vs) > 0 {
			v = vs[len(vs)-1]
		} else {
			v = nil
		}
	} else {
		phi := NewPhi(block, variable, create)
		phi.SetRange(b.CurrentRange)
		v = phi.Build()
	}
	b.writeVariableByBlock(variable, v, block) // NOTE: why write when the v is nil?
	if v != nil {
		return []Value{v}
	} else {
		return nil
	}
}

// --------------- `f.freeValue`

func (b *FunctionBuilder) BuildFreeValue(variable string) Value {
	freeValue := NewParam(variable, true, b)
	b.FreeValues[variable] = freeValue
	// b.CurrentScope.AddVariable(NewVariable(variable, freeValue), b.CurrentRange)
	b.WriteVariable(variable, freeValue)
	return freeValue
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

// --------------- Read

// ReadValue get value by name
func (b *FunctionBuilder) ReadValue(name string) Value {
	scope := b.CurrentBlock.ScopeTable
	if ret := ReadVariableFromScope(scope, name); ret != nil {
		ret.AddRange(b.CurrentRange, false)
		if ret.Value != nil {
			return ret.Value
		}
	}
	undefine := b.EmitUndefine(name)
	b.WriteVariable(name, undefine)
	return undefine
}

// ReadValueByVariable get value by variable
func (b *FunctionBuilder) ReadValueByVariable(v *Variable) Value {
	if ret := v.GetValue(); ret != nil {
		return ret
	}

	return b.ReadValue(v.GetName())
}

// ----------------- Write

// WriteVariable write value to variable
// will create Variable  and assign value
func (b *FunctionBuilder) WriteVariable(name string, value Value) {
	scope := b.CurrentBlock.ScopeTable
	scope.WriteVariable(name, value)
}

// WriteLocalVariable write value to local variable
func (b *FunctionBuilder) WriteLocalVariable(name string, value Value) {
	scope := b.CurrentBlock.ScopeTable
	scope.WriteLocalVariable(name, value)
}

// ------------------- Assign

// AssignVariable  assign value to variable
func (b *FunctionBuilder) AssignVariable(variable *Variable, value Value) {
	scope := b.CurrentBlock.ScopeTable
	scope.AssignVariable(variable, value)
}

// ------------------- Create

// CreateVariable create variable
func (b *FunctionBuilder) CreateVariable(name string) *Variable {
	scope := b.CurrentBlock.ScopeTable
	// return scope.CreateVariable(name, nil).(*Variable)
	return scope.CreateVariable(name).(*Variable)
}

// CreateLocalVariable create local variable
func (b *FunctionBuilder) CreateLocalVariable(name string) *Variable {
	scope := b.CurrentBlock.ScopeTable
	return scope.CreateLocalVariable(name).(*Variable)
}
