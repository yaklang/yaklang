package core

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"golang.org/x/exp/maps"
)

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
	NewVar(val values.JavaValue) *values.JavaRef
	GetVarId() *utils.VariableId
	SetVarId(*utils.VariableId)
	AssignVar(slot int, val values.JavaValue) (*values.JavaRef, bool)
	GetVar(slot int) *values.JavaRef
}

var _ StackSimulation = &StackSimulationImpl{}
var _ StackSimulation = &StackSimulationProxy{}

type StackSimulationImpl struct {
	stackEntry   *StackItem
	varTable     map[int]*values.JavaRef
	currentVarId *utils.VariableId
}

func (s *StackSimulationImpl) GetVarId() *utils.VariableId {
	return s.currentVarId
}

func (s *StackSimulationImpl) SetVarId(id *utils.VariableId) {
	s.currentVarId = id
}

func (s *StackSimulationImpl) GetVar(slot int) *values.JavaRef {
	return s.varTable[slot]
}

func (s *StackSimulationImpl) NewVar(val values.JavaValue) *values.JavaRef {
	defer func() {
		s.currentVarId = s.currentVarId.Next()
	}()
	newRef := values.NewJavaRef(s.currentVarId, val)
	//d.idToValue[d.currentVarId] = val
	return newRef
}

func (s *StackSimulationImpl) AssignVar(slot int, val values.JavaValue) (*values.JavaRef, bool) {
	typ := val.Type()
	ref, ok := s.varTable[slot]
	if !ok || ref.Type().String(&class_context.ClassContext{}) != typ.String(&class_context.ClassContext{}) {
		newRef := s.NewVar(val)
		s.varTable[slot] = newRef
		return newRef, true
	}
	newRef := *ref
	newRef.Id = newRef.Id.Horizontal()
	return &newRef, false
}

func NewEmptyStackEntry() *StackItem {
	return newStackItem(nil, nil)
}
func NewStackSimulation(entry *StackItem, varTable map[int]*values.JavaRef, generator *utils.VariableId) *StackSimulationImpl {
	sim := &StackSimulationImpl{
		stackEntry:   entry,
		varTable:     map[int]*values.JavaRef{},
		currentVarId: generator,
	}
	maps.Copy(sim.varTable, varTable)
	return sim
}

func (s *StackSimulationImpl) Size() int {
	size := 0
	for entry := s.stackEntry; entry.parent != nil; entry = entry.GetParent() {
		size++
	}
	return size
}
func (s *StackSimulationImpl) Push(value values.JavaValue) {
	s.stackEntry = newStackItem(s.stackEntry, value)
}

func (s *StackSimulationImpl) Peek() values.JavaValue {
	if s.stackEntry.parent == nil {
		panic("Stack is empty")
	}
	return s.stackEntry.value
}
func (s *StackSimulationImpl) Pop() values.JavaValue {
	if s.stackEntry.parent == nil {
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
