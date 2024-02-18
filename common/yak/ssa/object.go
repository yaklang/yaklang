package ssa

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// value
func SetMemberCall(obj, key, member Value) {
	obj.AddMember(key, member)
	member.SetObject(obj)
	member.SetKey(key)
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
	return field
}

func (b *FunctionBuilder) checkCanMemberCall(value, key Value) (string, Type) {
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
		return name, BasicTypes[AnyTypeKind]
	}

	// check is method
	if ret := GetMethod(value.GetType(), key.String()); ret != nil {
		return name, ret
	}

	switch value.GetType().GetTypeKind() {
	case StructTypeKind: // string
		typ, ok := ToObjectType(value.GetType())
		if !ok {
			log.Errorf("checkCanMemberCall: %v is structTypeKind but is not a ObjectType", value.GetType())
			break
		}
		if TypeCompare(BasicTypes[StringTypeKind], key.GetType()) {
			if fieldTyp := typ.GetField(key); fieldTyp != nil {
				return name, fieldTyp
			} else {
				// not this field
			}
		} else {
			// type check error
		}
	case TupleTypeKind:
		typ, ok := ToObjectType(value.GetType())
		if !ok {
			log.Errorf("checkCanMemberCall: %v is TupleTypeKind but is not a ObjectType", value.GetType())
			break
		}
		if TypeCompare(BasicTypes[NumberTypeKind], key.GetType()) {
			if fieldTyp := typ.GetField(key); fieldTyp != nil {
				return name, fieldTyp
			}
		}
	case MapTypeKind: // string / number
		typ, ok := ToObjectType(value.GetType())
		if !ok {
			log.Errorf("checkCanMemberCall: %v is MapTypeKind but is not a ObjectType", value.GetType())
			break
		}
		if TypeCompare(typ.KeyTyp, key.GetType()) {
			return name, typ.FieldType
		} else {
			// type check error
		}
	case SliceTypeKind:
		typ, ok := ToObjectType(value.GetType())
		if !ok {
			log.Errorf("checkCanMemberCall: %v is SliceTypeKind but is not a ObjectType", value.GetType())
			break
		}
		if TypeCompare(BasicTypes[NumberTypeKind], key.GetType()) {
			return name, typ.FieldType
		} else {
			// type check error
		}
	case BytesTypeKind, StringTypeKind: // number
		if TypeCompare(BasicTypes[NumberTypeKind], key.GetType()) {
			return name, BasicTypes[NumberTypeKind]
		} else {
			// type check error
		}
	case AnyTypeKind:
		return name, BasicTypes[AnyTypeKind]
	default:
	}
	return name, nil
}

func (b *FunctionBuilder) getExternLibMemberCall(value, key Value) string {
	return fmt.Sprintf("%s.%s", value.GetName(), key.String())
}

func (b *FunctionBuilder) ReadMemberCallVariable(value, key Value) Value {
	if extern, ok := ToExternLib(value); ok {
		// write to extern Lib
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

	// name, ok := b.checkCanMemberCall(value, key)
	// if ok {
	// 	if ret := b.PeekValue(name); ret != nil {
	// 		return ret
	// 	}
	// }
	// log.Infof("ReadMemberCallVariable:  %v", key)

	ret, _ := b.createField(value, key)
	return ret
}

func (b *FunctionBuilder) CreateMemberCallVariable(value, key Value) *Variable {
	if _, ok := ToExternLib(value); ok {
		name := b.getExternLibMemberCall(value, key)
		return b.CreateVariable(name, false)
	}

	// if name, ok := b.checkCanMemberCall(value, key); ok {
	_, name := b.createField(value, key)
	// log.Infof("CreateMemberCallVariable: %v, %v", retValue.GetName(), key)
	ret := b.CreateVariable(name, false)
	ret.SetMemberCall(value, key)
	return ret
}

func (b *FunctionBuilder) createField(value, key Value) (Value, string) {

	name, typ := b.checkCanMemberCall(value, key)
	if ret := b.PeekValueInThisFunction(name); ret != nil {
		return ret, name
	}

	RecoverScope := b.SetCurrent(value)
	ret := b.ReadValueInThisFunction(name)
	RecoverScope()

	if undefine, ok := ToUndefined(ret); ok {
		undefine.SetRange(b.CurrentRange)
		// undefine.SetName(b.setMember(key))
		if typ != nil {
			undefine.Kind = UndefinedMemberValid
			undefine.SetType(typ)
		} else {
			undefine.Kind = UndefinedMemberInValid
		}
		SetMemberCall(value, key, undefine)
	}

	return ret, name
}

func GetKeyString(v Value) string {
	if !v.IsMember() {
		return ""
	}

	key := v.GetKey()
	text := ""
	if ci, ok := ToConst(key); ok {
		text = ci.String()
	}
	if text == "" {
		list := strings.Split(*v.GetRange().SourceCode, ".")
		text = list[len(list)-1]
	}
	return text
}
