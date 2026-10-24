package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
)

func memberKeyNameForGlobal(key Value) string {
	if utils.IsNil(key) {
		return ""
	}
	keyName := GetKeyString(key)
	if keyName != "" {
		return keyName
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

	// Use lazy builder to register global variables. The lazy builder
	// defers valueFunc execution until GlobalVariablesBlueprint.Build()
	// is called. Build() is called:
	// 1. In buildFunctionDeclFront before init() block compilation
	// 2. In LoadGlobalVariable before restoring globals to scope
	// 3. In build(ast) after all declarations and functions are processed
	// This ensures valueFunc runs with a valid FunctionBuilder context
	// (before Finish), and the AddLazyBuilder fix (immediate execution
	// if Build() already ran) handles out-of-order cases.
	prog.GlobalVariablesBlueprint.AddLazyBuilder(func() {
		value := valueFunc()
		if utils.IsNil(value) {
			return
		}
		prog.GlobalVariablesBlueprint.RegisterStaticMember(name, value, false)
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

	if prog.GlobalVariablesBlueprint.GetStaticMember(name) == nil {
		return false
	}
	prog.GlobalVariablesBlueprint.RegisterStaticMember(name, r, false)
	return true
}

func (b *FunctionBuilder) GetGlobalVariables() map[string]Value {
	variables := make(map[string]Value)
	prog := b.GetProgram()

	if utils.IsNil(prog.GlobalVariablesBlueprint) {
		initContainer(b)
	}

	for _, name := range prog.GlobalVariablesBlueprint.StaticMember.Keys() {
		if name == "" {
			continue
		}
		if v := prog.GlobalVariablesBlueprint.GetStaticMember(name); v != nil {
			variables[name] = v
		}
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

	if utils.IsNil(prog) || utils.IsNil(prog.GlobalVariablesBlueprint) {
		return
	}

	// Build ensures all AddGlobalVariable lazy builders have executed.
	prog.GlobalVariablesBlueprint.Build()

	// Load registered global variables from StaticMember (typically a few
	// dozen), not all 21000+ memberPairs from globalVarsContainer.
	// The old code traversed GetLastWinsMemberPairs which included all
	// member call relationships (str[0], Store.Get, Fatal(...)), causing
	// 20-30s hangs on engineercms.
	//
	// For each global variable, also restore its member call results
	// (e.g. str[0] -> "alpha") into the current scope, so that cross-file
	// references like str[0] resolve correctly. We only traverse the
	// global variable's own memberPairs (a few per variable), not the
	// globalVarsContainer's 21000+ pairs.
	for _, name := range prog.GlobalVariablesBlueprint.StaticMember.Keys() {
		if name == "" {
			continue
		}
		member := prog.GlobalVariablesBlueprint.GetStaticMember(name)
		if member == nil {
			continue
		}
		variable := b.CreateVariableCross(name)
		if variable == nil {
			continue
		}
		if current := variable.GetValue(); !utils.IsNil(current) && current.GetId() == member.GetId() {
			continue
		}
		b.AssignVariable(variable, member)

		// Member call values stay attached to the global object. ReadMemberCallValue
		// resolves them from that object on demand; copying every member relation
		// into every function scope duplicates values and version chains.

	}
}

func (p *Program) GetGlobalVariable(name string) (Value, bool) {
	if p.GlobalVariablesBlueprint == nil {
		return nil, false
	}

	p.GlobalVariablesBlueprint.Build()

	// Direct O(1) lookup from StaticMember instead of O(N) traversal
	// of globalVarsContainer memberPairs via GetLatestMemberByKeyString.
	v := p.GlobalVariablesBlueprint.GetStaticMember(name)
	if v != nil {
		return v, true
	}
	return nil, false
}
