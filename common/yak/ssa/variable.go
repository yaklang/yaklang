package ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
)

type Variable struct {
	*ssautil.Versioned[Value]
	DefRange memedit.RangeIf
	UseRange map[memedit.RangeIf]struct{}

	// for object.member variable  access
	object      Value
	key         Value
	verboseName string
}

var _ ssautil.VersionedIF[Value] = (*Variable)(nil)

func NewVariable(globalIndex int, name string, local bool, scope ssautil.ScopedVersionedTableIF[Value]) ssautil.VersionedIF[Value] {
	ret := &Variable{
		Versioned: ssautil.NewVersioned[Value](globalIndex, name, local, scope).(*ssautil.Versioned[Value]),
		DefRange:  nil,
		UseRange:  map[memedit.RangeIf]struct{}{},
	}
	return ret
}

func (variable *Variable) Replace(val, to Value) {
	if variable.IsNil() {
		return
	}
	prog := variable.GetProgram()
	if prog != nil {
		prog.ForceSetOffsetValue(to, variable.DefRange)
		for r := range variable.UseRange {
			prog.ForceSetOffsetValue(to, r)
		}
	}

	variable.Versioned.Replace(val, to)
}

func (variable *Variable) Assign(value Value) error {
	if utils.IsNil(value) {
		return utils.Error("assign empty")
	}

	// set offset value for assign variable
	prog := value.GetProgram()
	if prog != nil {
		prog.SetOffsetValue(value, value.GetRange())
	}

	value.AddVariable(variable)
	if variable.IsMemberCall() {
		// setMemberVerboseName(value)
		value.SetVerboseName(getMemberVerboseName(variable.object, variable.key))
		obj, key := variable.GetMemberCall()
		setMemberCallRelationship(obj, key, value)
		if objTyp, ok := ToObjectType(obj.GetType()); ok {
			objTyp.AddField(key, value.GetType())
		}
	}
	return variable.Versioned.Assign(value)
}

func (v *Variable) SetMemberCall(obj, key Value) {
	v.object = obj
	v.key = key
}

func (b *Variable) IsMemberCall() bool {
	return b.object != nil
}

func (b *Variable) GetProgram() *Program {
	value := b.GetValue()
	if utils.IsNil(value) {
		return nil
	}
	return value.GetProgram()
}

func (b *Variable) GetMemberCall() (Value, Value) {
	return b.object, b.key
}

func (v *Variable) SetDefRange(r memedit.RangeIf) {
	if r == nil {
		log.Error("SetDefRange: range is nil use fallback")
		return
	}
	v.DefRange = r
	v.verboseName = r.GetText()
}

func (v *Variable) AddRange(r memedit.RangeIf, force bool) {
	if r == nil {
		log.Error("AddRange: range is nil")
	}
	//if force || len(*p.SourceCode) == len(v.GetName()) {
	//	v.UseRange[p] = struct{}{}
	//}
	value := v.GetValue()
	// phi not def range, so not have verboseName
	isPhi := !utils.IsNil(value) && value.GetOpcode() == SSAOpcodePhi

	if force || isPhi || r.GetText() == v.verboseName {
		v.UseRange[r] = struct{}{}
	}
}

func (v *Variable) NewError(kind ErrorKind, tag ErrorTag, msg string) {
	value := v.GetValue()
	value.GetFunc().NewErrorWithPos(kind, tag, v.DefRange, msg)
	for rangePos := range v.UseRange {
		value.GetFunc().NewErrorWithPos(kind, tag, rangePos, msg)
	}
}

func ReadVariableFromScope(scope ScopeIF, name string) *Variable {
	if ret := scope.ReadVariable(name); ret != nil {
		if variable, ok := ret.(*Variable); ok {
			return variable
		}
	}
	return nil
}

func ReadVariableFromCurrentScope(scope ScopeIF, name string) *Variable {
	if ret := scope.ReadVariableFromCurrentScope(name); ret != nil {
		if variable, ok := ret.(*Variable); ok {
			return variable
		}
	}
	return nil
}

// 查找当前以及父类scope中的第一个local Variable
func GetLocalVariableFromScope(scope ScopeIF, name string) *Variable {
	if variables := scope.GetVariables(name); variables != nil {
		for _, variable := range variables {
			if variable.GetLocal() {
				if ret, ok := variable.(*Variable); ok {
					return ret
				}
			}
		}
	}
	return nil
}

// 查找当前以及父类scope中的第一个Variable
func GetVariableFromScope(scope ScopeIF, name string) *Variable {
	if variables := scope.GetVariables(name); variables != nil {
		for _, variable := range variables {
			if ret, ok := variable.(*Variable); ok {
				return ret
			}
		}
	}
	return nil
}

// 查找当前以及父类scope中的所有Variable
func GetVariablesFromScope(scope ScopeIF, name string) []*Variable {
	var rets []*Variable
	if variables := scope.GetVariables(name); variables != nil {
		for _, variable := range variables {
			if ret, ok := variable.(*Variable); ok {
				rets = append(rets, ret)
			}
		}
	}
	return rets
}
