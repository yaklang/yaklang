package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

func NewInterface(parentI User, typ Type, low, high, max, Len, Cap Value, block *BasicBlock) *Interface {
	i := &Interface{
		anInstruction: newAnInstuction(block),
		parentI:       parentI,
		low:           low,
		high:          high,
		max:           max,
		Len:           Len,
		Cap:           Cap,
		users:         make([]User, 0),
	}
	i.SetType(typ)
	fixupUseChain(i)
	return i
}

func NewUpdate(address *Field, v Value, block *BasicBlock) *Update {
	s := &Update{
		anInstruction: newAnInstuction(block),
		Value:         v,
		Address:       address,
	}
	fixupUseChain(s)
	return s
}

func (b *FunctionBuilder) CreateInterfaceWithVs(keys []Value, vs []Value) *Interface {
	hasKey := true
	if len(keys) == 0 {
		hasKey = false
	}
	lValueLen := NewConst(len(vs))
	ityp := NewInterfaceType()
	itf := b.EmitInterfaceBuildNewType(lValueLen, lValueLen)
	for i, rv := range vs {
		var key Value
		if hasKey {
			key = keys[i]
		} else {
			key = NewConst(i)
		}
		field := b.EmitFieldMust(itf, key)
		field.SetType(rv.GetType())
		b.emitUpdate(field, rv)
		ityp.AddField(key, rv.GetType())
	}
	ityp.Finish()
	ityp.Len = len(vs)
	itf.SetType(ityp)
	return itf
}

// --------------- `f.symbol` hanlder, read && write
func (b *FunctionBuilder) getFieldWithCreate(i User, key Value, create bool) Value {
	var ftyp Type
	if I, ok := i.(*Interface); ok {
		if I.buildField != nil {
			return I.buildField(key.String())
		}
	}

	if t := i.GetType(); !utils.IsNil(t) && t.GetTypeKind() == InterfaceTypeKind {
		if it, ok := t.(*InterfaceType); ok {
			ftyp, _ = it.GetField(key)
		}
	}
	if index := slices.IndexFunc(i.GetValues(), func(v Value) bool {
		if f, ok := v.(*Field); ok {
			if f.Key == key {
				return true
			}
		}
		return false
	}); index != -1 {
		return i.GetValues()[index]
	}

	if parent := b.parent; parent != nil {
		// find in parent
		if field := parent.builder.ReadField(key.String()); field != nil {
			return field
		}
	}

	if create {
		field := &Field{
			anInstruction: newAnInstuction(b.CurrentBlock),
			Key:           key,
			I:             i,
			Update:        make([]Value, 0),
			users:         make([]User, 0),
		}
		if ftyp != nil {
			field.SetType(ftyp)
		}
		b.emit(field)
		fixupUseChain(field)
		return field
	} else {
		return nil
	}
}

func (b *FunctionBuilder) GetField(i User, key Value, create bool) *Field {
	if field, ok := b.getFieldWithCreate(i, key, create).(*Field); ok {
		return field
	}
	return nil

}
func (b *FunctionBuilder) ReadField(key string) *Field {
	return b.GetField(b.symbol, NewConst(key), false)
}
func (b *FunctionBuilder) NewField(key string) *Field {
	return b.GetField(b.symbol, NewConst(key), true)
}

func (b *FunctionBuilder) writeField(key string, v Value) {
	field := b.NewField(key)
	if field == nil {
		panic(fmt.Sprintf("writeField: %s not found", key))
	}
	if field.GetLastValue() != v {
		b.emitUpdate(field, v)
	}
}
