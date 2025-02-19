package ssautil

type VariableMemory[T versionedValue] struct {
	value      T
	variable   VersionedIF[T]
	prev, next *VariableMemory[T]
}

func (v *VariableMemory[T]) SetVariable(variable VersionedIF[T]) {
	v.variable = variable
}

func (v *VariableMemory[T]) GetVariable() VersionedIF[T] {
	return v.variable
}

func (v *VariableMemory[T]) SetValue(value T) {
	v.value = value
}

func (v *VariableMemory[T]) GetValue() T {
	return v.value
}

func (v *VariableMemory[T]) InsertNext(node *VariableMemory[T]) {
	v.next = node
	node.prev = v
}

func (v *VariableMemory[T]) GetNext() *VariableMemory[T] {
	return v.next
}

func (v *VariableMemory[T]) GetPrev() *VariableMemory[T] {
	return v.prev
}
