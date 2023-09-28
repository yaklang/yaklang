package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
)

// --------------- for assign
type LeftValue interface {
	Assign(Value, *FunctionBuilder)
	GetValue(*FunctionBuilder) Value
}

// --------------- only point variable to value with `f.currentDef`
// --------------- is SSA value
type IdentifierLV struct {
	variable string
}

func (i *IdentifierLV) Assign(v Value, f *FunctionBuilder) {
	f.WriteVariable(i.variable, v)
}

func (i *IdentifierLV) GetValue(f *FunctionBuilder) Value {
	v := f.ReadVariable(i.variable)
	if utils.IsNil(v) {
		// v = NewUndefine(i.variable, f.CurrentBlock)
		v = f.EmitUndefine(i.variable)
	}
	return v
}

func NewIndentifierLV(variable string) *IdentifierLV {
	return &IdentifierLV{
		variable: variable,
	}
}

var _ LeftValue = (*IdentifierLV)(nil)

// --------------- point variable to value `f.symbol[variable]value`
// --------------- it's memory address, not SSA value
func (field *Field) Assign(v Value, f *FunctionBuilder) {
	f.emitUpdate(field, v)
}

func (f *Field) GetValue(_ *FunctionBuilder) Value {
	return f
}

var _ LeftValue = (*Field)(nil)

// --------------- `f.currentDef` hanlder, read && write
func (f *Function) WriteVariable(variable string, value Value) {
	if b := f.builder; b != nil {
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

func (b *FunctionBuilder) ReadVariable(variable string) Value {
	variable = b.GetIdByBlockSymbolTable(variable)
	if b.CurrentBlock != nil {
		// for building function
		return b.readVariableByBlock(variable, b.CurrentBlock)
	} else {
		// for finish function
		return b.readVariableByBlock(variable, b.ExitBlock)
	}
}

func (b *FunctionBuilder) writeVariableByBlock(variable string, value Value, block *BasicBlock) {
	if _, ok := b.currentDef[variable]; !ok {
		b.currentDef[variable] = make(map[*BasicBlock]Value)
	}
	b.currentDef[variable][block] = value
}

func (b *FunctionBuilder) readVariableByBlock(variable string, block *BasicBlock) Value {
	if block.Skip {
		return nil
	}
	if map2, ok := b.currentDef[variable]; ok {
		if value, ok := map2[block]; ok {
			return value
		}
	}
	return b.readVariableRecursive(variable, block)
}

func (b *FunctionBuilder) readVariableRecursive(variable string, block *BasicBlock) Value {
	var v Value
	// if block in sealedBlock
	if !block.isSealed {
		phi := NewPhi(block, variable)
		block.inCompletePhi = append(block.inCompletePhi, phi)
		v = phi
	} else if len(block.Preds) == 0 {
		// this is enter block  in this function
	} else if len(block.Preds) == 1 {
		v = b.readVariableByBlock(variable, block.Preds[0])
	} else {
		v = NewPhi(block, variable).Build()
	}
	if v != nil {
		b.writeVariableByBlock(variable, v, block)
	}
	return v
}

func (b *BasicBlock) Sealed() {
	for _, p := range b.inCompletePhi {
		if v := p.Build(); v == nil {
			var res Value
			res = b.Parent.builder.UndefineHijack(p.GetVariable())
			if res == nil {
				res = NewUndefine(p.GetVariable(), p.Block)
			}
			if inst, ok := res.(Instruction); ok {
				EmitBefore(p.Block.LastInst(), inst)
			}
			p.Replace(res)
		}
	}
	b.inCompletePhi = nil
	b.isSealed = true
}

// --------------- `f.freevalue`

func (f *FunctionBuilder) BuildFreeValue(variable string) Value {
	// for parent := f.parent; parent != nil; parent = parent.parent {
	var build func(*FunctionBuilder) Value
	build = func(b *FunctionBuilder) Value {
		if b == nil {
			fmt.Printf("warn: con't found variable %s in function %s and parent-function %s\n", variable, f.Name, b.Name)
			return nil
		}
		if v := b.ReadVariable(variable); v != nil {
			return v
		} else {
			if v := build(b.parent.builder); v != nil {
				freevalue := &Parameter{
					variable:    variable,
					Func:        b.Function,
					users:       []User{},
					isFreevalue: true,
				}
				b.FreeValues = append(b.FreeValues, freevalue)
				b.WriteVariable(variable, freevalue)
				return freevalue
			} else {
				return nil
			}
		}
	}
	return build(f)
}

func (b *FunctionBuilder) CanBuildFreeValue(variable string) bool {
	for parent := b.parent; parent != nil; parent = parent.parent {
		if v := parent.builder.ReadVariable(variable); v != nil {
			return true
		}
	}
	return false
}
