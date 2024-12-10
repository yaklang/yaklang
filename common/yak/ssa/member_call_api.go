package ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// read member call variable, want method
func (b *FunctionBuilder) ReadMemberCallMethod(object, key Value, sign ...FunctionProcess) Value {
	return b.readMemberCallValueEx(object, key, sign...)
}

// read member call variable, want variable
func (b *FunctionBuilder) ReadMemberCallValue(object, key Value) Value {
	return b.readMemberCallValueEx(object, key)
}

func (b *FunctionBuilder) readMemberCallValueEx(object, key Value, sign ...FunctionProcess) Value {
	if res := b.CheckMemberCallNilValue(object, key, "readMemberCallVariableEx"); res != nil {
		return res
	}

	// to extern lib
	if extern, ok := ToExternLib(object); ok {
		return b.TryBuildExternLibValue(extern, key)
	}

	// normal member call
	return b.getFieldValue(object, key, sign...)
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
		ret := b.CreateVariable(name)
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
	ret := b.CreateVariable(name)
	ret.SetMemberCall(object, key)
	return ret
}
