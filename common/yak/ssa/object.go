package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

func GetFields(u Node) []*Field {
	f := make([]*Field, 0)
	for _, v := range u.GetUsers() {
		if field, ok := ToField(v); ok {
			if field.Obj == u {
				f = append(f, field)
			}
		}
	}
	return f
}

func GetField(u, key Value) *Field {
	fields := GetFields(u)
	if index := slices.IndexFunc(fields, func(v *Field) bool {
		return v.Key.String() == key.String()
	}); index != -1 {
		return fields[index]
	} else {
		return nil
	}
}

func NewMake(parentI Value, typ Type, low, high, step, Len, Cap Value) *Make {
	i := &Make{
		anInstruction: NewInstruction(),
		anValue:       NewValue(),
		low:           low,
		high:          high,
		step:          step,
		parentI:       parentI,
		Len:           Len,
		Cap:           Cap,
	}
	i.SetType(typ)
	return i
}

func NewUpdate(address, v Value) *Update {
	s := &Update{
		anInstruction: NewInstruction(),
		Value:         v,
		Address:       address,
	}
	return s
}

func NewFieldOnly(key, obj Value, block *BasicBlock) *Field {
	f := &Field{
		anInstruction: NewInstruction(),
		anValue:       NewValue(),
		Key:           key,
		Obj:           obj,
		update:        make([]User, 0),
		IsMethod:      false,
	}
	return f
}

// EmitInterfaceMake quick build key=>value based object
func (b *FunctionBuilder) EmitInterfaceMake(f func(feed func(key Value, val Value))) *Make {
	itf := b.EmitMakeWithoutType(NewConst(0), NewConst(0))
	ityp := NewObjectType()
	count := 0
	f(func(key Value, val Value) {
		field := b.EmitFieldMust(itf, key)
		field.SetType(val.GetType())
		b.EmitUpdate(field, val)
		ityp.AddField(key, val.GetType())
		count++
	})
	ityp.Finish()
	ityp.Len = count
	itf.SetType(ityp)
	return itf
}

func (b *FunctionBuilder) CreateInterfaceWithVs(keys []Value, vs []Value) *Make {
	hasKey := true
	if len(keys) == 0 {
		hasKey = false
	}
	lValueLen := NewConst(len(vs))
	ityp := NewObjectType()
	itf := b.EmitMakeWithoutType(lValueLen, lValueLen)
	for i, rv := range vs {
		if utils.IsNil(rv) {
			continue
		}
		var key Value
		if hasKey {
			key = keys[i]
		} else {
			key = NewConst(i)
		}
		field := b.EmitFieldMust(itf, key)
		field.SetType(rv.GetType())
		b.EmitUpdate(field, rv)
		ityp.AddField(key, rv.GetType())
	}
	ityp.Finish()
	ityp.Len = len(vs)
	itf.SetType(ityp)
	return itf
}

// func (b *FunctionBuilder) getExternLibInstanceForLeft(pa *Parameter, ci ConstInst) LeftValue {
// }
func (b *FunctionBuilder) getExternLibInstance(i, key Value) (ret Value) {
	pa, ok := ToExternLib(i)
	ci, ok2 := ToConst(key)
	defer func() {
		if ret != nil {
			pa.Member = append(pa.Member, ret)
		}
	}()
	if ok && ok2 && pa.BuildField != nil {
		if v := pa.BuildField(ci.String()); v != nil {
			return v
		} else {
			// handler
			want := b.TryGetSimilarityKey(pa.GetName(), ci.String())
			b.NewErrorWithPos(Error, SSATAG, b.CurrentRange, ExternFieldError("Lib", pa.GetName(), ci.String(), want))
			p := NewParam(pa.GetName()+"."+ci.String(), false, b.Function)
			p.SetExtern(true)
			return p
		}
	}
	return nil
}

// --------------- `f.symbol` handler, read && write
func (b *FunctionBuilder) getFieldWithCreate(i, key Value, forceCreate bool) Value {
	var fTyp Type

	if !forceCreate {
		// handler extern lib
		if v := b.getExternLibInstance(i, key); v != nil {
			return v
		}
	}

	// use last field
	if f := GetField(i, key); f != nil {
		return f
	}

	if ci, ok := ToConst(key); ok {
		ci.isIdentify = true
	}
	// if it, ok := ToObjectType(i.GetType()); ok {
	// 	if t, _ := it.GetField(key); t != nil {
	// 		fTyp = t
	// 	}
	// }

	// TODO:field freeValue
	// if parent := b.parentBuilder; parent != nil {
	// 	// find in parent
	// 	if field := parent.ReadField(key.String()); field != nil {
	// 		return field
	// 	}
	// }

	// create new field
	field := NewFieldOnly(key, i, b.CurrentBlock)
	if fTyp != nil {
		field.SetType(fTyp)
	}
	b.emit(field)
	return field
}
