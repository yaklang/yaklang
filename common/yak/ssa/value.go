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
	return b.ReadMemberCallVariable(caller, callee)
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

	if ret := ReadVariableFromScope(scope, name); ret != nil {
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
			para.defaultValue = parentValue
			para.SetType(parentValue.GetType())
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

	if enableClosureFreeValue {
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
	v := b.CreateVariable(variable)
	b.AssignVariable(v, undefine)
	return undefine
}

// ------------------- Assign

// AssignVariable  assign value to variable
func (b *FunctionBuilder) AssignVariable(variable *Variable, value Value) {
	// log.Infof("AssignVariable: %v, %v typ %s", variable.GetName(), value.GetName(), value.GetType())
	name := variable.GetName()
	_ = name
	if utils.IsNil(value) {
		log.Warnf("assign nil value to variable: %v", name)
		return
	}
	scope := b.CurrentBlock.ScopeTable
	scope.AssignVariable(variable, value)
	checkAssign := func() {
		if value.GetName() == variable.GetName() {
			if value.GetOpcode() == SSAOpcodeFreeValue || value.GetOpcode() == SSAOpcodeParameter {
				return
			}
		}

		if b.TryBuildExternValue(variable.GetName()) != nil {
			b.NewErrorWithPos(Warn, SSATAG, b.CurrentRange, ContAssignExtern(variable.GetName()))
		}

		// if not freeValue, or not `a = a`(just create FreeValue)
		if parentValue, ok := b.getParentFunctionVariable(variable.GetName()); ok {
			parentValue.AddMask(value)
			v := parentValue.GetVariable(variable.GetName())
			b.AddSideEffect(v, value)
		}
		if _, ok := b.RefParameter[variable.GetName()]; ok {
			b.AddForceSideEffect(variable.GetName(), value)
		}
		b.CheckAndSetSideEffect(variable, value)
	}
	checkAssign()

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

func (b *FunctionBuilder) CreateVariable(name string) *Variable {
	return b.createVariableEx(name, false)
}

func (b *FunctionBuilder) createVariableEx(name string, isLocal bool) *Variable {
	scope := b.CurrentBlock.ScopeTable
	ret := scope.CreateVariable(name, isLocal).(*Variable)
	ret.SetDefRange(b.CurrentRange)
	// set offset variable for program
	program := b.GetProgram()
	if program != nil {
		program.SetOffsetVariable(ret, b.CurrentRange)
	}
	return ret
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

func (b *FunctionBuilder) BuildFreeValue(variable string) *Parameter {
	freeValue := NewParam(variable, true, b)
	b.FreeValues[variable] = freeValue
	// b.WriteVariable(variable, freeValue)
	v := b.CreateVariable(variable)
	b.AssignVariable(v, freeValue)
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
		if parentVariable := ReadVariableFromScope(parentScope.scope, name); parentVariable != nil {
			return parentVariable.Value, true
		}
		parentScope = parentScope.next
	}
	return nil, false
}
