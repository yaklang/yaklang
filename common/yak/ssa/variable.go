package ssa

import "github.com/yaklang/yaklang/common/utils"

// --------------- for assign
type LeftValue interface {
	Assign(Value, *FunctionBuilder)
	GetPosition() *Position
	GetValue(*FunctionBuilder) Value
}

// --------------- only point variable to value with `f.currentDef`
// --------------- is SSA value
type IdentifierLV struct {
	variable string
	pos      *Position
}

func (i *IdentifierLV) Assign(v Value, f *FunctionBuilder) {
	v.AddLeftPositions(i.GetPosition())
	f.WriteVariable(i.variable, v)
}

func (i *IdentifierLV) GetValue(f *FunctionBuilder) Value {
	v := f.ReadVariable(i.variable, true)
	return v
}

func (i *IdentifierLV) GetPosition() *Position {
	return i.pos
}

func NewIdentifierLV(variable string, pos *Position) *IdentifierLV {
	return &IdentifierLV{
		variable: variable,
		pos:      pos,
	}
}

var _ LeftValue = (*IdentifierLV)(nil)

// --------------- point variable to value `f.symbol[variable]value`
// --------------- it's memory address, not SSA value
func (field *Field) Assign(v Value, f *FunctionBuilder) {
	f.EmitUpdate(field, v)
}

func (f *Field) GetValue(_ *FunctionBuilder) Value {
	return f
}

var _ LeftValue = (*Field)(nil)

// --------------- `f.currentDef` handler, read && write
func (f *FunctionBuilder) WriteVariable(variable string, value Value) {
	f.WriteVariableWithBlockSymbol(variable, value, f.GetSymbolTable(), f.CurrentBlock)
}
func (f *Function) WriteVariableWithBlockSymbol(variable string, value Value, blockSymbol *blockSymbolTable, block *BasicBlock) {
	variable = GetIdByBlockSymbolTable(variable, blockSymbol)
	f.writeVariableByBlock(variable, value, block)
}

func (b *Function) ReplaceVariable(variable string, v, to Value) {
	if m, ok := b.symbolTable[variable]; ok {
		for block, value := range m {
			m[block] = utils.ReplaceSliceItem(value, v, to)
		}
	}
}

func (b *Function) writeVariableByBlock(variable string, value Value, block *BasicBlock) {
	if _, ok := b.symbolTable[variable]; !ok {
		b.symbolTable[variable] = make(map[*BasicBlock]Values)
	}
	vs, ok := b.symbolTable[variable][block]
	if !ok {
		vs = make(Values, 0)
	}
	if value.GetVariable() == "" {
		value.SetVariable(variable)
	} else {
		value.AddLeftVariables(variable)
	}
	vs = append(vs, value)
	b.symbolTable[variable][block] = vs
}

// get value by variable and block
//
//	return : undefine \ value \ phi

// * first check builder.currentDef
//
// * if block sealed; just create a phi
// * if len(block.preds) == 0: undefine
// * if len(block.preds) == 1: just recursive
// * if len(block.preds) >  1: create phi and builder
func (b *FunctionBuilder) ReadVariable(variable string, create bool) Value {
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

func (b *FunctionBuilder) ReadVariableBefore(variable string, create bool, before Instruction) Value {
	var ret Value
	b.ReadVariableEx(variable, create, func(vs []Value) {
		for i := len(vs) - 1; i >= 0; i-- {
			vpos := vs[i].GetPosition()
			bpos := before.GetPosition()
			if vpos.StartLine <= bpos.StartLine {
				ret = vs[i]
				return
			}
		}
	})
	return ret
}

func (b *FunctionBuilder) ReadVariableEx(variable string, create bool, fun func([]Value)) {
	variable = b.GetIdByBlockSymbolTable(variable)
	var ret []Value
	block := b.CurrentBlock
	if block == nil {
		block = b.ExitBlock
	}
	ret = b.readVariableByBlockEx(variable, block, create)
	fun(ret)
}

func (b *FunctionBuilder) readVariableByBlock(variable string, block *BasicBlock, create bool) Value {
	ret := b.readVariableByBlockEx(variable, block, create)
	if len(ret) > 0 {
		return ret[len(ret)-1]
	} else {
		return nil
	}
}

func (b *FunctionBuilder) readVariableByBlockEx(variable string, block *BasicBlock, create bool) []Value {
	if block.Skip {
		return nil
	}
	if map2, ok := b.symbolTable[variable]; ok {
		if vs, ok := map2[block]; ok && len(vs) > 0 {
			return vs
		}
	}

	var v Value
	// if block in sealedBlock
	if !block.isSealed {
		if create {
			phi := NewPhi(block, variable, create)
			phi.SetPosition(b.CurrentPos)
			block.inCompletePhi = append(block.inCompletePhi, phi)
			v = phi
		}
	} else if len(block.Preds) == 0 {
		// v = nil
		if b.CanBuildFreeValue(variable) {
			v = b.BuildFreeValue(variable)
		} else if i := b.TryBuildExternValue(variable); i != nil {
			v = i
		} else if create {
			un := NewUndefine(variable)
			// b.emitInstructionBefore(un, block.LastInst())
			b.emitToBlock(un, block)
			v = un
		} else {
			v = nil
		}
	} else if len(block.Preds) == 1 {
		return b.readVariableByBlockEx(variable, block.Preds[0], create)
	} else {
		phi := NewPhi(block, variable, create)
		phi.SetPosition(b.CurrentPos)
		v = phi.Build()
	}
	if v != nil {
		b.writeVariableByBlock(variable, v, block)
		return []Value{v}
	} else {
		return nil
	}
}

// --------------- `f.freeValue`

func (b *FunctionBuilder) BuildFreeValue(variable string) Value {
	freeValue := NewParam(variable, true, b.Function)
	b.FreeValues = append(b.FreeValues, freeValue)
	b.WriteVariable(variable, freeValue)
	return freeValue
}

func (b *FunctionBuilder) CanBuildFreeValue(variable string) bool {
	parent := b.parentBuilder
	symbol := b.parentSymbolBlock
	block := b.parentCurrentBlock
	for parent != nil {
		variable = GetIdByBlockSymbolTable(variable, symbol)
		v := parent.readVariableByBlock(variable, block, false)
		e := parent.externInstance[variable]
		if v != nil && e != v {
			return true
		}

		// parent symbol and block
		symbol = parent.parentSymbolBlock
		block = parent.parentCurrentBlock
		// next parent
		parent = parent.parentBuilder
	}
	return false
}
