package core

import "github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"

type StackItem struct {
	parent *StackItem
	value  values.JavaValue
}

func (s *StackItem) GetParent() *StackItem {
	return s.parent
}

func newStackItem(parent *StackItem, value values.JavaValue) *StackItem {
	return &StackItem{
		parent: parent,
		value:  value,
	}
}

type StackSimulationProxy struct {
	*StackSimulationImpl
	push func(values.JavaValue)
	pop  func() values.JavaValue
}

func (s *StackSimulationProxy) Push(value values.JavaValue) {
	s.push(value)
}

func (s *StackSimulationProxy) Pop() values.JavaValue {
	return s.pop()
}
func (s *StackSimulationProxy) PopN(n int) []values.JavaValue {
	vals := make([]values.JavaValue, n)
	for i := 0; i < n; i++ {
		vals[i] = s.Pop()
	}
	return vals
}

func NewStackSimulationProxy(stack *StackSimulationImpl, push func(values.JavaValue), pop func() values.JavaValue) *StackSimulationProxy {
	return &StackSimulationProxy{
		StackSimulationImpl: stack,
		push:                push,
		pop:                 pop,
	}
}

type StackSimulation interface {
	Size() int
	Pop() values.JavaValue
	PopN(n int) []values.JavaValue
	Push(values.JavaValue)
	Peek() values.JavaValue
}
type StackSimulationImpl struct {
	stackEntry *StackItem
}

var startStackEntry = &StackItem{}

func NewStackSimulation(entry *StackItem) *StackSimulationImpl {
	return &StackSimulationImpl{
		stackEntry: entry,
	}
}

func (s *StackSimulationImpl) Size() int {
	size := 0
	for entry := s.stackEntry; entry != startStackEntry; entry = entry.GetParent() {
		size++
	}
	return size
}
func (s *StackSimulationImpl) Push(value values.JavaValue) {
	s.stackEntry = newStackItem(s.stackEntry, value)
}

func (s *StackSimulationImpl) Peek() values.JavaValue {
	if s.stackEntry == startStackEntry {
		panic("Stack is empty")
	}
	return s.stackEntry.value
}
func (s *StackSimulationImpl) Pop() values.JavaValue {
	if s.stackEntry == startStackEntry {
		panic("Stack is empty")
	}
	val := s.stackEntry.value
	s.stackEntry = s.stackEntry.GetParent()
	return val
}
func (s *StackSimulationImpl) PopN(n int) []values.JavaValue {
	vals := make([]values.JavaValue, n)
	for i := 0; i < n; i++ {
		vals[i] = s.Pop()
	}
	return vals
}
