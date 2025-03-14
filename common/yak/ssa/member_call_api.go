package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

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
	if se, ok := ToSideEffect(object); ok {
		if ref := se.Value.GetReference(); ref != nil {
			objectt = ref
		}
	}

	// normal member call
	return b.getFieldValue(objectt, key, wantFunction)
}

// create member call variable
func (b *FunctionBuilder) CreateMemberCallVariable(object, key Value, isForce ...bool) *Variable {
	var createVariable func(string) *Variable
	if len(isForce) > 0 && isForce[0] {
		createVariable = func(name string) *Variable {
			return b.CreateVariableForce(name)
		}
	} else {
		createVariable = func(name string) *Variable {
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
	if se, ok := ToSideEffect(object); ok {
		if ref := se.Value.GetReference(); ref != nil {
			objectt = ref
		}
	}

	// normal member call
	// name := b.getFieldName(object, key)
	res := checkCanMemberCallExist(objectt, key)
	name := res.name
	if objectt.GetOpcode() != SSAOpcodeParameter {
		// if member not exist, create undefine member in object position
		b.checkAndCreatDefaultMember(res, objectt, key)
	}
	// log.Infof("CreateMemberCallVariable: %v, %v", retValue.GetName(), key)
	ret := createVariable(name)
	ret.SetMemberCall(objectt, key)
	return ret
}
