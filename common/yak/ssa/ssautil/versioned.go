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

	// Assign assign a value to the variable
	Assign(T) error
	String() string

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

	// the version of variable in current scope
	scope *ScopedVersionedTable[T]

	isPhi bool

	isAssigned *utils.AtomicBool
	Value      T
}

var _ VersionedIF[string] = (*Versioned[string])(nil)

func NewVersioned[T comparable](versionIndex, globalIndex int, name string, scope *ScopedVersionedTable[T]) VersionedIF[T] {
	ret := &Versioned[T]{
		versionIndex: versionIndex,
		globalIndex:  globalIndex,
		lexicalName:  name,
		scope:        scope,
		isPhi:        false,
		isAssigned:   utils.NewAtomicBool(),
		Value:        *new(T),
	}
	ret.captureVariable = ret
	return ret
}

func (v *Versioned[T]) IsNil() bool {
	var zero T
	return v.Value == zero
}

func (v *Versioned[T]) GetValue() T {
	return v.Value
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
	return fmt.Sprintf("#%d %s_%d", v.globalIndex, v.lexicalName, v.versionIndex)
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
	var zero VersionedIF[T]
	return v.captureVariable == zero
}
