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
		f.currentDef[variable] = make(map[*BasicBlock]*Values)
	}
	if vs, ok := f.currentDef[variable][block]; !ok {
		f.currentDef[variable][block] = &Values{
			v:    value,
			next: nil,
		}

	} else {
		f.currentDef[variable][block] = &Values{
			v:    value,
			next: vs,
		}
	}
}

func (f *Function) readVariableByBlock(variable string, block *BasicBlock) Value {
	if block.skip {
		return nil
	}
	if map2, ok := f.currentDef[variable]; ok {
		if value, ok := map2[block]; ok {
			return value.v
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
		v = NewPhi(f, block, variable).Build()
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
	b.inCompletePhi = nil
	b.isSealed = true
}

// --------------- `f.freevalue`

func (f *Function) BuildFreeValue(variable string) Value {
	// for parent := f.parent; parent != nil; parent = parent.parent {
	var build func(*Function) Value
	build = func(fun *Function) Value {
		if fun == nil {
			fmt.Printf("warn: con't found variable %s in function %s and parent-function %s\n", variable, f.name, fun.name)
			return nil
		}
		if v := fun.readVariable(variable); v != nil {
			return v
		} else {
			if v := build(fun.parent); v != nil {
				freevalue := &Parameter{
					variable:    variable,
					Func:        fun,
					user:        []User{},
					isFreevalue: true,
				}
				fun.FreeValues = append(fun.FreeValues, freevalue)
				fun.writeVariable(variable, freevalue)
				return freevalue
			} else {
				return nil
			}
		}
	}

	return build(f)
}

func (f *Function) CanBuildFreeValue(variable string) bool {
	for parent := f.parent; parent != nil; parent = parent.parent {
		if v := parent.readVariable(variable); v != nil {
			return true
		}
	}
	return false
}

// --------------- `f.symbol` hanlder, read && write

func (f *Function) getFieldWithCreate(i Value, key Value, create bool) *Field {
	if i, ok := i.(*Interface); ok {
		if field, ok := i.field[key]; ok {
			return field
		}
	}
	if parent := f.parent; parent != nil {
		// find in parent
		if field := parent.readField(key.String()); field != nil {
			return field
		}
	}

	if create {
		field := &Field{
			anInstruction: f.newAnInstuction(),
			Key:           key,
			I:             i,
			update:        make([]Value, 0),
			users:         make([]User, 0),
		}

		f.emit(field)
		fixupUseChain(field)
		return field
	} else {
		return nil
	}
}
func (f *Function) readField(key string) *Field {
	return f.getFieldWithCreate(f.symbol, NewConst(key), false)
}
func (f *Function) newField(key string) *Field {
	return f.getFieldWithCreate(f.symbol, NewConst(key), true)
}

func (f *Function) writeField(key string, v Value) {
	field := f.getFieldWithCreate(f.symbol, NewConst(key), true)
	if field == nil {
		panic(fmt.Sprintf("writeField: %s not found", key))
	}
	if field.GetLastValue() != v {
		f.emitUpdate(field, v)
	}
}
