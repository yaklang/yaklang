package ssautil

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type Versioned[T any] struct {
	// origin desc the variable's last or renamed version
	origin       *Versioned[T]
	versionIndex int
	globalIndex  int
	lexicalName  string

	// the version of variable in current scope
	scope *ScopedVersionedTable[T]

	isPhi bool

	isAssigned *utils.AtomicBool
	Value      T
}

func (v *Versioned[T]) SetPhi(b bool) {
	v.isPhi = b
}

func (v *Versioned[T]) IsPhi() bool {
	return v.isPhi
}

func (v *Versioned[T]) String() string {
	if v.lexicalName == "" {
		return fmt.Sprintf("symbolic #%d", v.globalIndex)
	}

	if v.IsPhi() {
		return fmt.Sprintf("#%d(phi) %s_%d", v.globalIndex, v.lexicalName, v.versionIndex)
	}
	return fmt.Sprintf("#%d %s_%d", v.globalIndex, v.lexicalName, v.versionIndex)
}

func (v *Versioned[T]) IsRoot() bool {
	return v.origin == nil
}

func (v *Versioned[T]) GetRootVersion() *Versioned[T] {
	if v.IsRoot() {
		return v
	}
	return v.origin.GetRootVersion()
}

func (v *Versioned[T]) GetId() int {
	return v.globalIndex
}

func (v *Versioned[T]) Assign(val T) error {
	if v.isAssigned.IsSet() {
		log.Warnf("ssa: #%v have been assigned by %v", v.globalIndex, v.Value)
		return utils.Error("ssautil.VersionedVar should be assigned once")
	}

	if isZeroValue(val) {
		log.Warnf("ssa: #%v is trying to be assigned by nil", v.GetId())
	}
	v.isAssigned.Set()
	v.Value = val
	return nil
}
