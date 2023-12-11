package ssa

import "fmt"

// TODO: implement Variable in scope
type Variable struct {
	Name       string
	Range      *Position
	RightRange []*Position
	Value      *Value
}

type Scope struct {
	//TODO: save Variable not Value
	Id          int // scope id in a function
	VarMap      map[string]Value
	Var         []Value           // sort by Position
	SymbolTable map[string]string // variable -> variable-ID(variable-scopeID)
	Range       *Position
	Function    *Function
	Parent      *Scope
	Children    []*Scope
}

func NewScope(id int, Range *Position, Func *Function) *Scope {
	return &Scope{
		Id:          id,
		VarMap:      make(map[string]Value),
		Var:         make([]Value, 0),
		SymbolTable: make(map[string]string),
		Range:       Range,
		Function:    Func,
		Parent:      nil,
		Children:    make([]*Scope, 0),
	}
}

func (s *Scope) AddChild(child *Scope) {
	s.Children = append(s.Children, child)
	child.Parent = s
}

func (s *Scope) SetLocalVariable(text string) string {
	newText := fmt.Sprintf("%s-%d", text, s.Id)
	s.SymbolTable[text] = newText
	fmt.Printf("scope(%d) add local-variable: %s\n", s.Id, newText)
	return newText
}

func (s *Scope) GetLocalVariable(text string) string {
	ret, ok := s.SymbolTable[text]
	if !ok {
		if s.Parent != nil {
			ret = s.Parent.GetLocalVariable(text)
			if ret != text {
				s.SymbolTable[text] = ret
			}
		} else {
			ret = text
		}
	}
	return ret
}

// block symbol-table stack
func (b *FunctionBuilder) ScopeStart() {
	newScope := NewScope(b.NewScopeId(), b.CurrentPos, b.Function)
	b.CurrentScope.AddChild(newScope)
	b.CurrentScope = newScope
}

func (b *FunctionBuilder) NewScopeId() int {
	b.scopeId++
	return b.scopeId
}

func (b *FunctionBuilder) ScopeEnd() {
	b.CurrentScope = b.CurrentScope.Parent
}

func (b *FunctionBuilder) SetScopeLocalVariable(text string) string {
	return b.CurrentScope.SetLocalVariable(text)
}

func (b *FunctionBuilder) GetScopeLocalVariable(id string) string {
	return b.CurrentScope.GetLocalVariable(id)
}
