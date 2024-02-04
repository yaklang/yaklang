package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
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
		v := b.CreateMemberCallVariable(itf, key)
		b.AssignVariable(v, rv)
	}
	ityp.Finish()
	ityp.Len = len(vs)
	itf.SetType(ityp)
	return itf
}

// --------------- `f.symbol` handler, read && write
func (b *FunctionBuilder) getFieldWithCreate(i, key Value, forceCreate bool) Value {
	if ci, ok := ToConst(key); ok {
		ci.isIdentify = true
	}
	field := NewFieldOnly(key, i, b.CurrentBlock)
	b.emit(field)
	return field
}

func (b *FunctionBuilder) checkCanMemberCall(value, key Value) (string, bool) {
	type MemberCallKind int
	const (
		None MemberCallKind = iota
		StringKind
		NumberKind
		DynamicKind
	)

	var name string
	kind := None
	if constInst, ok := ToConst(key); ok {
		if constInst.IsNumber() {
			name = fmt.Sprintf("#%d[%d]", value.GetId(), constInst.Number())
			kind = NumberKind
		}
		if constInst.IsString() {
			name = fmt.Sprintf("#%d.%s", value.GetId(), constInst.VarString())
			kind = StringKind
		}
	} else {
		name = fmt.Sprintf("#%d.#%d", value.GetId(), key.GetId())
		kind = DynamicKind
	}

	if kind == DynamicKind {
		//TODO: check type

		return name, true
	}

	switch value.GetType().GetTypeKind() {
	case StructTypeKind: // string
		typ, ok := ToObjectType(value.GetType())
		if !ok {
			log.Errorf("checkCanMemberCall: %v is structTypeKind but is not a ObjectType", value.GetType())
			break
		}
		if fieldTyp := typ.GetField(key); fieldTyp != nil {
			if TypeCompare(fieldTyp, key.GetType()) {
				return name, true
			} else {
				// type check error
			}
		} else {
			// not this field
		}
	case MapTypeKind: // string / number
		typ, ok := ToObjectType(value.GetType())
		if !ok {
			log.Errorf("checkCanMemberCall: %v is structTypeKind but is not a ObjectType", value.GetType())
			break
		}
		if TypeCompare(typ.FieldType, key.GetType()) {
			return name, true
		} else {
			// type check error
		}
	case SliceTypeKind, BytesTypeKind, StringTypeKind: // number
		if TypeCompare(BasicTypes[NumberTypeKind], key.GetType()) {
			return name, true
		} else {
			// type check error
		}
	case AnyTypeKind:
		return name, true
	default:
	}
	return name, true
}

func (b *FunctionBuilder) getExternLibMemberCall(value, key Value) string {
	return fmt.Sprintf("%s.%s", value.GetName(), key.String())
}

func (b *FunctionBuilder) ReadMemberCallVariable(value, key Value) Value {
	if extern, ok := ToExternLib(value); ok {
		name := b.getExternLibMemberCall(value, key)
		// if ret := b.PeekValue(name); ret != nil {
		// 	return ret
		// }
		if ret := ReadVariableFromScope(b.CurrentBlock.ScopeTable, name); ret != nil {
			return ret.Value
		}

		if ret := extern.BuildField(key.String()); ret != nil {
			return ret
		}

		// handler
		// want := b.TryGetSimilarityKey(pa.GetName(), ci.String())
		want := b.TryGetSimilarityKey(extern.GetName(), key.String())
		b.NewErrorWithPos(Error, SSATAG, b.CurrentRange, ExternFieldError("Lib", extern.GetName(), key.String(), want))
		p := NewParam(name, false, b)
		p.SetExtern(true)
		return p
	}

	if name, ok := b.checkCanMemberCall(value, key); ok {
		if ret := b.PeekValue(name); ret != nil {
			return ret
		}
	}
	return b.EmitField(value, key)
}

func (b *FunctionBuilder) CreateMemberCallVariable(value, key Value) *Variable {
	if _, ok := ToExternLib(value); ok {
		name := b.getExternLibMemberCall(value, key)
		return b.CreateVariable(name, false)
	}

	if name, ok := b.checkCanMemberCall(value, key); ok {
		return b.CreateVariable(name, false)
	}

	return nil
}
