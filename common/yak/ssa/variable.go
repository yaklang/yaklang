package ssa

import (
	"fmt"
)

// --------------- for assign
type LeftValue interface {
	Assign(Value, *Function)
	GetValue(*Function) Value
}

// --------------- only point variable to value with `f.currentDef`
// --------------- is SSA value
type IdentifierLV struct {
	variable string
}

func (i *IdentifierLV) Assign(v Value, f *Function) {
	f.writeVariable(i.variable, v)
}

func (i *IdentifierLV) GetValue(f *Function) Value {
	return f.readVariable(i.variable)
}

var _ LeftValue = (*IdentifierLV)(nil)

// --------------- point variable to value `f.symbol[variable]value`
// --------------- it's memory address, not SSA value
func (field *Field) Assign(v Value, f *Function) {
	f.emitUpdate(field, v)
}

func (f *Field) GetValue(_ *Function) Value {
	return f
}

var _ LeftValue = (*Field)(nil)

// --------------- `f.currentDef` hanlder, read && write
func (f *Function) writeVariable(variable string, value Value) {
	f.writeVariableByBlock(variable, value, f.currentBlock)
}

func (f *Function) readVariable(variable string) Value {
	if f.currentBlock != nil {
		// for building function
	return f.readVariableByBlock(variable, f.currentBlock)
	} else {
		// for finish function
		return f.readVariableByBlock(variable, f.ExitBlock)
	}
}

func (f *Function) writeVariableByBlock(variable string, value Value, block *BasicBlock) {
	_, ok := f.currentDef[variable]
	if !ok {
		f.currentDef[variable] = make(map[*BasicBlock]Value)
	}
	f.currentDef[variable][block] = value
}

func (f *Function) readVariableByBlock(variable string, block *BasicBlock) Value {
	if map2, ok := f.currentDef[variable]; ok {
		if value, ok := map2[block]; ok {
			return value
		}
	}
	return f.readVariableRecursive(variable, block)
}

func (f *Function) readVariableRecursive(variable string, block *BasicBlock) Value {
	var v Value
	// if block in sealedBlock
	if !block.isSealed {
		phi := NewPhi(f, block, variable)
		block.inCompletePhi[variable] = phi
		v = phi
	} else if len(block.Preds) == 0 {
		// this is enter block  in this function
	} else if len(block.Preds) == 1 {
		v = f.readVariableByBlock(variable, block.Preds[0])
	} else {
		phi := NewPhi(f, block, variable)
		f.writeVariableByBlock(variable, phi, block)
		v = phi.Build()
	}
	if v != nil {
		f.writeVariableByBlock(variable, v, block)
	}
	return v
}

func (b *BasicBlock) Sealed() {
	for _, p := range b.inCompletePhi {
		p.Build()
	}
	b.isSealed = true
}
