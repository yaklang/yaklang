package ssa

import (
	"sort"

	"github.com/yaklang/yaklang/common/utils"
)

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
func (f *Function) WriteVariable(variable string, value Value) {
	if b := f.builder; b != nil {
		variable = b.GetIdByBlockSymbolTable(variable)
		b.writeVariableByBlock(variable, value, b.CurrentBlock)
	}
	// if value is InstructionValue
	f.WriteSymbolTable(variable, value)
}

func (f *Function) ReplaceSymbolTable(v InstructionValue, to Value) {
	variable := v.GetVariable()
	// remove
	if t, ok := f.symbolTable[variable]; ok {
		f.symbolTable[variable] = utils.RemoveSliceItem(t, v)
	}
	f.WriteSymbolTable(variable, to)
}

func (f *Function) WriteSymbolTable(variable string, value Value) {
	var v InstructionValue
	switch value := value.(type) {
	case InstructionValue:
		v = value
	case *Const:
		v = &ConstInst{
			Const:         value,
			anInstruction: newAnInstruction(f.builder.CurrentBlock),
		}
	default:
		return
	}

	if utils.IsNil(v) {
		return
	}

	list, ok := f.symbolTable[variable]
	if !ok {
		list = make([]InstructionValue, 0, 1)
	}
	list = append(list, v)
	sort.Slice(list, func(i, j int) bool {
		return list[i].GetPosition().StartLine > list[j].GetPosition().StartLine
	})
	f.symbolTable[variable] = list
	v.SetVariable(variable)
}

func (b *FunctionBuilder) ReplaceVariable(variable string, v, to Value) {
	if m, ok := b.currentDef[variable]; ok {
		for block, value := range m {
			if value == v {
				m[block] = to
			}
		}
	}
}

func (b *FunctionBuilder) writeVariableByBlock(variable string, value Value, block *BasicBlock) {
	if _, ok := b.currentDef[variable]; !ok {
		b.currentDef[variable] = make(map[*BasicBlock]Value)
	}
	b.currentDef[variable][block] = value
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
func (b *FunctionBuilder) ReadVariable(variable string, create bool) (ret Value) {
	variable = b.GetIdByBlockSymbolTable(variable)
	if b.CurrentBlock != nil {
		// for building function
		ret = b.readVariableByBlock(variable, b.CurrentBlock, create)
	} else {
		// for finish function
		ret = b.readVariableByBlock(variable, b.ExitBlock, create)
	}
	return
}
func (b *FunctionBuilder) readVariableByBlock(variable string, block *BasicBlock, create bool) Value {
	if block.Skip {
		return nil
	}
	if map2, ok := b.currentDef[variable]; ok {
		if value, ok := map2[block]; ok {
			return value
		}
	}

	var v Value
	// if block in sealedBlock
	if !block.isSealed {
		if create {
			phi := NewPhi(block, variable, create)
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
			un := NewUndefine(variable, block)
			EmitInst(un)
			v = un
		} else {
			v = nil
		}
	} else if len(block.Preds) == 1 {
		v = b.readVariableByBlock(variable, block.Preds[0], create)
	} else {
		v = NewPhi(block, variable, create).Build()
	}
	// if _, ok := v.(*Undefine); !ok && v != nil {
	if v != nil {
		b.writeVariableByBlock(variable, v, block)
	}
	return v
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

func (f *FunctionBuilder) GetVariableBefore(name string, before Instruction) Value {
	name = f.GetIdByBlockSymbolTable(name)
	if ivs, ok := f.symbolTable[name]; ok {
		for _, iv := range ivs {
			if iv.GetPosition().StartLine < before.GetPosition().StartLine {
				if ci, ok := iv.(*ConstInst); ok {
					return ci.Const
				} else {
					return iv
				}
			}
		}
	}
	return nil
}
