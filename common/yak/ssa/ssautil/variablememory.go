package ssautil

type VariableKind int

const (
	NormalVariable VariableKind = iota
	AddressVariable
	DereferenceVariable
)

type VariableMemory[T versionedValue] struct {
	Value     T
	variables map[string]VersionedIF[T]
	kind      VariableKind
}

func (v *VariableMemory[T]) GetKind() VariableKind {
	return v.kind
}

func (v *VariableMemory[T]) SetKind(kind VariableKind) {
	v.kind = kind
}

func (v *VariableMemory[T]) SetVariable(variable VersionedIF[T]) {
	v.variables[variable.GetName()] = variable
}

func (v *VariableMemory[T]) GetVariableByName(name string) (VersionedIF[T], bool) {
	s, ok := v.variables[name]
	return s, ok
}

func (v *VariableMemory[T]) GetVariables() map[string]VersionedIF[T] {
	return v.variables
}

func (v *VariableMemory[T]) GetValue() T {
	return v.Value
}
