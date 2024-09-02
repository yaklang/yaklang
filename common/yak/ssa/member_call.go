package ssa

// get field value
func (b *FunctionBuilder) getFieldValue(object, key Value, wantFunction bool) Value {
	if ret := b.getStaticFieldValue(object, key, wantFunction); ret != nil {
		return ret
	}

	// normal method
	if wantFunction {
		if fun := GetMethod(object.GetType(), key.String()); fun != nil {
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

func (b *FunctionBuilder) getDefaultMemberByClass(object, key Value) Value {
	if !b.SupportClass {
		return nil
	}
	// class blue print
	bluePrint, ok := ToClassBluePrintType(object.GetType())
	if !ok {
		return nil
	}
	// this object from constructor
	call, ok := ToCall(object)
	if !ok {
		return nil
	}
	if call.Method != bluePrint.Constructor {
		return nil
	}

	// get member
	if member := bluePrint.GetNormalMember(key.String()); member != nil {
		return member
	}
	return nil
}

func (b *FunctionBuilder) createDefaultMember(res checkMemberResult, object, key Value, wantFunction bool) Value {
	if ret := b.getDefaultMemberByClass(object, key); ret != nil {
		return ret
	}
	// create undefined memberCall value if the value can not be peeked
	name := res.name
	var defaultMember Value
	if para, ok := ToParameter(object); ok {
		defaultMember = b.NewParameterMember(name, para, key)
	} else {
		recoverScope := b.SetCurrent(object, true)
		un := b.writeUndefine(name)
		recoverScope()
		if res.exist {
			un.Kind = UndefinedMemberValid
		} else {
			un.Kind = UndefinedMemberInValid
		}
		defaultMember = un
	}
	// Determine the type of member call.
	// If the type is nil and wantFunction , a new type will be created and IsMethod will be set to true to give itself a receiver
	typ := res.typ
	if typ == nil && wantFunction {
		t := NewFunctionTypeDefine(name, nil, nil, false)
		t.SetIsMethod(true, object.GetType())
	}
	objectTyp := object.GetType()
	if objectTyp != nil {
		if fts := objectTyp.GetFullTypeNames(); len(fts) != 0 {
			typ.SetFullTypeNames(fts)
		}
	}
	defaultMember.SetType(typ)

	// set member-call relationship
	setMemberCallRelationship(object, key, defaultMember)
	setMemberVerboseName(defaultMember)
	return defaultMember
}

func (b *FunctionBuilder) checkAndCreatDefaultMember(res checkMemberResult, object, key Value) Value {
	// 	recoverScope := b.SetCurrent(object, true)
	if ret := b.PeekValueInThisFunction(res.name); ret != nil {
		return ret
	}

	// need default member
	return b.createDefaultMember(res, object, key, false)
}
