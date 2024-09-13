package ssa

import "github.com/yaklang/yaklang/common/utils"

// get field value
func (b *FunctionBuilder) getFieldValue(object, key Value, wantFunction bool) Value {

	if ret := b.getStaticFieldValue(object, key, wantFunction); ret != nil {
		return ret
	}
	// normal method
	if wantFunction {
		if fun := GetMethod(object.GetType(), key.String()); fun != nil {
			fun.SetObject(object)
			return fun
		}
	}

	res := checkCanMemberCallExist(object, key)
	// normal member
	// use name  peek value
	if ret := b.PeekValueInThisFunction(res.name); ret != nil {
		return ret
	}

	// default member
	value := b.createDefaultMember(res, object, key, wantFunction)
	b.AssignVariable(b.CreateVariable(res.name), value)
	return value
}

func (b *FunctionBuilder) getStaticFieldValue(object, key Value, wantFunction bool) Value {
	// only static member and method need to be checked
	if !b.SupportClassStaticModifier {
		return nil
	}
	// get member or method
	getValueFromClass := func(class *ClassBluePrint) Value {
		var get func(string) Value
		if wantFunction {
			get = class.GetStaticMethod
		} else {
			get = class.GetStaticMember
		}
		return get(key.String())
	}
	// className.Key
	if un, ok := ToUndefined(object); ok && un.Kind == UndefinedValueInValid {
		if bp := b.GetClassBluePrint(un.GetName()); bp != nil {
			if ret := getValueFromClass(bp); ret != nil {
				return ret
			}
		}
	}

	// classInstance.Key
	if blueprint, ok := object.GetType().(*ClassBluePrint); ok {
		if ret := getValueFromClass(blueprint); ret != nil {
			return ret
		}
	}
	return nil
}

func (b *FunctionBuilder) getDefaultMemberOrMethodByClass(object, key Value, method bool) Value {
	if !b.SupportClass {
		return nil
	}
	// class blue print
	bluePrint, ok := ToClassBluePrintType(object.GetType())
	if !ok {
		return nil
	}
	if method {
		if normalMethod := bluePrint.GetNormalMethod(key.String()); !utils.IsNil(normalMethod) {
			return normalMethod
		}
	} else {
		if member := bluePrint.GetNormalMember(key.String()); !utils.IsNil(member) {
			return member
		}
	}
	return nil
}

func (b *FunctionBuilder) createDefaultMember(res checkMemberResult, object, key Value, wantFunction bool) Value {
	// create undefined memberCall value if the value can not be peeked
	name := res.name
	memberHandler := func(typ Type, member Value) {
		if wantFunction {
			t := NewFunctionTypeDefine(name, nil, nil, false)
			t.SetIsMethod(true, object.GetType())
		}
		objType := object.GetType()
		if objType != nil {
			if fts := objType.GetFullTypeNames(); len(fts) != 0 {
				typ.SetFullTypeNames(fts)
			}
		}
		member.SetType(typ)
		setMemberCallRelationship(object, key, member)
		setMemberVerboseName(member)
	}
	if para, ok := ToParameter(object); ok {
		if member, ok2 := para.GetStringMember(key.String()); ok2 {
			return member
		}
		member := b.NewParameterMember(name, para, key)
		memberHandler(res.typ, member)
		return member
	}
	if field := b.getDefaultMemberOrMethodByClass(object, key, wantFunction); !utils.IsNil(field) {
		return field
	}
	recoverScope := b.SetCurrent(object, true)
	un := b.writeUndefine(name)
	recoverScope()
	if res.exist {
		un.Kind = UndefinedMemberValid
	} else {
		un.Kind = UndefinedMemberInValid
	}
	memberHandler(res.typ, un)
	return un
}

func (b *FunctionBuilder) checkAndCreatDefaultMember(res checkMemberResult, object, key Value) Value {
	// 	recoverScope := b.SetCurrent(object, true)
	if ret := b.PeekValueInThisFunction(res.name); ret != nil {
		return ret
	}

	// need default member
	return b.createDefaultMember(res, object, key, false)
}
