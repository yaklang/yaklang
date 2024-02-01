package ssa

import (
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
)

type Variable struct {
	*ssautil.Versioned[Value]
	DefRange *Range
	UseRange map[*Range]struct{}
}

var _ ssautil.VersionedIF[Value] = (*Variable)(nil)

func NewVariable(globalIndex int, name string, local bool, scope *ssautil.ScopedVersionedTable[Value]) ssautil.VersionedIF[Value] {
	ret := &Variable{
		Versioned: ssautil.NewVersioned[Value](globalIndex, name, local, scope).(*ssautil.Versioned[Value]),
		DefRange:  nil,
		UseRange:  map[*Range]struct{}{},
	}
	return ret
}

func (v *Variable) SetDefRange(r *Range) {
	v.DefRange = r
}

func (v *Variable) Assign(value Value) error {
	value.GetProgram().SetInstructionWithName(v.GetName(), value)
	v.Versioned.Assign(value)
	value.AddVariable(v)
	return nil
}

func (v *Variable) AddRange(p *Range, force bool) {
	if force || len(*p.SourceCode) == len(v.GetName()) {
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

func ReadVariableFromScope(scope *ssautil.ScopedVersionedTable[Value], name string) *Variable {
	if ret := scope.ReadVariable(name); ret != nil {
		if variable, ok := ret.(*Variable); ok {
			return variable
		}
	}
	return nil
}
