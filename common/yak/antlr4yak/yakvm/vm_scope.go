package yakvm

func (v *Frame) CreateAndSwitchSubScope(table *SymbolTable) {
	v.scope = v.scope.CreateSubScope(table)
	vm := v.vm
	if vm.debugMode && vm.debugger != nil {
		vm.debugger.AddScopeRef(v.scope)
	}
}
func (v *Frame) CurrentScope() *Scope {
	return v.scope
}

func (v *Frame) NewScope(table *SymbolTable) *Scope {
	return v.CurrentScope().CreateSubScope(table)
}

func (v *Frame) ExitScope() {
	v.ExitScopeWithCount(1)
}

func (v *Frame) ExitScopeWithCount(count int) {
	for i := 0; i < count; i++ {
		if v.scope.parent == nil {
			panic("BUG: Exit Scope Error")
		}
		v.scope = v.scope.parent
	}
}
