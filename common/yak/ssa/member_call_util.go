package ssa

import (
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

func traceMemberND(stage string, object, key Value, detail string) {
	if os.Getenv("YAK_SSA_TRACE_MEMBER_ND") == "" {
		return
	}
	if utils.IsNil(object) || utils.IsNil(key) {
		return
	}
	if _, ok := ToParameter(object); !ok {
		if _, ok := ToParameterMember(object); !ok {
			return
		}
	}
	keyRange := ""
	if r := key.GetRange(); r != nil {
		keyRange = r.GetText()
	}
	keyStable := buildMemberKeyStableSignature(key)
	log.Infof("[member-nd] %s obj=%s(id=%d,op=%s) key=%s(id=%d,op=%s) key_verbose=%q key_range=%q detail=%s",
		stage,
		object.GetVerboseName(), object.GetId(), object.GetOpcode(),
		GetKeyString(key), key.GetId(), key.GetOpcode(),
		key.GetVerboseName(), keyRange, fmt.Sprintf("%s key_stable=%q", detail, keyStable),
	)
}

func buildMemberKeyStableSignature(key Value) string {
	return buildMemberKeyStableSignatureWithVisited(key, make(map[int64]bool))
}

func buildMemberKeyStableSignatureWithVisited(key Value, visited map[int64]bool) string {
	if utils.IsNil(key) {
		return ""
	}
	if id := key.GetId(); id > 0 {
		if visited[id] {
			return fmt.Sprintf("visited:%d", id)
		}
		visited[id] = true
	}
	if param, ok := ToParameter(key); ok {
		if param.IsFreeValue {
			return fmt.Sprintf("parameter:free:%s", param.GetName())
		}
		return fmt.Sprintf("parameter:index:%d:%s", param.FormalParameterIndex, param.GetName())
	}
	if pm, ok := ToParameterMember(key); ok {
		if sig := buildParameterMemberStableSignature(pm, visited); sig != "" {
			return sig
		}
	}
	if keyConst, ok := ToConstInst(key); ok {
		switch {
		case keyConst.IsString():
			return fmt.Sprintf("const:string:%s", keyConst.VarString())
		case keyConst.IsNumber():
			return fmt.Sprintf("const:number:%d", keyConst.Number())
		case keyConst.IsBoolean():
			return fmt.Sprintf("const:boolean:%t", keyConst.Boolean())
		default:
			raw := keyConst.Const.GetRawValue()
			return fmt.Sprintf("const:raw:%T:%v", raw, raw)
		}
	}

	if r := key.GetRange(); r != nil {
		if editor := r.GetEditor(); editor != nil {
			if sourceHash := editor.GetIrSourceHash(); sourceHash != "" {
				return fmt.Sprintf("dynamic:range:%s:%d:%d", sourceHash, r.GetStartOffset(), r.GetEndOffset())
			}
			if filePath := editor.GetFilePath(); filePath != "" {
				return fmt.Sprintf("dynamic:file:%s:%d:%d", filePath, r.GetStartOffset(), r.GetEndOffset())
			}
		}
		if start, end := r.GetStart(), r.GetEnd(); start != nil && end != nil {
			return fmt.Sprintf("dynamic:pos:%s:%s", start, end)
		}
	}

	if verbose := key.GetVerboseName(); verbose != "" {
		return fmt.Sprintf("dynamic:verbose:%s", verbose)
	}
	if name := key.GetName(); name != "" {
		return fmt.Sprintf("dynamic:name:%s", name)
	}
	return fmt.Sprintf("dynamic:id:%d", key.GetId())
}

func buildParameterMemberStableSignature(member *ParameterMember, visited map[int64]bool) string {
	if member == nil {
		return ""
	}
	if object := member.GetObject(); !utils.IsNil(object) {
		if key := member.GetKey(); !utils.IsNil(key) {
			objectStable := buildMemberKeyStableSignatureWithVisited(object, visited)
			keyStable := buildMemberKeyStableSignatureWithVisited(key, visited)
			if objectStable != "" && keyStable != "" {
				return fmt.Sprintf("parameter-member:%s->%s", objectStable, keyStable)
			}
		}
	}

	inner := member.parameterMemberInner
	if inner == nil {
		return ""
	}

	keyStable := inner.MemberCallKeyStable
	if keyStable == "" && inner.MemberCallKey > 0 {
		if key, ok := member.GetValueById(inner.MemberCallKey); ok && !utils.IsNil(key) {
			keyStable = buildMemberKeyStableSignatureWithVisited(key, visited)
		}
	}

	switch inner.MemberCallKind {
	case ParameterMemberCall:
		return fmt.Sprintf("parameter-member:param[%d]->%s", inner.MemberCallObjectIndex, keyStable)
	case FreeValueMemberCall:
		return fmt.Sprintf("parameter-member:free[%s]->%s", inner.MemberCallObjectName, keyStable)
	case MoreParameterMember:
		if fun := member.GetFunc(); fun != nil && inner.MemberCallObjectIndex >= 0 && inner.MemberCallObjectIndex < len(fun.ParameterMembers) {
			parentID := fun.ParameterMembers[inner.MemberCallObjectIndex]
			if parentValue, ok := member.GetValueById(parentID); ok && !utils.IsNil(parentValue) {
				if parentMember, ok := ToParameterMember(parentValue); ok {
					parentStable := buildParameterMemberStableSignature(parentMember, visited)
					if parentStable != "" {
						return fmt.Sprintf("parameter-member:%s->%s", parentStable, keyStable)
					}
				}
			}
		}
		return fmt.Sprintf("parameter-member:member[%d]->%s", inner.MemberCallObjectIndex, keyStable)
	default:
		return ""
	}
}

func getStoredMemberKeyStableSignature(member Value) string {
	if utils.IsNil(member) {
		return ""
	}
	if key := member.GetKey(); !utils.IsNil(key) {
		return buildMemberKeyStableSignature(key)
	}
	if pm, ok := ToParameterMember(member); ok && pm.parameterMemberInner != nil {
		return pm.MemberCallKeyStable
	}
	return ""
}

func sameMemberKey(left, right Value) bool {
	if utils.IsNil(left) || utils.IsNil(right) {
		return false
	}
	if left.GetId() == right.GetId() {
		return true
	}
	leftStable := buildMemberKeyStableSignature(left)
	rightStable := buildMemberKeyStableSignature(right)
	return leftStable != "" && leftStable == rightStable
}

func getExistingMemberValue(object, key Value) (Value, bool) {
	if utils.IsNil(object) || utils.IsNil(key) {
		return nil, false
	}
	if existed, ok := object.GetMember(key); ok && !utils.IsNil(existed) {
		traceMemberND("direct-hit", object, key, fmt.Sprintf("member=%s(id=%d)", existed.GetVerboseName(), existed.GetId()))
		return existed, true
	}
	keyStable := buildMemberKeyStableSignature(key)
	var found Value
	foundStable := ""
	object.ForEachMember(func(memberKey, member Value) bool {
		switch {
		case sameMemberKey(memberKey, key):
			found = member
			foundStable = buildMemberKeyStableSignature(memberKey)
			return false
		case keyStable != "":
			memberStable := getStoredMemberKeyStableSignature(member)
			if memberStable == "" || memberStable != keyStable {
				return true
			}
			found = member
			foundStable = memberStable
			return false
		}
		return true
	})
	if utils.IsNil(found) {
		traceMemberND("semantic-fallback-miss", object, key, fmt.Sprintf("wanted_stable=%q", keyStable))
		return nil, false
	}
	traceMemberND("semantic-fallback-hit", object, key, fmt.Sprintf("member=%s(id=%d) matched_stable=%q", found.GetVerboseName(), found.GetId(), foundStable))
	return found, true
}

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

	handlerMemberCall := func(phi *Phi) {
		if phi == nil {
			return
		}
		for _, edgeID := range phi.Edge {
			edgeValue, ok := phi.GetValueById(edgeID)
			if !ok || edgeValue == nil {
				continue
			}
			if _, ok := getExistingMemberValue(edgeValue, key); ok { // 避免循环
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
					handlerMemberCall(phi)
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

// check can member call, return member name and type
func checkCanMemberCallExist(value, key Value, function ...bool) (ret checkMemberResult) {
	if utils.IsNil(value) || utils.IsNil(key) {
		log.Errorf("BUG: checkCanMemberCallExist called with nil value: %v, %v", value, key)
		return
	}
	ret.exist = true
	valueType := value.GetType()
	ret.ObjType = valueType
	keyText := key.String()
	if constInst, ok := ToConstInst(key); ok {
		if constInst.IsNumber() {
			ret.name = fmt.Sprintf("#%d[%d]", value.GetId(), constInst.Number())
		}
		if constInst.IsString() {
			ret.name = fmt.Sprintf("#%d.%s", value.GetId(), keyText)
		}
	} else {
		// key is not const value
		// can't get member value
		ret.name = fmt.Sprintf("#%d.#%d", value.GetId(), key.GetId())
		switch valueType.GetTypeKind() {
		case SliceTypeKind, MapTypeKind:
			objTyp, _ := ToObjectType(valueType)
			ret.typ = objTyp.FieldType
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
	if method := GetMethod(valueType, keyText, true); !utils.IsNil(method) {
		ret.typ = method.GetType()
		return
	}
	if blueprint, b := ToClassBluePrintType(valueType); b {
		if method := blueprint.GetStaticMethod(keyText); !utils.IsNil(method) {
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
	switch valueType.GetTypeKind() {
	case ObjectTypeKind:
		typ, ok := ToObjectType(valueType)
		if !ok {
			log.Errorf("checkCanMemberCall: %v is structTypeKind but is not a ObjectType", valueType)
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
		typ, ok := ToObjectType(valueType)
		if !ok {
			log.Errorf("checkCanMemberCall: %v is structTypeKind but is not a ObjectType", valueType)
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
		typ, ok := ToObjectType(valueType)
		if !ok {
			log.Errorf("checkCanMemberCall: %v is TupleTypeKind but is not a ObjectType", valueType)
			break
		}
		if TypeCompare(CreateNumberType(), key.GetType()) {
			if fieldTyp := typ.GetField(key); fieldTyp != nil {
				ret.typ = fieldTyp
				return
			}
		}
	case MapTypeKind: // string / number
		typ, ok := ToObjectType(valueType)
		if !ok {
			log.Errorf("checkCanMemberCall: %v is MapTypeKind but is not a ObjectType", valueType)
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
		typ, ok := ToObjectType(valueType)
		if !ok {
			log.Errorf("checkCanMemberCall: %v is SliceTypeKind but is not a ObjectType", valueType)
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
		class := valueType.(*Blueprint)
		if member := class.GetStaticMember(keyText); !utils.IsNil(member) {
			ret.typ = member.GetType()
			return
		}
		if member := class.GetNormalMember(keyText); !utils.IsNil(member) {
			ret.typ = member.GetType()
			return
		}
	case OrTypeKind:
		// 拆开 OrType
		orTyp, ok := ToOrType(value.GetType())
		if !ok {
			return
		}
		var mergedTypes []Type
		var found bool
		for _, subTyp := range orTyp.GetTypes() {
			// 构造一个假的 Value 但类型替换成子类型
			fakeVal := value
			fakeVal.SetType(subTyp)
			subRes := checkCanMemberCallExist(fakeVal, key, function...)
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
	member, exist := getExistingMemberValue(value, key)
	if exist {
		ret.typ = member.GetType()
		return
	}
	ret.exist = false
	return
}

func getMemberVerboseName(obj, key Value) string {
	if utils.IsNil(obj) {
		return GetKeyString(key)
	}
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
