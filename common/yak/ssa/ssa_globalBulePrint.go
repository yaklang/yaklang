package ssa

import "github.com/yaklang/yaklang/common/utils"

func (b *FunctionBuilder) AddGlobalVariable(name string, valueFunc func() Value) {
	prog := b.GetProgram()

	if prog.GlobalVariablesBlueprint == nil {
		return
	}

	prog.GlobalVariablesBlueprint.AddLazyBuilder(func() {
		value := valueFunc()
		if utils.IsNil(value) {
			return
		}

		prog.GlobalVariablesBlueprint.RegisterStaticMember(name, value, false)
		globalVarsContainer := prog.GlobalVariablesBlueprint.Container()
		if utils.IsNil(globalVarsContainer) {
			return
		}

		scope := b.CurrentBlock.ScopeTable
		for _, v := range scope.GetAllVariables() {
			if object := v.GetValue().GetObject(); object != nil && object.GetId() == value.GetId() {
				variable := b.CreateMemberCallVariable(globalVarsContainer, b.EmitConstInstPlaceholder(v.GetName()))
				b.AssignVariable(variable, v.GetValue())
			}
		}
		variable := b.CreateMemberCallVariable(globalVarsContainer, b.EmitConstInstPlaceholder(name))
		b.AssignVariable(variable, value)
	})

}

func (b *FunctionBuilder) CheckGlobalVariablePhi(l *Variable, r Value) bool {
	name := l.GetName()
	prog := b.GetProgram()

	if prog.GlobalVariablesBlueprint == nil {
		return false
	}

	globalVarsContainer := prog.GlobalVariablesBlueprint.Container()
	if globalVarsContainer == nil {
		return false
	}

	for i, _ := range globalVarsContainer.GetAllMember() {
		if i.String() == name {
			globalVarsContainer.GetAllMember()[i] = r
			return true
		}
	}
	return false
}

func (b *FunctionBuilder) GetGlobalVariables() map[string]Value {
	variables := make(map[string]Value)
	prog := b.GetProgram()

	if prog.GlobalVariablesBlueprint == nil {
		return variables
	}

	globalVarsContainer := prog.GlobalVariablesBlueprint.Container()
	if globalVarsContainer == nil {
		return variables
	}

	for i, m := range globalVarsContainer.GetAllMember() {
		variables[i.String()] = m
	}
	return variables
}

func (b *FunctionBuilder) GetGlobalVariableR(name string) Value {
	prog := b.GetProgram()

	if m, ok := prog.GetGlobalVariable(name); ok {
		return m
	}
	return nil
}

func (b *FunctionBuilder) LoadGlobalVariable() {
	prog := b.GetProgram()
	globalVarsContainer := prog.GlobalVariablesBlueprint.Container()
	for i, m := range globalVarsContainer.GetAllMember() {
		variable := b.CreateVariableCross(i.String())
		b.AssignVariable(variable, m)
	}
}

func (p *Program) GetGlobalVariable(name string) (Value, bool) {
	if p.GlobalVariablesBlueprint == nil {
		return nil, false
	}

	p.GlobalVariablesBlueprint.Build()
	globalVarsContainer := p.GlobalVariablesBlueprint.Container()
	if globalVarsContainer == nil {
		return nil, false
	}

	return globalVarsContainer.GetStringMember(name)
}
