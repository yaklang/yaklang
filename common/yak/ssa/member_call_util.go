package ssa

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

// value
func setMemberCallRelationship(obj, key, member Value) {
	if utils.IsNil(obj) || utils.IsNil(key) || utils.IsNil(member) {
		log.Debugf("BUG: setMemberCallRelationship called with nil value: %v, %v, %v", obj, key, member)
		return
	}
	if GetKeyString(key) == "" {
		log.Warnf("setMemberCallRelationship empty key obj=%s typ=%v member=%s", obj.GetVerboseName(), obj.GetType(), member.GetVerboseName())
	}
	obj.AddMember(key, member)
	if memberObj := member.GetObject(); utils.IsNil(memberObj) || memberObj.GetId() == obj.GetId() {
		member.SetObject(obj)
	}
	if memberKey := member.GetKey(); utils.IsNil(memberKey) {
		member.SetKey(key)
	}
	if user, ok := obj.(User); ok {
		key.AddUser(user)
	}
	//todo：fix one value for more object-key

	handlerMemberCall := func(obj Value) {
		for _, edgeID := range obj.(*Phi).Edge {
			edgeValue, ok := obj.GetValueById(edgeID)
			if !ok || edgeValue == nil {
				continue
			}
			if _, ok := edgeValue.GetMember(key); ok { // 避免循环
				continue
			}
			if _, ok := edgeValue.(*Call); ok {
				setMemberCallRelationship(edgeValue, key, member)
			}
			if und, ok := edgeValue.(*Undefined); ok {
				if und.Kind == UndefinedValueInValid {
					setMemberCallRelationship(edgeValue, key, member)
				}
			}
		}
	}

	if phi, ok := ToPhi(obj); ok {
		for _, edgeId := range phi.Edge {
			edgeValue, ok := obj.GetValueById(edgeId)
			if !ok || edgeValue == nil {
				continue
			}
			if und, ok := ToUndefined(edgeValue); ok { // 遇到库类和return phi value
				if und.Kind == UndefinedValueValid || und.Kind == UndefinedValueReturn {
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
	exist   bool
	name    string
	ObjType Type
	typ     Type
}

type memberCallVisitKey struct {
	typKind      TypeKind
	typID        int64
	typRaw       string
	valueID      int64
	keyID        int64
	wantFunction bool
}

func makeMemberCallVisitKey(value, key Value, typ Type, wantFunction bool) memberCallVisitKey {
	var valueID int64 = -1
	if !utils.IsNil(value) {
		valueID = value.GetId()
	}
	var keyID int64 = -1
	if !utils.IsNil(key) {
		keyID = key.GetId()
	}
	typKind := AnyTypeKind
	typID := int64(-1)
	typRaw := ""
	if !utils.IsNil(typ) {
		typID = typ.GetId()
		typKind = typ.GetTypeKind()
		if typID <= 0 {
			typRaw = typ.RawString()
		}
	}
	return memberCallVisitKey{
		typKind:      typKind,
		typID:        typID,
		typRaw:       typRaw,
		valueID:      valueID,
		keyID:        keyID,
		wantFunction: wantFunction,
	}
}

// check can member call, return member name and type
func checkCanMemberCallExist(value, key Value, function ...bool) (ret checkMemberResult) {
	wantFunction := len(function) > 0 && function[0]
	objTyp := defaultAnyType
	if !utils.IsNil(value) {
		objTyp = value.GetType()
	}
	return checkCanMemberCallExistEx(value, key, objTyp, wantFunction, nil, false)
}

func checkCanMemberCallExistEx(value, key Value, objTyp Type, wantFunction bool, visited map[memberCallVisitKey]struct{}, skipPhi bool) (ret checkMemberResult) {
	if utils.IsNil(value) || utils.IsNil(key) {
		log.Errorf("BUG: checkCanMemberCallExist called with nil value: %v, %v", value, key)
		return
	}

	if utils.IsNil(objTyp) {
		objTyp = defaultAnyType
	}

	if visited == nil {
		if !skipPhi {
			if _, ok := ToPhi(value); ok {
				visited = make(map[memberCallVisitKey]struct{}, 8)
			}
		}
		if visited == nil && objTyp.GetTypeKind() == OrTypeKind {
			visited = make(map[memberCallVisitKey]struct{}, 8)
		}
	}
	if visited != nil {
		vk := makeMemberCallVisitKey(value, key, objTyp, wantFunction)
		if _, ok := visited[vk]; ok {
			ret.exist = true
			ret.ObjType = objTyp
			ret.typ = defaultAnyType
			if constInst, ok := ToConstInst(key); ok {
				if constInst.IsNumber() {
					ret.name = fmt.Sprintf("#%d[%d]", value.GetId(), constInst.Number())
				} else {
					ret.name = fmt.Sprintf("#%d.%s", value.GetId(), constInst.VarString())
				}
			} else {
				ret.name = fmt.Sprintf("#%d.#%d", value.GetId(), key.GetId())
			}
			return ret
		}
		visited[vk] = struct{}{}
		defer delete(visited, vk)
	}

	ret.exist = true
	ret.ObjType = objTyp
	if constInst, ok := ToConstInst(key); ok {
		if constInst.IsNumber() {
			ret.name = fmt.Sprintf("#%d[%d]", value.GetId(), constInst.Number())
		} else {
			ret.name = fmt.Sprintf("#%d.%s", value.GetId(), constInst.VarString())
		}
	} else {
		// key is not const value
		// can't get member value
		ret.name = fmt.Sprintf("#%d.#%d", value.GetId(), key.GetId())
		switch objTyp.GetTypeKind() {
		case SliceTypeKind, MapTypeKind:
			typ, _ := ToObjectType(objTyp)
			ret.typ = typ.FieldType
			return
		case BytesTypeKind, StringTypeKind:
			ret.typ = CreateNumberType()
			return
		default:
			ret.typ = CreateAnyType()
			return
		}
	}

	// if kind == DynamicKind {
	// }

	// check is method
	if method := GetMethod(objTyp, key.String(), true); !utils.IsNil(method) {
		ret.typ = method.GetType()
		return
	}
	if blueprint, b := ToClassBluePrintType(objTyp); b {
		if method := blueprint.GetStaticMethod(key.String()); !utils.IsNil(method) {
			ret.typ = method.GetType()
			return
		}
	}
	if wantFunction {
		if ret.typ == nil {
			ret.exist = false
		}
		return ret
	}

	// Phi value: merge member existence/types from edges (limit recursion depth by skipping nested phi expansion).
	if !skipPhi {
		if phi, ok := ToPhi(value); ok {
			var mergedTypes []Type
			var found bool
			for _, edgeID := range phi.Edge {
				edgeValue, ok := value.GetValueById(edgeID)
				if !ok || edgeValue == nil {
					continue
				}
				subRes := checkCanMemberCallExistEx(edgeValue, key, edgeValue.GetType(), wantFunction, visited, true)
				if subRes.exist {
					found = true
				}
				if !utils.IsNil(subRes.typ) {
					mergedTypes = append(mergedTypes, subRes.typ)
				}
			}
			ret.exist = found
			if len(mergedTypes) == 1 {
				ret.typ = mergedTypes[0]
			} else if len(mergedTypes) > 1 {
				ret.typ = NewOrType(mergedTypes...)
			}
			return ret
		}
	}

	switch objTyp.GetTypeKind() {
	case ObjectTypeKind:
		typ, ok := ToObjectType(objTyp)
		if !ok {
			log.Errorf("checkCanMemberCall: %v is structTypeKind but is not a ObjectType", objTyp)
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
		typ, ok := ToObjectType(objTyp)
		if !ok {
			log.Errorf("checkCanMemberCall: %v is structTypeKind but is not a ObjectType", objTyp)
			break
		}
		if TypeCompare(CreateStringType(), key.GetType()) {
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
		typ, ok := ToObjectType(objTyp)
		if !ok {
			log.Errorf("checkCanMemberCall: %v is TupleTypeKind but is not a ObjectType", objTyp)
			break
		}
		if TypeCompare(CreateNumberType(), key.GetType()) {
			if fieldTyp := typ.GetField(key); fieldTyp != nil {
				ret.typ = fieldTyp
				return
			}
		}
	case MapTypeKind: // string / number
		typ, ok := ToObjectType(objTyp)
		if !ok {
			log.Errorf("checkCanMemberCall: %v is MapTypeKind but is not a ObjectType", objTyp)
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
		typ, ok := ToObjectType(objTyp)
		if !ok {
			log.Errorf("checkCanMemberCall: %v is SliceTypeKind but is not a ObjectType", objTyp)
			break
		}
		if TypeCompare(CreateNumberType(), key.GetType()) {
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
		if TypeCompare(CreateNumberType(), key.GetType()) {
			ret.typ = CreateNumberType()
			return
		} else {
			// type check error
		}
	case AnyTypeKind:
		ret.typ = CreateAnyType()
		return
	case ClassBluePrintTypeKind:
		class := objTyp.(*Blueprint)
		if member := class.GetStaticMember(key.String()); !utils.IsNil(member) {
			ret.typ = member.GetType()
			return
		}
		if member := class.GetNormalMember(key.String()); !utils.IsNil(member) {
			ret.typ = member.GetType()
			return
		}
	case OrTypeKind:
		// 拆开 OrType
		orTyp, ok := ToOrType(objTyp)
		if !ok {
			return
		}
		var mergedTypes []Type
		var found bool
		for _, subTyp := range orTyp.GetTypes() {
			subRes := checkCanMemberCallExistEx(value, key, subTyp, wantFunction, visited, skipPhi)
			if subRes.exist {
				found = true
			}
			if !utils.IsNil(subRes.typ) {
				mergedTypes = append(mergedTypes, subRes.typ)
			}
		}
		ret.exist = found
		if len(mergedTypes) == 1 {
			ret.typ = mergedTypes[0]
		} else if len(mergedTypes) > 1 {
			ret.typ = NewOrType(mergedTypes...)
		}
		return ret
	case NullTypeKind:
	default:
	}
	//保底操作，从val-member中获取
	if member, exist := value.GetMember(key); exist {
		ret.typ = member.GetType()
		return
	}
	member, exist := value.GetStringMember(key.String())
	if exist {
		ret.typ = member.GetType()
		return
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
	if utils.IsNil(key) {
		return ""
	}
	text := ""
	if ci, ok := ToConstInst(key); ok {
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
	if key.GetName() == "" {
		return fmt.Sprintf("%s.%s", value.GetName(), key.String())
	}
	return fmt.Sprintf("%s.%s", value.GetName(), key.GetName())
}
