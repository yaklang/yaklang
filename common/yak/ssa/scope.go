package ssa

// --------------- Read

// ReadValue get value by name
func (b *FunctionBuilder) ReadValue(name string) Value {
	return b.readValueEx(name, true)
}

func (b *FunctionBuilder) PeekValue(name string) Value {
	return b.readValueEx(name, false)
}

func (b *FunctionBuilder) readValueEx(name string, create bool) Value {
	scope := b.CurrentBlock.ScopeTable
	if ret := ReadVariableFromScope(scope, name); ret != nil {
		if ret.GetScope() != scope && b.IsParentFunctionVariable(ret) {
			// the ret variable should be FreeValue
			para := b.BuildFreeValue(name)
			para.defaultValue = ret.Value
			return para
		}

		// in main function
		ret.AddRange(b.CurrentRange, false)
		if ret.Value != nil {
			// has value, just return
			return ret.Value
		}
	}

	if ret := b.TryBuildExternValue(name); ret != nil {
		return ret
	}

	if b.parentScope != nil {
		return b.BuildFreeValue(name)
	}

	if create {
		undefine := b.EmitUndefine(name)
		b.WriteVariable(name, undefine)
		return undefine
	}
	return nil
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
	ret := b.CreateVariable(name, true)
	b.AssignVariable(ret, value)
}

// ------------------- Assign

// AssignVariable  assign value to variable
func (b *FunctionBuilder) AssignVariable(variable *Variable, value Value) {
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

	if !value.IsExtern() {
		value.GetProgram().SetInstructionWithName(variable.GetName(), value)
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

func (b *FunctionBuilder) getMemberCallName(value, key Value) string {
	var name string
	scope := b.CurrentBlock.ScopeTable
	variable := scope.GetVariableFromValue(value)
	if variable == nil {
		return ""
	}
	if constInst, ok := ToConst(key); ok {
		if constInst.IsNumber() {
			name = scope.CoverNumberMemberCall(variable, int(constInst.Number()))
		}
		if constInst.IsString() {
			name = scope.CoverStringMemberCall(variable, constInst.VarString())
		}
	} else {
		keyVariable := scope.GetVariableFromValue(key)
		if keyVariable != nil {
			name = scope.CoverDynamicMemberCall(variable, keyVariable)
		}
	}
	return name
}

func (b *FunctionBuilder) ReadMemberCallVariable(value, key Value) Value {
	if externLib, ok := ToExternLib(value); ok {
		if ret := externLib.BuildField(key.String()); ret != nil {
			return ret
		}
		//TODO: create undefine
	}

	//TODO: check value is a object

	name := b.getMemberCallName(value, key)
	if name == "" {
		//TODO: error
		return nil
	}
	return b.ReadValue(name)
}

func (b *FunctionBuilder) CreateMemberCallVariable(value, key Value) *Variable {
	name := b.getMemberCallName(value, key)
	if name == "" {
		//TODO: error
		return nil
	}

	return b.CreateVariable(name, false)
}

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
