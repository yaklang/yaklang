package ssa

import (
	"sync/atomic"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
)

type Variable struct {
	*ssautil.Versioned[Value]
	DefRange  *memedit.Range
	useRanges atomic.Pointer[variableUseRanges]

	// for object.member variable  access
	object      Value
	key         Value
	verboseName string

	// skipPersistOffsets suppresses offset re-persistence when a variable
	// is restored from DB (GetVariableFromDB → Assign → persistAllOffsets).
	// During restore, all offsets already exist in the DB; re-persisting
	// them only inflates the offsetSaved dedup map and UseRange map
	// (which uses *Range pointers as keys, so each restore adds new entries).
	// This flag is set by GetVariableFromDB and cleared after Assign.
	skipPersistOffsets bool
}

var _ ssautil.VersionedIF[Value] = (*Variable)(nil)

func NewVariable(globalIndex int, name string, local bool, scope ssautil.ScopedVersionedTableIF[Value]) ssautil.VersionedIF[Value] {
	ret := &Variable{
		Versioned: ssautil.NewVersioned[Value](globalIndex, name, local, scope).(*ssautil.Versioned[Value]),
		DefRange:  nil,
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
		variable.ForEachUseRange(func(r *memedit.Range) {
			prog.ForceSetOffsetValue(to, r)
		})
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
	if err := variable.Versioned.Assign(value); err != nil {
		return err
	}
	variable.persistAllOffsets()
	return nil
}

func (v *Variable) SetMemberCall(obj, key Value) {
	if utils.IsNil(v) {
		return
	}
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

func (v *Variable) SetDefRange(r *memedit.Range) {
	if r == nil {
		log.Error("SetDefRange: range is nil use fallback")
		return
	}
	r = v.internRange(r)
	v.DefRange = r
	v.verboseName = r.GetText()
	v.persistOffset(r)
}

func (v *Variable) AddRange(r *memedit.Range, force bool) {
	if utils.IsNil(r) {
		log.Error("AddRange: range is nil")
		return
	}
	//if force || len(*p.SourceCode) == len(v.GetName()) {
	//	v.UseRange[p] = struct{}{}
	//}
	value := v.GetValue()
	// phi not def range, so not have verboseName
	isPhi := !utils.IsNil(value) && value.GetOpcode() == SSAOpcodePhi

	if force || isPhi || r.GetText() == v.verboseName {
		r = v.internRange(r)
		uses := v.useRanges.Load()
		if uses == nil {
			created := &variableUseRanges{}
			if v.useRanges.CompareAndSwap(nil, created) {
				uses = created
			} else {
				uses = v.useRanges.Load()
			}
		}
		if uses.add(r) {
			v.persistOffset(r)
		}
	}
}

func (v *Variable) persistAllOffsets() {
	if v == nil || v.skipPersistOffsets {
		return
	}
	if v.DefRange != nil {
		v.persistOffset(v.DefRange)
	}
	v.ForEachUseRange(func(r *memedit.Range) {
		v.persistOffset(r)
	})
}

func (v *Variable) persistOffset(r *memedit.Range) {
	if v == nil || utils.IsNil(r) || v.skipPersistOffsets {
		return
	}
	prog := v.GetProgram()
	if utils.IsNil(prog) || utils.IsNil(prog.Cache) || utils.IsNil(prog.Cache.indexes) {
		return
	}
	prog.Cache.indexes.SaveVariableOffset(v, r)
}

func (v *Variable) NewError(kind ErrorKind, tag ErrorTag, msg string) {
	if !v.ShouldAddError() {
		return
	}
	value := v.GetValue()
	if utils.IsNil(value) {
		return
	}
	value.GetFunc().NewErrorWithPos(kind, tag, v.DefRange, msg)
	v.ForEachUseRange(func(rangePos *memedit.Range) {
		value.GetFunc().NewErrorWithPos(kind, tag, rangePos, msg)
	})
}

func (v *Variable) ForEachUseRange(fn func(*memedit.Range)) {
	if v == nil || fn == nil {
		return
	}
	if uses := v.useRanges.Load(); uses != nil {
		uses.each(fn)
	}
}

func (v *Variable) IsPointer() bool {
	return v.GetKind() == ssautil.PointerVariable
}

func ReadVariableFromScope(scope ScopeIF, name string) *Variable {
	if utils.IsNil(scope) {
		return nil
	}
	if ret := scope.ReadVariable(name, true); ret != nil {
		if variable, ok := ret.(*Variable); ok {
			return variable
		}
	}
	return nil
}

func ReadVariableFromScopeAndParent(scope ScopeIF, name string) *Variable {
	if utils.IsNil(scope) {
		return nil
	}
	if ret := scope.ReadVariable(name); ret != nil {
		if variable, ok := ret.(*Variable); ok {
			return variable
		}
	}
	return nil
}

func GetFristVariableFromScope(scope ScopeIF, name string) *Variable {
	return findVariableFromScope(scope, name, true, false)
}

func GetFristVariableFromScopeAndParent(scope ScopeIF, name string) *Variable {
	return findVariableFromScope(scope, name, false, false)
}

func GetFristLocalVariableFromScope(scope ScopeIF, name string) *Variable {
	return findVariableFromScope(scope, name, true, true)
}

func GetFristLocalVariableFromScopeAndParent(scope ScopeIF, name string) *Variable {
	return findVariableFromScope(scope, name, false, true)
}

func findVariableFromScope(scope ScopeIF, name string, current, local bool) *Variable {
	if utils.IsNil(scope) {
		return nil
	}
	match := isVariableVersion
	if local {
		match = isLocalVariableVersion
	}
	found := ssautil.FindVariableByName[Value](scope, name, current, match)
	ret, _ := found.(*Variable)
	return ret
}

func isVariableVersion(variable ssautil.VersionedIF[Value]) bool {
	_, ok := variable.(*Variable)
	return ok
}

func isLocalVariableVersion(variable ssautil.VersionedIF[Value]) bool {
	return isVariableVersion(variable) && variable.GetLocal()
}

func GetAllVariablesFromScope(scope ScopeIF, name string) []*Variable {
	var rets []*Variable
	if variables := scope.GetAllVariablesByName(name, true); variables != nil {
		for _, variable := range variables {
			if ret, ok := variable.(*Variable); ok {
				rets = append(rets, ret)
			}
		}
	}
	return rets
}

func GetAllVariablesFromScopeAndParent(scope ScopeIF, name string) []*Variable {
	var rets []*Variable
	if variables := scope.GetAllVariablesByName(name); variables != nil {
		for _, variable := range variables {
			if ret, ok := variable.(*Variable); ok {
				rets = append(rets, ret)
			}
		}
	}
	return rets
}

func (v *Variable) ShouldAddError() bool {
	if utils.IsNil(v) {
		return false
	}
	value := v.GetValue()
	if utils.IsNil(value) {
		return false
	}

	if v.verboseName == "_" {
		return false
	}

	// if is anonymous struct from make
	if make, ok := ToMake(v.object); ok && make.Anonymous {
		return false
	}

	return true
}
