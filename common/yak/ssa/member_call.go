package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
)

// get field value
func (b *FunctionBuilder) getFieldValue(object, key Value, wantFunction bool) Value {
	res := checkCanMemberCallExist(object, key, wantFunction)
	// normal member
	// use name  peek value
	if ret := b.PeekValueInThisFunction(res.name); ret != nil {
		if _, ok := ToParameterMember(ret); !ok {
			return ret
		}
	}

	// default member
	value := b.createDefaultMember(res, object, key, wantFunction)
	return value
}

func (b *FunctionBuilder) getStaticFieldValue(object, key Value, wantFunction bool) Value {
	// only static member and method need to be checked
	if !b.isSupportClassStaticModifier() {
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

func (b *FunctionBuilder) InterfaceAddFieldBuild(
	size int, keys func(int) Value, value func(int) Value,
) *Make {
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
	if !b.isSupportClass() {
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
	// normal method
	typ := res.typ
	if wantFunction {
		// this get will call function Builder, then should refresh the typ
		if fun := GetMethod(object.GetType(), key.String()); fun != nil {
			typ = fun.GetType()
		}
	}
	memberHandler := func(member Value) {
		// todo: phi type is anytype,unknown other value
		if typ == nil || typ.GetTypeKind() == AnyTypeKind {
			if wantFunction {
				t := NewFunctionTypeDefine(name, nil, nil, false)
				t.SetIsMethod(true, object.GetType())
				typ = t
			}
			if typ == nil {
				typ = CreateAnyType()
			}
			objType := object.GetType()
			if objType != nil {
				if fts := objType.GetFullTypeNames(); len(fts) != 0 {
					typ.SetFullTypeNames(fts)
				}
			}
		}
		if typ != nil {
			member.SetType(typ)
		}
		setMemberCallRelationship(object, key, member)
		setMemberVerboseName(member)
	}
	writeUndefined := func() *Undefined {
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
	if para, ok := ToParameter(object); ok {
		// if member, ok2 := para.GetStringMember(key.String()); ok2 {
		// 	return member
		// }
		member := b.NewParameterMember(name, para, key)

		memberHandler(member)
		return member
	}
	config := b.prog.config
	if member, ok := ToParameterMember(object); ok {
		if !wantFunction || config.isSupportConstMethod {
			parameterMember := b.NewMoreParameterMember(name, member, key)
			memberHandler(parameterMember)
			return parameterMember
		}
	}
	if ret := b.getStaticFieldValue(object, key, wantFunction); ret != nil {
		return ret
	}
	// this function only try get value field
	if field := b.getDefaultMemberOrMethodByClass(object, key, false); !utils.IsNil(field) {
		return field
	}
	un := writeUndefined()
	memberHandler(un)
	return un
}

func (b *FunctionBuilder) checkAndCreateDefaultMember(res checkMemberResult, object, key Value) {
	// 	recoverScope := b.SetCurrent(object, true)
	if ret := b.PeekValueInThisFunction(res.name); ret != nil {
		return
	}
	if !res.exist {
		object.NewError(Error, "ObjectError",
			InvalidField(res.ObjType.String(), GetKeyString(key)),
		)
	}

	currentScope := b.CurrentBlock.ScopeTable
	if utils.IsNil(object.GetBlock()) {
		return
	}
	objectScope := object.GetBlock().ScopeTable
	if utils.IsNil(currentScope) || utils.IsNil(objectScope) {
		return
	}
	// is sub-scope and not same, just child scope
	if currentScope.IsSameOrSubScope(objectScope) && !currentScope.Compare(objectScope) {
		b.createDefaultMember(res, object, key, false)
	}
}
