package ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// --------------- Read

// ReadValueByVariable get value by variable
func (b *FunctionBuilder) ReadValueByVariable(v *Variable) Value {
	if v == nil {
		return nil
	}
	if ret := v.GetValue(); ret != nil {
		return ret
	}

	if para, ok := ToParameter(v.object); ok {
		res := checkCanMemberCallExist(para, v.key)
		newParamterMember := b.NewParameterMember(res.name, para, v.key)
		newParamterMember.SetType(res.typ)
		setMemberCallRelationship(para, v.key, newParamterMember)
		setMemberVerboseName(newParamterMember)
	}
	if !utils.IsNil(v.object) {
		ret := checkCanMemberCallExist(v.object, v.key)
		if val := b.PeekValueInThisFunction(ret.name); !utils.IsNil(val) {
			return val
		}
		if val := b.getDefaultMemberOrMethodByClass(v.object, v.key, false); !utils.IsNil(val) {
			return val
		}
	}

	return b.ReadValue(v.GetName())
}

// ReadValue get value by name
func (b *FunctionBuilder) ReadValue(name string) Value {
	return b.readValueEx(name, true, true && b.SupportClosure)
}

func (b *FunctionBuilder) ReadOrCreateVariable(name string) Value {
	return b.ReadValue(name)
}

func (b *FunctionBuilder) ReadOrCreateMemberCallVariable(caller, callee Value) Value {
	return b.ReadMemberCallValue(caller, callee)
}

func (b *FunctionBuilder) ReadValueInThisFunction(name string) Value {
	return b.readValueEx(name, true, false)
}

func (b *FunctionBuilder) PeekValueByVariable(v *Variable) Value {
	if ret := v.GetValue(); ret != nil {
		return ret
	}

	return b.PeekValue(v.GetName())
}

func (b *FunctionBuilder) PeekValue(name string) Value {
	return b.readValueEx(name, false, true && b.SupportClosure)
}

func (b *FunctionBuilder) PeekValueInThisFunction(name string) Value {
	return b.readValueEx(name, false, false)
}

func (b *FunctionBuilder) readValueEx(
	name string,
	create bool, // disable create undefine
	enableClosureFreeValue bool, // disable free-value
) Value {
	scope := b.CurrentBlock.ScopeTable
	program := b.GetProgram()
	if ret := ReadVariableFromScopeAndParent(scope, name); ret != nil {
		if b.CurrentRange != nil {
			ret.AddRange(b.CurrentRange, false)
			// set offset variable
			if program != nil {
				program.SetOffsetVariable(ret, b.CurrentRange)
			}
		}
		if ret.Value != nil {
			// has value, just return
			return ret.Value
		}
	}

	if enableClosureFreeValue {
		if parentValue, ok := b.getParentFunctionVariable(name); ok {
			// the ret variable should be FreeValue
			para := b.BuildFreeValue(name)
			para.SetDefault(parentValue)
			para.SetType(parentValue.GetType())
			parentValue.AddOccultation(para)
			return para
		}
	}

	if ret := b.TryBuildExternValue(name); ret != nil {

		// create variable for extern lib value
		variable := ret.GetVariable(name)
		if variable == nil {
			variable = b.CreateVariable(name)
			ret.AddVariable(variable)
			variable.Assign(ret)
		} else {
			variable.AddRange(b.CurrentRange, true)
		}
		// set offset value for extern value
		if program != nil {
			program.SetOffsetValue(ret, b.CurrentRange)
		}
		return ret
	}

	if enableClosureFreeValue && create {
		if b.parentScope != nil {
			return b.BuildFreeValue(name)
		}
	}
	if create {
		return b.writeUndefine(name)
	}
	return nil
}

func (b *FunctionBuilder) writeUndefine(variable string, names ...string) *Undefined {
	name := variable
	if len(names) > 0 {
		name = names[0]
	}
	undefine := b.EmitUndefined(name)
	v := b.CreateVariableForce(variable)
	b.AssignVariable(v, undefine)
	return undefine
}

// ------------------- Assign

// AssignVariable  assign value to variable
func (b *FunctionBuilder) AssignVariable(variable *Variable, value Value) {
	if variable == nil {
		log.Errorf("assign variable is nil")
		return
	}
	// log.Infof("AssignVariable: %v, %v typ %s", variable.GetName(), value.GetName(), value.GetType())
	name := variable.GetName()
	_ = name
	if utils.IsNil(value) {
		log.Infof("assign nil value to variable: %v, it will not work on ssa ir format", name)
		return
	}
	scope := b.CurrentBlock.ScopeTable
	scope.AssignVariable(variable, value)

	if value.GetName() == variable.GetName() {
		if value.GetOpcode() == SSAOpcodeFreeValue || value.GetOpcode() == SSAOpcodeParameter {
			return
		}
	}

	if b.TryBuildExternValue(variable.GetName()) != nil {
		b.NewErrorWithPos(Warn, SSATAG, b.CurrentRange, ContAssignExtern(variable.GetName()))
	}

	// if not freeValue, or not `a = a`(just create FreeValue)
	if !variable.GetLocal() && b.SupportClosure {
		if parentValue, ok := b.getParentFunctionVariable(variable.GetName()); ok &&
			GetFristLocalVariableFromScopeAndParent(scope, variable.GetName()) == nil {
			parentValue.AddMask(value)
			v := parentValue.GetVariable(variable.GetName())
			b.AddSideEffect(v, value)
			para := b.BuildFreeValue(variable.GetName())
			para.SetDefault(parentValue)
			para.SetType(parentValue.GetType())
			parentValue.AddOccultation(para)
		}
	}
	if val, ok := b.RefParameter[variable.GetName()]; ok {
		b.AddForceSideEffect(variable.GetName(), value, val.Index)
	}
	b.CheckAndSetSideEffect(variable, value)

	if !value.IsExtern() || value.GetName() != variable.GetName() {
		// if value not extern instance
		// or variable assign by extern instance (extern instance but name not equal)
		b.GetProgram().SetInstructionWithName(variable.GetName(), value)
	}
}

