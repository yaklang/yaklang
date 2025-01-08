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

	// normal member call
	return b.getFieldValue(object, key, wantFunction)
}

// create member call variable
func (b *FunctionBuilder) CreateMemberCallVariable(object, key Value) *Variable {
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
		ret := b.CreateVariableForce(name)
		ret.SetMemberCall(extern, key)
		return ret
	}

	// normal member call
	// name := b.getFieldName(object, key)
	res := checkCanMemberCallExist(object, key)
	name := res.name
	if object.GetOpcode() != SSAOpcodeParameter {
		// if member not exist, create undefine member in object position
		b.checkAndCreatDefaultMember(res, object, key)
	}
	// log.Infof("CreateMemberCallVariable: %v, %v", retValue.GetName(), key)
	ret := b.CreateVariableForce(name)
	ret.SetMemberCall(object, key)
	return ret
}
