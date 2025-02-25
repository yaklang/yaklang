package ssautil

type VariableMemory[T versionedValue] struct {
	value      T
	variable   VersionedIF[T]
	prev, next *VariableMemory[T]
	isphi      bool
	edges      []*VariableMemory[T]
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

func (v *VariableMemory[T]) IsPhiMemery() bool {
	return v.isphi
}

func (v *VariableMemory[T]) SetPhiMemery(isphi bool) {
	v.isphi = isphi
}

func (v *VariableMemory[T]) GetEdges() []*VariableMemory[T] {
	return v.edges
}

func (v *VariableMemory[T]) AddEdge(edge *VariableMemory[T]) {
	v.edges = append(v.edges, edge)
}

func (v *VariableMemory[T]) SetEdge(edges []*VariableMemory[T]) {
	v.edges = edges
}
