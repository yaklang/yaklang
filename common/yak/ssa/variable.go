package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
)

type Variable struct {
	*ssautil.Versioned[Value]
	DefRange *Range
	UseRange map[*Range]struct{}

	// for object.member variable  access
	object Value
	key    Value
}

var _ ssautil.VersionedIF[Value] = (*Variable)(nil)

func NewVariable(globalIndex int, name string, local bool, scope ssautil.ScopedVersionedTableIF[Value]) ssautil.VersionedIF[Value] {
	ret := &Variable{
		Versioned: ssautil.NewVersioned[Value](globalIndex, name, local, scope).(*ssautil.Versioned[Value]),
		DefRange:  nil,
		UseRange:  map[*Range]struct{}{},
	}
	return ret
}

func (variable *Variable) Assign(value Value) error {
	if utils.IsNil(value) {
		return utils.Error("assign empty")
	}
	value.AddVariable(variable)
	if variable.IsMemberCall() {
		// setMemberVerboseName(value)
		value.SetVerboseName(getMemberVerboseName(variable.object, variable.key))
		obj, key := variable.GetMemberCall()
		SetMemberCall(obj, key, value)
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

func (b *Variable) GetMemberCall() (Value, Value) {
	return b.object, b.key
}

func (v *Variable) SetDefRange(r *Range) {
	v.DefRange = r
}

func (v *Variable) AddRange(p *Range, force bool) {
	//if force || len(*p.SourceCode) == len(v.GetName()) {
	//	v.UseRange[p] = struct{}{}
	//}
	if force || len(p.GetText()) == len(v.GetName()) {
		v.UseRange[p] = struct{}{}
	}
}

func (v *Variable) NewError(kind ErrorKind, tag ErrorTag, msg string) {
	value := v.GetValue()
	value.GetFunc().NewErrorWithPos(kind, tag, v.DefRange, msg)
	for rangePos := range v.UseRange {
		value.GetFunc().NewErrorWithPos(kind, tag, rangePos, msg)
	}
}

func ReadVariableFromScope(scope *Scope, name string) *Variable {
	if ret := scope.ReadVariable(name); ret != nil {
		if variable, ok := ret.(*Variable); ok {
			return variable
		}
	}
	return nil
}
