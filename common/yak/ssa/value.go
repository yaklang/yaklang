package ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
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

func (b *FunctionBuilder) readValueFromIncludeStack(name string) Value {
	// read value from include stack
	var value Value
	b.includeStack.ForeachStack(func(program *Program) bool {
		mainFunc := program.GetFunctionEx(string(MainFunctionName), "")
		if mainFunc == nil || mainFunc.ExitBlock == 0 {
			return true
		}
		block, ok := b.GetBasicBlockByID(mainFunc.ExitBlock)
		if !ok {
			return true
		}
		if ret := ReadVariableFromScope(block.ScopeTable, name); ret != nil && ret.Value != nil {
			value = ret.Value
			return false
		}
		return true
	})
	//5kb
	if value != nil && len(value.String()) <= 1024*5 {
		return value
	}
	return nil
}
func (b *FunctionBuilder) readValueEx(
	name string,
	create bool, // disable create undefine
	enableClosureFreeValue bool, // disable free-value
) Value {
	scope := b.CurrentBlock.ScopeTable
	program := b.GetProgram()
	local := GetFristLocalVariableFromScopeAndParent(scope, name)

	if ret := ReadVariableFromScopeAndParent(scope, name); ret != nil {
		if local != nil && ret.GetCaptured().GetGlobalIndex() != local.GetGlobalIndex() {
			ret = local
		}
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
	val := b.readValueFromIncludeStack(name)
	if val != nil {
		return val
	}
	isClosure := func() bool {
		if enableClosureFreeValue {
			return true
		}
		_, ok := b.captureFreeValue[name]
		if ok {
			return true
		}
		return false
	}
	enableReadParent := isClosure()
	if enableReadParent {
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

	if enableReadParent && create {
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
		log.Debugf("assign nil value to variable: %v, it will not work on ssa ir format", name)
		return
	}
	scope := b.CurrentBlock.ScopeTable
	if variable.IsPointer() {
		// variable.SetPointHandler(func(valueTmp Value, scopet ssautil.ScopedVersionedTableIF[Value]) {
		// 	tmp := b.CurrentBlock.ScopeTable
		// 	defer func() {
		// 		b.CurrentBlock.ScopeTable = tmp
		// 	}()

		// 	b.CurrentBlock.ScopeTable = scopet
		// 	obj := variable.object

		// 	v := b.CreateMemberCallVariable(obj, b.EmitConstInstPlaceholder("@value"))
		// 	p := b.CreateMemberCallVariable(obj, b.EmitConstInstPlaceholder("@pointer"))
		// 	p.SetKind(ssautil.PointerVariable)
		// 	scopet.AssignVariable(v, value)
		// 	if p.GetValue() == nil {
		// 		scopet.AssignVariable(p, variable.GetValue())
		// 	}

		// 	n := strings.TrimPrefix(variable.GetValue().String(), "&")
		// 	originName, originGlobalId := SplitName(n)
		// 	_ = originGlobalId

		// 	newValue := b.CopyValue(value)
		// 	newValue.SetName(originName)
		// 	newValue.SetVerboseName(originName)

		// 	newVariable := b.CreateVariableById(originName)
		// 	if v := b.CreateVariableGlobalIndex(originName, originGlobalId); v != nil {
		// 		newVariable.SetCaptured(v)
		// 	}
		// 	scopet.AssignVariable(newVariable, newValue)
		// })
		// variable.PointHandler(value, scope)

		obj := variable.object

		v := b.CreateMemberCallVariable(obj, b.EmitConstInstPlaceholder("@value"))
		p := b.CreateMemberCallVariable(obj, b.EmitConstInstPlaceholder("@pointer"))
		p.SetKind(ssautil.PointerVariable)
		scope.AssignVariable(v, value)
		if p.GetValue() == nil {
			scope.AssignVariable(p, variable.GetValue())
		}

		n := strings.TrimPrefix(variable.GetValue().String(), "&")
		originName, originGlobalId := SplitName(n)

		newValue := b.CopyValue(value)
		newValue.SetName(originName)
		newValue.SetVerboseName(originName)

		if newVariable := b.CreateVariableGlobalIndex(originName, originGlobalId); v != nil {
			b.AssignVariable(newVariable, newValue)
			newVariable.SetCross(true)
		}
	} else {
		scope.AssignVariable(variable, value)
	}

	if val, ok := b.RefParameter[variable.GetName()]; ok {
		b.AddForceSideEffect(variable, value, val.Index, val.Kind)
	}
	if value.GetName() == variable.GetName() {
		if value.GetOpcode() == SSAOpcodeFreeValue || value.GetOpcode() == SSAOpcodeParameter {
			return
		}
	}

	if b.isTryBuildValue() && !variable.GetLocal() {
		b.TryBuildValueWithoutParent(variable.GetName(), value)
	}

	if b.TryBuildExternValue(variable.GetName()) != nil {
		b.NewErrorWithPos(Warn, SSATAG, b.CurrentRange, ContAssignExtern(variable.GetName()))
	}

	// if not freeValue, or not `a = a`(just create FreeValue)
	_, exist := b.captureFreeValue[name]
	if !variable.GetLocal() && (exist || b.SupportClosure) {
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
	if _, ok := b.RefParameter[variable.GetName()]; !ok {
		b.CheckMemberSideEffect(variable, value)
	}

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

func (b *FunctionBuilder) CreateVariableCross(name string, pos ...CanStartStopToken) *Variable {
	if variable, ok := b.getCrossScopeVariable(name); ok {
		if value := variable.GetValue(); value != nil {
			return variable
		}
	}
	return b.createVariableEx(name, false, pos...)
}

func (b *FunctionBuilder) CreateVariableGlobalIndex(name string, globalIndex int) *Variable {
	scope := b.CurrentBlock.ScopeTable

	newVariable := b.CreateVariableById(name)
	for _, v := range GetAllVariablesFromScopeAndParent(scope, name) {
		if v.GetGlobalIndex() == globalIndex {
			newVariable.SetCaptured(v)
		}
	}
	return newVariable
}

func (b *FunctionBuilder) CreateVariableById(name string, pos ...CanStartStopToken) *Variable {
	scope := b.CurrentBlock.ScopeTable

	ret := scope.CreateVariable(name, false)
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

func (b *FunctionBuilder) CreateVariable(name string, pos ...CanStartStopToken) *Variable {
	if variable, ok := b.getCurrentScopeVariable(name); ok {
		if value := variable.GetValue(); value != nil {
			if _, ok := ToConstInst(value); ok {
				return variable
			}
			if _, ok := ToMake(value); ok {
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
	if utils.IsNil(scope) {
		return nil
	}
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
			b.FreeValues[variable.(*Variable)] = freeValue.GetId()
			return freeValue
		}
	}

	freeValue := NewParam(name, true, b)
	v := b.CreateVariableHead(name)
	headScope.AssignVariable(v, freeValue)
	b.FreeValues[v] = freeValue.GetId()

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

func (b *FunctionBuilder) getCurrentScopeVariable(name string) (*Variable, bool) {
	scope := b.CurrentBlock.ScopeTable
	if variable := ReadVariableFromScope(scope, name); variable != nil {
		return variable, true
	}
	return nil, false
}

func (b *FunctionBuilder) getCrossScopeVariable(name string) (*Variable, bool) {
	scope := b.CurrentBlock.ScopeTable
	if variable := ReadVariableFromScopeAndParent(scope, name); variable != nil {
		return variable, true
	}
	return nil, false
}
