package ssa

import "fmt"

// --------------- for assign
type LeftValue interface {
	Assign(Value)
	GetValue() Value
}

type Identifier struct {
	variable string
	f        *Function
}

func (i *Identifier) Assign(v Value) {
	i.f.wirteVariable(i.variable, v)
}

func (i *Identifier) GetValue() Value {
	return i.f.readVariable(i.variable)
}

var _ LeftValue = (*Identifier)(nil)

func (f *Function) wirteVariable(variable string, value Value) {
	f.wirteVariableByBlock(variable, value, f.currentBlock)
}

func (f *Function) readVariable(variable string) Value {
	return f.readVariableByBlock(variable, f.currentBlock)
}

func (f *Function) wirteVariableByBlock(variable string, value Value, block *BasicBlock) {
	_, ok := f.currentDef[variable]
	if !ok {
		f.currentDef[variable] = make(map[*BasicBlock]Value)
	}
	f.currentDef[variable][block] = value
}

func (f *Function) readVariableByBlock(variable string, block *BasicBlock) Value {
	map2, ok := f.currentDef[variable]
	if !ok {
		// in enter block
		if para, ok := f.Param[variable]; ok {
			return para
		}
		fmt.Printf("con't found variable %s in map currentDef", variable)
		panic("")
	}
	value, ok := map2[block]
	if !ok {
		value = f.readVariableRecursive(variable, block)
	}
	if value == nil {
		fmt.Printf("con't found variable %s", variable)
		panic("")
	}
	return value
}

func (f *Function) readVariableRecursive(variable string, block *BasicBlock) Value {
	var v Value
	// if block in sealedBlock
	if !block.isSealed {
		phi := NewPhi(f, block, variable)
		block.inCompletePhi[variable] = phi
		v = phi
	} else if len(block.Preds) == 1 {
		v = f.readVariableByBlock(variable, block.Preds[0])
	} else {
		phi := NewPhi(f, block, variable)
		f.wirteVariableByBlock(variable, phi, block)
		v = phi.Build()
	}
	f.wirteVariableByBlock(variable, v, block)
	return v
}

func (b *BasicBlock) Sealed() {
	for _, p := range b.inCompletePhi {
		p.Build()
	}
	b.isSealed = true
}
