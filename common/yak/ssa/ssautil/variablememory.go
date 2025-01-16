package ssautil

type VariableKind int

const (
	NormalVariable VariableKind = iota
	AddressVariable
	DereferenceVariable
)

type VariableMemory[T versionedValue] struct {
	Value T
	self  VersionedIF[T]
	kind  VariableKind
}

func (v *VariableMemory[T]) GetKind() VariableKind {
	return v.kind
}

func (v *VariableMemory[T]) SetKind(kind VariableKind) {
	v.kind = kind
}

func (v *VariableMemory[T]) GetSelf() VersionedIF[T] {
	return v.self
}

func (v *VariableMemory[T]) GetValue() T {
	return v.Value
}
