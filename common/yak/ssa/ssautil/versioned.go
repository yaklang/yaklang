package ssautil

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// VersionedIF is an interface for versioned variable, scope will use this interface
type VersionedIF[T comparable] interface {
	// value
	// IsNil return true if the variable is nil
	IsNil() bool
	GetValue() T

	// Replace
	Replace(T, T)
	// Assign assign a value to the variable
	Assign(T) error

	// show, string and name
	String() string
	GetName() string

	// version
	SetVersion(int)
	GetVersion() int

	// local
	GetLocal() bool

	// capture
	CaptureInScope(*ScopedVersionedTable[T]) (VersionedIF[T], bool)
	CanCaptureInScope(*ScopedVersionedTable[T]) bool
	// capture variable
	SetCaptured(VersionedIF[T]) // this capture will set self, when variable create.
	GetCaptured() VersionedIF[T]

	// scope
	GetScope() *ScopedVersionedTable[T]

	// version and root
	GetGlobalIndex() int // global id
	GetRootVersion() VersionedIF[T]
	IsRoot() bool
}

type Versioned[T comparable] struct {
	// origin desc the variable's last or renamed version
	captureVariable VersionedIF[T]
	versionIndex    int
	globalIndex     int
	lexicalName     string

	local bool

	// the version of variable in current scope
	scope *ScopedVersionedTable[T]

	isAssigned *utils.AtomicBool
	Value      T
}

var _ VersionedIF[string] = (*Versioned[string])(nil)

func NewVersioned[T comparable](globalIndex int, name string, local bool, scope *ScopedVersionedTable[T]) VersionedIF[T] {
	ret := &Versioned[T]{
		captureVariable: nil,
		versionIndex:    -1,
		globalIndex:     globalIndex,
		lexicalName:     name,
		local:           local,
		scope:           scope,
		isAssigned:      utils.NewAtomicBool(),
	}
	ret.captureVariable = ret
	return ret
}

func (v *Versioned[T]) IsNil() bool {
	var zero T
	return v.Value == zero
}

func (v *Versioned[T]) GetValue() (ret T) {
	return v.Value
}

func (v *Versioned[T]) Replace(val, to T) {
	if v.Value == val {
		v.Value = to
	}
}

func (v *Versioned[T]) Assign(val T) error {
	if v.isAssigned.IsSet() {
		log.Warnf("ssa: #%v have been assigned by %v", v.globalIndex, v.Value)
		return utils.Error("ssautil.VersionedVar should be assigned once")
	}

	if isZeroValue(val) {
		log.Warnf("ssa: #%v is trying to be assigned by nil", v.GetGlobalIndex())
	}
	v.isAssigned.Set()
	v.Value = val
	return nil
}

func (v *Versioned[T]) String() string {
	if v.lexicalName == "" {
		return fmt.Sprintf("symbolic #%d", v.globalIndex)
	}
	ret := fmt.Sprintf("#%d %s", v.globalIndex, v.lexicalName)
	if v.versionIndex > 0 {
		ret += fmt.Sprintf("_%d", v.versionIndex)
	}
	return ret
}

func (v *Versioned[T]) GetName() string {
	return v.lexicalName
}
func (v *Versioned[T]) SetVersion(version int) {
	v.versionIndex = version
}

func (v *Versioned[T]) GetVersion() int {
	return v.versionIndex
}

func (v *Versioned[T]) GetLocal() bool {
	return v.local
}

func (v *Versioned[T]) CaptureInScope(base *ScopedVersionedTable[T]) (VersionedIF[T], bool) {
	baseVariable := base.ReadVariable(v.GetName())
	if baseVariable == nil {
		// not exist in base scope, this variable just set in sub-scope,
		// just skip
		return nil, false
	}
	if baseVariable.GetCaptured() != v.GetCaptured() {
		return nil, false
	}

	return baseVariable, true
}

func (v *Versioned[T]) CanCaptureInScope(base *ScopedVersionedTable[T]) bool {
	_, ok := v.CaptureInScope(base)
	return ok
}

func (v *Versioned[T]) SetCaptured(capture VersionedIF[T]) {
	v.captureVariable = capture.GetCaptured()
}

func (v *Versioned[T]) GetCaptured() VersionedIF[T] {
	return v.captureVariable
}

func (v *Versioned[T]) GetScope() *ScopedVersionedTable[T] {
	return v.scope
}

func (v *Versioned[T]) GetGlobalIndex() int {
	return v.globalIndex
}

func (v *Versioned[T]) GetRootVersion() VersionedIF[T] {
	if v.IsRoot() {
		return v
	}
	return v.captureVariable
}
func (v *Versioned[T]) IsRoot() bool {
	return v.captureVariable == v
}
