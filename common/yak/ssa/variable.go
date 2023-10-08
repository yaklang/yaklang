package ssa

import (
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

func NewIndentifierLV(variable string, pos *Position) *IdentifierLV {
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

// --------------- `f.currentDef` hanlder, read && write
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
		f.symbolTable[variable] = utils.Remove(t, v)
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
			Const:         *value,
			anInstruction: newAnInstuction(f.builder.CurrentBlock),
		}
	default:
		return
	}
	if _, ok := f.symbolTable[variable]; !ok {
		f.symbolTable[variable] = make([]InstructionValue, 0, 1)
	}
	f.symbolTable[variable] = append(f.symbolTable[variable], v)
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
		} else if i := b.TryBuildExternInstance(variable); i != nil {
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

// --------------- `f.freevalue`

func (b *FunctionBuilder) BuildFreeValue(variable string) Value {
	freevalue := NewParam(variable, true, b.Function)
	b.FreeValues = append(b.FreeValues, freevalue)
	b.WriteVariable(variable, freevalue)
	return freevalue
}

func (b *FunctionBuilder) CanBuildFreeValue(variable string) bool {
	for parent := b.parent; parent != nil; parent = parent.parent {
		if v := parent.builder.ReadVariable(variable, false); v != nil {
			if IsExternInstanc(v) {
				return false
			} else {
				return true
			}
		}
	}
	return false
}
