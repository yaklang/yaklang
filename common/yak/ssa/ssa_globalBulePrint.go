package ssa

import "github.com/yaklang/yaklang/common/utils"

func memberKeyNameForGlobal(key Value) string {
	if lit, ok := ToConstInst(key); ok && lit.Const != nil {
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
			if object := GetLatestObject(v.GetValue()); object != nil && object.GetId() == value.GetId() {
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

	if _, ok := GetLatestMemberByKeyString(globalVarsContainer, name); !ok {
		return false
	}
	setMemberCallRelationship(globalVarsContainer, b.EmitConstInstPlaceholder(name), r)
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

	for _, pair := range GetLastWinsMemberPairs(globalVarsContainer) {
		variables[pair.Key.String()] = pair.Member
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

	if utils.IsNil(prog.GlobalVariablesBlueprint) {
		log.Errorf("global variables blueprint is nil")
		return
	}

	if utils.IsNil(prog.GlobalVariablesBlueprint.Container()) {
		initContainer(b)
	}

	globalVarsContainer := prog.GlobalVariablesBlueprint.Container()
	for _, pair := range GetLastWinsMemberPairs(globalVarsContainer) {
		variable := b.CreateVariableCross(pair.Key.String())
		if variable == nil {
			continue
		}
		if current := variable.GetValue(); !utils.IsNil(current) && current.GetId() == pair.Member.GetId() {
			continue
		}
		b.AssignVariable(variable, pair.Member)
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

	return GetLatestMemberByKeyString(globalVarsContainer, name)
}
