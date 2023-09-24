package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

func IsObject(v Value) bool {
	_, ok := v.(*Object)
	return ok
}

func ToObject(v Value) *Object {
	o, _ := v.(*Object)
	return o
}

func IsField(v Value) bool {
	_, ok := v.(*Field)
	return ok
}

func ToField(v Value) *Field {
	if o, ok := v.(*Field); ok {
		return o
	} else {
		return nil
	}

}

func NewObject(parentI User, typ Type, low, high, max, Len, Cap Value, block *BasicBlock) *Object {
	i := &Object{
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

func (b *FunctionBuilder) CreateInterfaceWithVs(keys []Value, vs []Value) *Object {
	hasKey := true
	if len(keys) == 0 {
		hasKey = false
	}
	lValueLen := NewConst(len(vs))
	ityp := NewObjectType()
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
	var isMethod bool
	if I, ok := i.(*Object); ok {
		if I.buildField != nil {
			if v := I.buildField(key.String()); v != nil {
				return v
			}
		}
	}
	if t := i.GetType(); !utils.IsNil(t) {
		if c, ok := key.(*Const); ok && c.IsString() {
			if v := t.GetMethod(c.VarString()); v != nil {
				isMethod = true
				ftyp = v
			}
		}
	}

	if t := i.GetType(); !utils.IsNil(t) && t.GetTypeKind() == ObjectTypeKind {
		if it, ok := t.(*ObjectType); ok {
			if t, _ := it.GetField(key); t != nil {
				ftyp = t
				isMethod = false
			}
		}
	}
	if index := slices.IndexFunc(i.GetValues(), func(v Value) bool {
		if f, ok := v.(*Field); ok {
			if f.Key == key && f.I == i {
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
		field.isMethod = isMethod
		b.emit(field)
		fixupUseChain(field)
		return field
	} else {
		return nil
	}
}

func (b *FunctionBuilder) NewCaptureField(text string) *Field {
	f := &Field{
		anInstruction: newAnInstuction(b.CurrentBlock),
		Key:           NewConst(text),
		I:             b.GetParentBuilder().GetSymbol(),
		OutCapture:    true,
		Update:        make([]Value, 0),
		users:         make([]User, 0),
	}
	return f
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