// ------------------- Create

// CreateVariable create variable
func (b *FunctionBuilder) CreateLocalVariable(name string) *Variable {
	return b.createVariableEx(name, true)
}

func (b *FunctionBuilder) CreateVariableForce(name string, pos ...CanStartStopToken) *Variable {
	return b.createVariableEx(name, false, pos...)
}

func (b *FunctionBuilder) CreateVariable(name string, pos ...CanStartStopToken) *Variable {
	if variable := b.getCurrentScopeVariable(name); variable != nil {
		if value := variable.GetValue(); value != nil {
			if _, ok := ToConst(value); ok {
				return variable
			}
			if _, ok := value.(*SideEffect); ok {
				return variable
			}
		}
	}
	return b.createVariableEx(name, false, pos...)
}

func (b *FunctionBuilder) createVariableEx(name string, isLocal bool, pos ...CanStartStopToken) *Variable {
	scope := b.CurrentBlock.ScopeTable

	ret := scope.CreateVariable(name, isLocal)
	variable := ret.(*Variable)

	r := b.CurrentRange
	if r == nil && len(pos) > 0 {
		r = b.GetCurrentRange(pos[0])
	}
	if r != nil {
		variable.SetDefRange(r)
	}
	// set offset variable for program
	program := b.GetProgram()
	if program != nil {
		program.SetOffsetVariable(variable, b.CurrentRange)
	}
	return variable
}

func (b *FunctionBuilder) CreateVariableHead(name string, pos ...CanStartStopToken) *Variable {
	return b.CreateVariableHeadEx(name, false, pos...)
}

func (b *FunctionBuilder) CreateVariableHeadEx(name string, isLocal bool, pos ...CanStartStopToken) *Variable {
	scope := b.CurrentBlock.ScopeTable
	headScope := scope.GetHead()

	ret := headScope.CreateVariable(name, isLocal)
	variable := ret.(*Variable)

	r := b.CurrentRange
	if r == nil && len(pos) > 0 {
		r = b.GetCurrentRange(pos[0])
	}
	if r != nil {
		variable.SetDefRange(r)
	}
	// set offset variable for program
	program := b.GetProgram()
	if program != nil {
		program.SetOffsetVariable(variable, b.CurrentRange)
	}
	return variable
}

// // CreateLocalVariable create local variable
// func (b *FunctionBuilder) CreateLocalVariable(name string) *Variable {
// 	scope := b.CurrentBlock.ScopeTable
// 	ret := scope.CreateLocalVariable(name).(*Variable)
// 	ret.SetDefRange(b.CurrentRange)
// 	return ret
// }

// func (b *FunctionBuilder) getMemberCallVariable(value, key Value) string {
// 	return name
// }

// --------------- `f.freeValue`

func (b *FunctionBuilder) BuildFreeValue(name string) *Parameter {
	scope := b.CurrentBlock.ScopeTable
	headScope := scope.GetHead()
	if variable := headScope.ReadVariable(name); variable != nil {
		value := variable.GetValue()
		// TODO: 这里读取到的freevalue可能是同名但是不同variable的情况（由side-effect生成的修改外部的freevalue）
		if freeValue, ok := ToParameter(value); ok && freeValue.IsFreeValue && name == freeValue.GetName() {
			return freeValue
		} else {
			freeValue := NewParam(name, true, b)
			b.FreeValues[variable.(*Variable)] = freeValue
			return freeValue
		}
	}

	freeValue := NewParam(name, true, b)
	v := b.CreateVariableHead(name)
	headScope.AssignVariable(v, freeValue)
	b.FreeValues[v] = freeValue

	// b.WriteVariable(variable, freeValue)
	return freeValue
}

func (b *FunctionBuilder) BuildFreeValueByVariable(variable *Variable) *Parameter {
	scope := b.CurrentBlock.ScopeTable
	headScope := scope.GetHead()

	name := variable.GetName()
	freeValue := NewParam(name, true, b)
	freeValue.SetRange(b.CurrentRange)
	if find := headScope.ReadVariable(name); find != nil && variable != find {
		if freeValueFinded, ok := ToParameter(find.GetValue()); ok {
			return freeValueFinded
		}
	} else {
		//b.FreeValues[variable] = freeValue
		// b.WriteVariable(variable, freeValue)
	}
	return freeValue
}

func (b *FunctionBuilder) IsParentFunctionVariable(v *Variable) bool {
	_, ok := b.getParentFunctionVariable(v.GetName())
	return ok
}

func (b *FunctionBuilder) getParentFunctionVariable(name string) (Value, bool) {
	// in closure function
	// check is Capture parent-function value
	parentScope := b.parentScope
	for parentScope != nil {
		if parentVariable := ReadVariableFromScopeAndParent(parentScope.scope, name); parentVariable != nil {
			return parentVariable.Value, true
		}
		parentScope = parentScope.next
	}
	return nil, false
}

func (b *FunctionBuilder) getCurrentScopeVariable(name string) *Variable {
	scope := b.CurrentBlock.ScopeTable
	if variable := ReadVariableFromScope(scope, name); variable != nil {
		return variable
	}
	return nil
}
