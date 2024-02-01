package ssa

// --------------- Read

// ReadValue get value by name
func (b *FunctionBuilder) ReadValue(name string) Value {
	scope := b.CurrentBlock.ScopeTable
	if ret := ReadVariableFromScope(scope, name); ret != nil {
		ret.AddRange(b.CurrentRange, false)
		if ret.Value != nil {
			return ret.Value
		}
	}
	undefine := b.EmitUndefine(name)
	b.WriteVariable(name, undefine)
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
	ret := b.CreateVariable(name)
	b.AssignVariable(ret, value)
}

// WriteLocalVariable write value to local variable
func (b *FunctionBuilder) WriteLocalVariable(name string, value Value) {
	ret := b.CreateLocalVariable(name)
	b.AssignVariable(ret, value)
}

// ------------------- Assign

// AssignVariable  assign value to variable
func (b *FunctionBuilder) AssignVariable(variable *Variable, value Value) {
	scope := b.CurrentBlock.ScopeTable
	scope.AssignVariable(variable, value)
}

// ------------------- Create

// CreateVariable create variable
func (b *FunctionBuilder) CreateVariable(name string) *Variable {
	scope := b.CurrentBlock.ScopeTable
	// return scope.CreateVariable(name, nil).(*Variable)
	ret := scope.CreateVariable(name).(*Variable)
	ret.SetDefRange(b.CurrentRange)
	return ret
}

// CreateLocalVariable create local variable
func (b *FunctionBuilder) CreateLocalVariable(name string) *Variable {
	scope := b.CurrentBlock.ScopeTable
	ret := scope.CreateLocalVariable(name).(*Variable)
	ret.SetDefRange(b.CurrentRange)
	return ret
}
