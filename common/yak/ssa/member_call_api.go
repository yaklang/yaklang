package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
)

// ReadMemberCallMethodOrValue read member call method or value depends on type
func (b *FunctionBuilder) ReadMemberCallMethodOrValue(object, key Value) Value {
	res := checkCanMemberCallExist(object, key, false)
	if res.exist && res.typ.GetTypeKind() == FunctionTypeKind {
		return b.readMemberCallValueEx(object, key, true)
	}
	return b.readMemberCallValueEx(object, key, false)
}

// read member call variable, want method
func (b *FunctionBuilder) ReadMemberCallMethod(object, key Value) Value {
	return b.readMemberCallValueEx(object, key, true)
}

// read member call variable, want variable
func (b *FunctionBuilder) ReadMemberCallValue(object, key Value) Value {
	return b.readMemberCallValueEx(object, key, false)
}

func (b *FunctionBuilder) ReadMemberCallValueByName(object Value, key string) Value {
	name := fmt.Sprintf("#%d.%s", object.GetId(), key)
	if ret := b.PeekValueInThisFunction(name); ret != nil {
		return ret
	}
	return nil
}

func (b *FunctionBuilder) readMemberCallValueEx(object, key Value, wantFunction bool) Value {
	if res := b.CheckMemberCallNilValue(object, key, "readMemberCallVariableEx"); res != nil {
		return res
	}

	// to extern lib
	if extern, ok := ToExternLib(object); ok {
		return b.TryBuildExternLibValue(extern, key)
	}
	objectt := object
	if se, ok := ToSideEffect(objectt); ok {
		modify, ok := se.GetValueById(se.Value)
		if ok && modify != nil {
			if ref := modify.GetReference(); ref != nil {
				objectt = ref
			}
		}
	}

	// normal member call
	return b.getFieldValue(objectt, key, wantFunction)
}

// create member call variable
func (b *FunctionBuilder) CreateMemberCallVariable(object, key Value, cross ...bool) *Variable {
	iscross := false
	if len(cross) > 0 {
		iscross = cross[0]
	}

	createVariable := func(name string) *Variable {
		if iscross {
			return b.CreateVariableCross(name)
		} else {
			return b.CreateVariable(name)
		}
	}

	if utils.IsNil(object) || utils.IsNil(key) {
		log.Errorf("CreateMemberCallVariable: object or key is nil")
		return nil
	}
	// check
	if object.GetId() == -1 || key.GetId() == -1 {
		log.Infof("CreateMemberCallVariable: %v, %v", object.GetName(), key)
	}
	// extern lib
	if extern, ok := ToExternLib(object); ok {
		name := getExternLibMemberCall(object, key)
		ret := createVariable(name)
		ret.SetMemberCall(extern, key)
		return ret
	}
	objectt := object
	if se, ok := ToSideEffect(objectt); ok {
		modify, ok := se.GetValueById(se.Value)
		if ok && modify != nil {
			if ref := modify.GetReference(); ref != nil {
				objectt = ref
			}
		}
	}

	// normal member call
	// name := b.getFieldName(object, key)
	res := checkCanMemberCallExist(objectt, key)
	name := res.name
	if objectt.GetOpcode() != SSAOpcodeParameter {
		// if member not exist, create undefine member in object position
		b.checkAndCreateDefaultMember(res, objectt, key)
	}
	// log.Infof("CreateMemberCallVariable: %v, %v", retValue.GetName(), key)
	ret := createVariable(name)
	ret.SetMemberCall(objectt, key)
	return ret
}
