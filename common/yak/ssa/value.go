package ssa

// --------------- Read

// ReadValue get value by name
func (b *FunctionBuilder) ReadValue(name string) Value {
	return b.readValueEx(name, true, false)
}

func (b *FunctionBuilder) ReadValueInThisFunction(name string) Value {
	return b.readValueEx(name, true, true)
}

func (b *FunctionBuilder) PeekValue(name string) Value {
	return b.readValueEx(name, false, false)
}

func (b *FunctionBuilder) PeekValueInThisFunction(name string) Value {
	return b.readValueEx(name, false, true)
}

func (b *FunctionBuilder) readValueEx(
	name string,
	create bool, // disable create undefine
	onlyThisFunction bool, //disable free-value
) Value {
	scope := b.CurrentBlock.ScopeTable
	if ret := ReadVariableFromScope(scope, name); ret != nil {
		if !onlyThisFunction {
			if ret.GetScope() != scope {
				if b.IsParentFunctionVariable(ret) {
					// the ret variable should be FreeValue
					para := b.BuildFreeValue(name)
					para.defaultValue = ret.Value
					return para
				}
			}
		}

		if b.CurrentRange != nil {
			ret.AddRange(b.CurrentRange, false)
		}
		if ret.Value != nil {
			// has value, just return
			return ret.Value
		}
	}

	if ret := b.TryBuildExternValue(name); ret != nil {
		return ret
	}

	if !onlyThisFunction {
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
	b.WriteVariable(variable, undefine)
	return undefine
}

// ReadValueByVariable get value by variable
func (b *FunctionBuilder) ReadValueByVariable(v *Variable) Value {
	if ret := v.GetValue(); ret != nil {
		return ret
	}

	return b.ReadValue(v.GetName())
}

// ----------------- Write

// WriteVariable write value to variable
// will create Variable  and assign value
func (b *FunctionBuilder) WriteVariable(name string, value Value) {
	ret := b.CreateVariable(name, false)
	b.AssignVariable(ret, value)
}

func (b *FunctionBuilder) WriteLocalVariable(name string, value Value) {
	ret := b.CreateVariable(name, true)
	b.AssignVariable(ret, value)
}

// ------------------- Assign

// AssignVariable  assign value to variable
func (b *FunctionBuilder) AssignVariable(variable *Variable, value Value) {
	name := variable.GetName()
	_ = name
	scope := b.CurrentBlock.ScopeTable
	scope.AssignVariable(variable, value)
	// skip FreeValue
	if value.GetOpcode() != OpFreeValue || value.GetName() != variable.GetName() {
		if b.TryBuildExternValue(variable.GetName()) != nil {
			b.NewErrorWithPos(Warn, SSATAG, b.CurrentRange, ContAssignExtern(variable.GetName()))
		}

		// if not freeValue, or not `a = a`(just create FreeValue)
		if parentValue, ok := b.getParentFunctionVariable(variable); ok {
			parentValue.AddMask(value)
		}
	}

	if variable.IsMemberCall() {
		obj, key := variable.GetMemberCall()
		SetMemberCall(obj, key, value)
	}

	if !value.IsExtern() || value.GetName() != variable.GetName() {
		// if value not extern instance
		// or variable assign by extern instance (extern instance but name not equal)
		b.GetProgram().SetInstructionWithName(variable.GetName(), value)
	}
	value.AddVariable(variable)
}

// ------------------- Create

// CreateVariable create variable
func (b *FunctionBuilder) CreateVariable(name string, isLocal bool) *Variable {
	scope := b.CurrentBlock.ScopeTable
	ret := scope.CreateVariable(name, isLocal).(*Variable)
	ret.SetDefRange(b.CurrentRange)
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
	b.WriteVariable(variable, freeValue)
	return freeValue
}

func (b *FunctionBuilder) IsParentFunctionVariable(v *Variable) bool {
	_, ok := b.getParentFunctionVariable(v)
	return ok
}

func (b *FunctionBuilder) getParentFunctionVariable(v *Variable) (Value, bool) {
	// in closure function
	// check is Capture parent-function value
	if b.parentScope != nil {
		if parentVariable := ReadVariableFromScope(b.parentScope, v.GetName()); parentVariable != nil {
			// parent has this variable
			if parentVariable.GetCaptured() == v.GetCaptured() {
				// capture same variable
				return parentVariable.Value, true
			}
		}
	}
	return nil, false
}
