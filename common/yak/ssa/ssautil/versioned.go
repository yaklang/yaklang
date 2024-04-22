package ssautil

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type versionedValue interface {
	comparable
	SSAValue
}

type SSAValue interface {
	IsUndefined() bool
	SelfDelete()
	GetId() int64
}

// VersionedIF is an interface for versioned variable, scope will use this interface
type VersionedIF[T versionedValue] interface {
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
	CaptureInScope(ScopedVersionedTableIF[T]) (VersionedIF[T], bool)
	CanCaptureInScope(ScopedVersionedTableIF[T]) bool
	// capture variable
	SetCaptured(VersionedIF[T]) // this capture will set self, when variable create.
	GetCaptured() VersionedIF[T]

	// scope
	GetScope() ScopedVersionedTableIF[T]

	// version and root
	GetGlobalIndex() int // global id
	GetRootVersion() VersionedIF[T]
	IsRoot() bool
	GetId() int64
	MarshalJSON() ([]byte, error)
	UnmarshalJSON([]byte) error
}

type Versioned[T versionedValue] struct {
	// origin desc the variable's last or renamed version
	captureVariable VersionedIF[T]
	versionIndex    int
	globalIndex     int
	lexicalName     string

	local bool

	// the version of variable in current scope
	scope ScopedVersionedTableIF[T]

	isAssigned *utils.AtomicBool
	Value      T
}

func (v *Versioned[T]) GetId() int64 {
	if isZeroValue(v.Value) {
		return 0
	}
	return v.Value.GetId()
}

func (v *Versioned[T]) UnmarshalJSON(raw []byte) error {
	if v == nil {
		return nil
	}
	params := make(map[string]any)
	err := json.Unmarshal(raw, &params)
	if err != nil {
		return err
	}
	capId := v.versionIndex
	_ = capId

	v.versionIndex = utils.MapGetInt(params, "version_index")
	v.globalIndex = utils.MapGetInt(params, "global_index")
	v.lexicalName = utils.MapGetString(params, "lexical_name")
	v.local = utils.MapGetBool(params, "local")
	v.isAssigned = utils.NewAtomicBool()
	v.isAssigned.SetTo(utils.MapGetBool(params, "is_assigned"))

	valIdx := utils.MapGetInt(params, "value")
	// lazy value for ssa.Value
	_ = valIdx

	// lazy scope, scope 可能是不需要的，
	// 因为一般在反序列化这个结果的过程中，
	// 都已经知道是谁的 Scope 了，
	//外部赋值即可满足需求

	return nil
}

func (v *Versioned[T]) MarshalJSON() ([]byte, error) {
	params := make(map[string]any)
	var capId int64 = 0
	if v.captureVariable != nil {
		capId = v.captureVariable.GetId()
	}
	params["capture_variable"] = capId
	params["version_index"] = v.versionIndex
	params["global_index"] = v.globalIndex
	params["lexical_name"] = v.lexicalName
	params["local"] = v.local
	params["scope"] = v.scope.GetPersistentId()
	params["is_assigned"] = v.isAssigned.IsSet()
	var valIdx int64 = 0
	if isZeroValue(v.Value) {
		valIdx = v.Value.GetId()
	}
	params["value"] = valIdx
	return json.Marshal(params)
}

func NewVersioned[T versionedValue](globalIndex int, name string, local bool, scope ScopedVersionedTableIF[T]) VersionedIF[T] {
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
	if !val.IsUndefined() {
		v.isAssigned.Set()
		rVal := reflect.ValueOf(v.Value)
		if rVal.IsValid() && !rVal.IsZero() && v.Value.IsUndefined() {
			v.Value.SelfDelete()
		}
	}
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

func (v *Versioned[T]) CaptureInScope(base ScopedVersionedTableIF[T]) (VersionedIF[T], bool) {
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

func (v *Versioned[T]) CanCaptureInScope(base ScopedVersionedTableIF[T]) bool {
	_, ok := v.CaptureInScope(base)
	return ok
}

func (v *Versioned[T]) SetCaptured(capture VersionedIF[T]) {
	v.captureVariable = capture.GetCaptured()
}

func (v *Versioned[T]) GetCaptured() VersionedIF[T] {
	return v.captureVariable
}

func (v *Versioned[T]) GetScope() ScopedVersionedTableIF[T] {
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
