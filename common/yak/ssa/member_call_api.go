package ssa

import "github.com/yaklang/yaklang/common/log"

// read member call variable, want method
func (b *FunctionBuilder) ReadMemberCallMethod(object, key Value) Value {
	return b.readMemberCallValueEx(object, key, true)
}

// read member call variable, want variable
func (b *FunctionBuilder) ReadMemberCallValue(object, key Value) Value {
	return b.readMemberCallValueEx(object, key, false)
}

func (b *FunctionBuilder) readMemberCallValueEx(object, key Value, wantFunction bool) Value {
	if res := b.CheckMemberCallNilValue(object, key, "readMemberCallVariableEx"); res != nil {
		return res
	}

	// to extern lib
	if extern, ok := ToExternLib(object); ok {
		b.readMemberCallInExternLib(extern, key)
	}

	// normal member call
	return b.getFieldValue(object, key, wantFunction)
}

// create member call variable
func (b *FunctionBuilder) CreateMemberCallVariable(object, key Value) *Variable {
	// check
	if object.GetId() == -1 || key.GetId() == -1 {
		log.Infof("CreateMemberCallVariable: %v, %v", object.GetName(), key)
	}
	// extern lib
	if _, ok := ToExternLib(object); ok {
		name := getExternLibMemberCall(object, key)
		return b.CreateVariable(name)
	}

	// normal member call
	// name := b.getFieldName(object, key)
	res := checkCanMemberCallExist(object, key)
	name := res.name
	if !res.exist && object.GetOpcode() != SSAOpcodeParameter {
		//TODO: this create undefine needed?
		// if member not exist, create undefine member in object position
		b.getOriginMember(name, res.typ, object, key)
	}
	// log.Infof("CreateMemberCallVariable: %v, %v", retValue.GetName(), key)
	ret := b.CreateVariable(name)
	ret.SetMemberCall(object, key)
	return ret
}
