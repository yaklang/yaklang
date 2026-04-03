package ssa

import "github.com/yaklang/yaklang/common/utils"

func memberKeyNameForGlobal(key Value) string {
	if lit, ok := key.(*ConstInst); ok && lit.Const != nil {
		return lit.Const.str
	}
	return key.String()
}

func initContainer(fb *FunctionBuilder) {
	container := fb.EmitEmptyContainer()

	prog := fb.GetProgram()
	if !utils.IsNil(prog.GlobalVariablesBlueprint) {
		prog.GlobalVariablesBlueprint.InitializeWithContainer(container)
	}
}

func (b *FunctionBuilder) AddGlobalVariable(name string, valueFunc func() Value) {
	prog := b.GetProgram()

	if utils.IsNil(prog.GlobalVariablesBlueprint) {
		initContainer(b)
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

func (b *FunctionBuilder) TryUpdateGlobalVariable(l *Variable, r Value) bool {
	name := l.GetName()
	return b.TryUpdateGlobalVariableByName(name, r)
}

func (b *FunctionBuilder) TryUpdateGlobalVariableByName(name string, r Value) bool {
	prog := b.GetProgram()

	if utils.IsNil(prog.GlobalVariablesBlueprint) {
		initContainer(b)
	}

	globalVarsContainer := prog.GlobalVariablesBlueprint.Container()
	if globalVarsContainer == nil {
		return false
	}

	// Only update when the global already exists in the container
	if _, ok := globalVarsContainer.GetStringMember(name); !ok {
		return false
	}

	globalVarsContainer.SetStringMember(name, r)
	return true
}

func (b *FunctionBuilder) GetGlobalVariables() map[string]Value {
	variables := make(map[string]Value)
	prog := b.GetProgram()

	if utils.IsNil(prog.GlobalVariablesBlueprint) {
		initContainer(b)
	}

	globalVarsContainer := prog.GlobalVariablesBlueprint.Container()
	if globalVarsContainer == nil {
		return variables
	}

	globalVarsContainer.ForEachMember(func(i, m Value) bool {
		variables[memberKeyNameForGlobal(i)] = m
		return true
	})
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

	if utils.IsNil(prog.GlobalVariablesBlueprint) {
		log.Errorf("global variables blueprint is nil")
		return
	}

	if utils.IsNil(prog.GlobalVariablesBlueprint.Container()) {
		initContainer(b)
	}

	globalVarsContainer := prog.GlobalVariablesBlueprint.Container()
	scope := b.CurrentBlock.ScopeTable
	globalVarsContainer.ForEachMember(func(i, m Value) bool {
		variable := b.CreateVariableCross(memberKeyNameForGlobal(i))
		if variable == nil {
			return true
		}
		if current := variable.GetValue(); !utils.IsNil(current) && current.GetId() == m.GetId() {
			return true
		}
		scope.AssignVariable(variable, m)
		return true
	})
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
