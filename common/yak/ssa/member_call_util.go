package ssa

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// value
func setMemberCallRelationship(obj, key, member Value) {
	obj.AddMember(key, member)
	member.SetObject(obj)
	member.SetKey(key)

	obj.AddMember(key, member)
	member.SetObject(obj)
	member.SetKey(key)

	handlerMemberCall := func(obj Value) {
		for _, v := range obj.(*Phi).Edge {
			if _, ok := v.GetMember(key); ok { // 避免循环
				continue
			}
			if _, ok := v.(*Call); ok {
				setMemberCallRelationship(v, key, member)
			}
			if und, ok := v.(*Undefined); ok {
				if und.Kind == UndefinedValueInValid {
					setMemberCallRelationship(v, key, member)
				}
			}
		}
	}

	if phi, ok := obj.(*Phi); ok {
		for _, v := range phi.Edge {
			if und, ok := ToUndefined(v); ok { // 遇到库类
				if und.Kind == UndefinedValueValid {
					handlerMemberCall(obj)
				}
			}
		}
	}
}

func CombineMemberCallVariableName(caller, callee Value) (string, bool) {
	res := checkCanMemberCallExist(caller, callee)
	return res.name, res.exist
}

type checkMemberResult struct {
	exist bool
	name  string
	typ   Type
}

// check can member call, return member name and type
func checkCanMemberCallExist(value, key Value, function ...bool) (ret checkMemberResult) {
	ret.exist = true
	if constInst, ok := ToConst(key); ok {
		if constInst.IsNumber() {
			ret.name = fmt.Sprintf("#%d[%d]", value.GetId(), constInst.Number())
		}
		if constInst.IsString() {
			ret.name = fmt.Sprintf("#%d.%s", value.GetId(), constInst.VarString())
		}
	} else {
		// key is not const value
		// can't get member value
		ret.name = fmt.Sprintf("#%d.#%d", value.GetId(), key.GetId())
		switch value.GetType().GetTypeKind() {
		case SliceTypeKind, MapTypeKind:
			objTyp, _ := ToObjectType(value.GetType())
			ret.typ = objTyp.FieldType
			return
		case BytesTypeKind, StringTypeKind:
			ret.typ = BasicTypes[NumberTypeKind]
			return
		default:
			ret.typ = CreateAnyType()
			return
		}
	}

	// if kind == DynamicKind {
	// }

	// check is method
	if method := GetMethod(value.GetType(), key.String()); method != nil {
		ret.typ = method.GetType()
		return
	}
	if blueprint, b := ToClassBluePrintType(value.GetType()); b {
		if method := blueprint.GetStaticMethod(key.String()); !utils.IsNil(method) {
			ret.typ = method.GetType()
			return
		}
	}
	if len(function) > 0 && function[0] {
		if ret.typ == nil {
			ret.exist = false
		}
		return ret
	}
	switch value.GetType().GetTypeKind() {
	case ObjectTypeKind:
		typ, ok := ToObjectType(value.GetType())
		if !ok {
			log.Errorf("checkCanMemberCall: %v is structTypeKind but is not a ObjectType", value.GetType())
			break
		}
		if fieldTyp := typ.GetField(key); fieldTyp != nil {
			ret.typ = fieldTyp
		} else {
			// not this field
			ret.typ = CreateAnyType()
		}
		return
	case StructTypeKind: // string
		typ, ok := ToObjectType(value.GetType())
		if !ok {
			log.Errorf("checkCanMemberCall: %v is structTypeKind but is not a ObjectType", value.GetType())
			break
		}
		if TypeCompare(BasicTypes[StringTypeKind], key.GetType()) {
			if fieldTyp := typ.GetField(key); fieldTyp != nil {
				ret.typ = fieldTyp
				return
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
				ret.typ = fieldTyp
				return
			}
		}
	case MapTypeKind: // string / number
		typ, ok := ToObjectType(value.GetType())
		if !ok {
			log.Errorf("checkCanMemberCall: %v is MapTypeKind but is not a ObjectType", value.GetType())
			break
		}
		if TypeCompare(typ.KeyTyp, key.GetType()) {
			if typ.FieldType.GetTypeKind() == AnyTypeKind {
				if fieldTyp := typ.GetField(key); fieldTyp != nil {
					ret.typ = fieldTyp
					return
				}
			}
			ret.typ = typ.FieldType
			return
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
			if typ.FieldType.GetTypeKind() == AnyTypeKind {
				if fieldTyp := typ.GetField(key); fieldTyp != nil {
					ret.typ = fieldTyp
					return
				}
			}
			ret.typ = typ.FieldType
			return
		} else {
			// type check error
		}
	case BytesTypeKind, StringTypeKind: // number
		if TypeCompare(BasicTypes[NumberTypeKind], key.GetType()) {
			ret.typ = BasicTypes[NumberTypeKind]
			return
		} else {
			// type check error
		}
	case AnyTypeKind:
		ret.typ = CreateAnyType()
		return
	case ClassBluePrintTypeKind:
		class := value.GetType().(*Blueprint)
		if member := class.GetStaticMember(key.String()); !utils.IsNil(member) {
			ret.typ = member.GetType()
			return
		}
		if member := class.GetNormalMember(key.String()); !utils.IsNil(member) {
			ret.typ = member.GetType()
			return
		}
	case NullTypeKind:
	default:
	}
	ret.exist = false
	return
}

func getMemberVerboseName(obj, key Value) string {
	return fmt.Sprintf("%s.%s", obj.GetVerboseName(), GetKeyString(key))
}

func setMemberVerboseName(member Value) {
	if !member.IsMember() {
		return
	}
	text := getMemberVerboseName(member.GetObject(), member.GetKey())
	member.SetVerboseName(text)
}

func GetKeyString(key Value) string {
	text := ""
	if ci, ok := ToConst(key); ok {
		text = ci.String()
	}
	if text == "" {
		text = key.GetVerboseName()
	}

	if text == "" {
		rawText := key.GetRange().GetText()
		idx := strings.LastIndex(rawText, ".")
		if idx != -1 {
			text = rawText[idx+1:]
		} else {
			text = rawText
		}
		//if r := key.GetRange(); r != nil && r.SourceCode != nil {
		//	list := strings.Split(*r.SourceCode, ".")
		//	text = list[len(list)-1]
		//}
	}
	return text
}

func (b *FunctionBuilder) CheckMemberCallNilValue(value, key Value, funcName string) Value {
	if utils.IsNil(value) {
		log.Errorf("BUG: %s from nil ssa.Value: %v", funcName, value)
	}
	if utils.IsNil(key) {
		log.Errorf("BUG: %s from nil ssa.Value: %v", funcName, key)
	}
	if utils.IsNil(value) && utils.IsNil(key) {
		log.Error("BUG: ReadMemberCallMethodVariable's value and key is all nil...")
		return b.EmitUndefined("")
	} else if utils.IsNil(value) && !utils.IsNil(key) {
		log.Errorf("BUG:%s's value is nil, key: %v", funcName, key)
		return b.EmitUndefined("")
	} else if !utils.IsNil(value) && utils.IsNil(key) {
		log.Errorf("BUG: %s's key is nil, value: %v", funcName, value)
		return b.EmitUndefined("")
	}
	return nil
}
func getExternLibMemberCall(value, key Value) string {
	return fmt.Sprintf("%s.%s", value.GetName(), key.String())
}
