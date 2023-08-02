package ssa

import "fmt"

// --------------- for assign
type LeftValue interface {
	Assign(Value)
	GetValue() Value
}

type IdentifierLV struct {
	variable string
	f        *Function
}

func (i *IdentifierLV) Assign(v Value) {
	i.f.wirteVariable(i.variable, v)
}

func (i *IdentifierLV) GetValue() Value {
	return i.f.readVariable(i.variable)
}

var _ LeftValue = (*IdentifierLV)(nil)

func (a *Alloc) Assign(v Value) {
	a.Parent.emitStore(a, v)
}

func (a *Alloc) GetValue() Value {
	return a.v
}

var _ LeftValue = (*Alloc)(nil)

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
		if parent := f.parent; parent != nil {
			if v := parent.readVariable(variable); v != nil {
				alloc := parent.emitAlloc(variable)
				parent.emitStore(alloc, v)
				parent.wirteVariable(variable, alloc)
				f.FreeValue = append(f.FreeValue, alloc)
				load := f.emitLoad(alloc)
				return load
			} else {
				fmt.Printf("con't found variable %s in function %s and parent-function %s", variable, f.name, parent.name)
			}
		}
		return nil
	} else {
		value, ok := map2[block]
		if !ok {
			value = f.readVariableRecursive(variable, block)
		}
		return value
	}
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
