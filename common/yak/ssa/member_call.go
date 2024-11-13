package ssa

import "github.com/yaklang/yaklang/common/utils"

// get field value
func (b *FunctionBuilder) getFieldValue(object, key Value, wantFunction bool) Value {
	res := checkCanMemberCallExist(object, key, wantFunction)
	// normal member
	// use name  peek value
	if ret := b.PeekValueInThisFunction(res.name); ret != nil {
		return ret
	}

	// default member
	value := b.createDefaultMember(res, object, key, wantFunction)
	return value
}

func (b *FunctionBuilder) getStaticFieldValue(object, key Value, wantFunction bool) Value {
	// only static member and method need to be checked
	if !b.SupportClassStaticModifier {
		return nil
	}
	// get member or method
	getValueFromClass := func(class *Blueprint) Value {
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
		if bp := b.GetBluePrint(un.GetName()); bp != nil {
			if ret := getValueFromClass(bp); ret != nil {
				return ret
			}
		}
	}

	// classInstance.Key
	if blueprint, ok := object.GetType().(*Blueprint); ok {
		if ret := getValueFromClass(blueprint); ret != nil {
			return ret
		}
	}
	return nil
}

func (b *FunctionBuilder) InterfaceAddFieldBuild(size int, keys func(int) Value, value func(int) Value) *Make {
	// lValueLen := NewConst(size)
	var lValueLen Value = nil
	itf := b.EmitMakeWithoutType(lValueLen, lValueLen)
	if utils.IsNil(itf) {
		return nil
	}
	if b.MarkedVariable != nil {
		itf.SetName(b.MarkedVariable.GetName())
		b.MarkedThisObject = itf
		defer func() {
			b.MarkedThisObject = nil
		}()
	}
	ityp := NewObjectType()
	itf.SetType(ityp)
	for i := 0; i < size; i++ {
		key := keys(i)
		value := value(i)
		v := b.CreateMemberCallVariable(itf, key)
		b.AssignVariable(v, value)
	}
	ityp.Finish()
	// ityp.Len = len(vs)
	return itf
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
	}
	if member := bluePrint.GetNormalMember(key.String()); !utils.IsNil(member) {
		return member
	}
	return nil
}

func (b *FunctionBuilder) createDefaultMember(res checkMemberResult, object, key Value, wantFunction bool) Value {
	// create undefined memberCall value if the value can not be peeked
	name := res.name
	memberHandler := func(typ Type, member Value) {
		// todo: phi type is anytype,unknown other value
		if typ == nil || typ.GetTypeKind() == AnyTypeKind {
			if wantFunction {
				t := NewFunctionTypeDefine(name, nil, nil, false)
				t.SetIsMethod(true, object.GetType())
				typ = t

				objType := object.GetType()
				if objType != nil {
					if fts := objType.GetFullTypeNames(); len(fts) != 0 {
						typ.SetFullTypeNames(fts)
					}
				}
			}
			if typ == nil {
				typ = CreateAnyType()
			}
		}
		if typ != nil {
			member.SetType(typ)
		}
		setMemberCallRelationship(object, key, member)
		setMemberVerboseName(member)
	}
	writeUndefind := func() *Undefined {
		recoverScope := b.SetCurrent(object, true)
		un := b.writeUndefine(name)
		recoverScope()
		if res.exist {
			un.Kind = UndefinedMemberValid
		} else {
			un.Kind = UndefinedMemberInValid
		}
		return un
	}
	// normal method
	if wantFunction {
		if fun := GetMethod(object.GetType(), key.String()); fun != nil {
			fun.SetObject(object)
			un := writeUndefind()
			memberHandler(res.typ, un)
			// un := writeUndefind()
			// memberHandler(res.typ, un)
			return un
		}
	}
	if para, ok := ToParameter(object); ok {
		if member, ok2 := para.GetStringMember(key.String()); ok2 {
			return member
		}
		member := b.NewParameterMember(name, para, key)
		memberHandler(res.typ, member)
		return member
	}
	if ret := b.getStaticFieldValue(object, key, wantFunction); ret != nil {
		return ret
	}
	if field := b.getDefaultMemberOrMethodByClass(object, key, wantFunction); !utils.IsNil(field) {
		return field
	}
	un := writeUndefind()
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
