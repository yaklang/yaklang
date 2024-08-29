package ssa

func (b *FunctionBuilder) readMemberCallInExternLib(extern *ExternLib, key Value) Value {
	// write to extern Lib
	name := getExternLibMemberCall(extern, key)
	if ret := ReadVariableFromScope(b.CurrentBlock.ScopeTable, name); ret != nil {
		return ret.Value
	}

	if ret := extern.BuildField(key.String()); ret != nil {
		// set program offsetMap for extern value
		b.GetProgram().SetOffsetValue(ret, b.CurrentRange)

		// create variable for extern value
		variable := ret.GetVariable(name)
		if variable == nil {
			ret.AddVariable(b.CreateMemberCallVariable(extern, key))
		} else {
			variable.AddRange(b.CurrentRange, true)
		}

		// set member call
		setMemberCallRelationship(extern, key, ret)
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
	return b.createDefaultMember(res, object, key, wantFunction)
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

func (b *FunctionBuilder) createDefaultMember(res checkMemberResult, object, key Value, wantFunction bool) Value {
	// create undefined memberCall value if the value can not be peeked
	name := res.name
	var defaultMember Value
	if para, ok := ToParameter(object); ok {
		defaultMember = b.NewParameterMember(name, para, key)
	} else {
		un := b.writeUndefine(name)
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

func (b *FunctionBuilder) getOriginMember(name string, typ Type, value, key Value) Value {
	recoverScope := b.SetCurrent(value, true)
	origin := b.ReadValueInThisFunction(name)
	recoverScope()
	if undefine, ok := ToUndefined(origin); ok {
		undefine.SetRange(b.CurrentRange)
		// undefine.SetName(b.setMember(key))
		if typ != nil {
			undefine.Kind = UndefinedMemberValid
			undefine.SetType(typ)
		} else {
			undefine.Kind = UndefinedMemberInValid
		}
		setMemberCallRelationship(value, key, undefine)
	}
	setMemberVerboseName(origin)
	return origin
}
