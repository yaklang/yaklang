package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type Variable struct {
	Name  string
	Range map[*Range]struct{}
	V     Value
}

func NewVariable(name string, i Value) *Variable {
	ret := &Variable{
		Name:  name,
		Range: make(map[*Range]struct{}),
		V:     i,
	}
	i.AddVariable(ret)
	return ret
}
func (v *Variable) String() string {
	ret := ""
	ret += fmt.Sprintln("Variable ", v.Name, LineDisasm(v.V))
	return ret
}

func (v *Variable) AddRange(p *Range) {
	// v.Range = append(v.Range, p)
	// fmt.Println(v.Name, p.StartColumn)
	v.Range[p] = struct{}{}
}

func (v *Variable) NewError(kind ErrorKind, tag ErrorTag, msg string) {
	for R := range v.Range {
		v.V.GetFunc().NewErrorWithPos(kind, tag, R, msg)
	}
}

type item struct {
	v *Variable
	r *Range
}

type Scope struct {
	Id                 int // scope id in a function
	VarMap             map[string][]*Variable
	Var                []item            // sort by Position
	SymbolTable        map[string]string // variable -> variable-ID(variable-scopeID)
	SymbolTableReverse map[string]string // variable -> variable-ID(variable-scopeID)
	Range              *Range
	Function           *Function
	Parent             *Scope
	Children           []*Scope
}

func NewScope(id int, R *Range, Func *Function) *Scope {
	return &Scope{
		Id:                 id,
		VarMap:             make(map[string][]*Variable),
		Var:                make([]item, 0),
		SymbolTable:        make(map[string]string),
		SymbolTableReverse: make(map[string]string),
		Range:              R,
		Function:           Func,
		Parent:             nil,
		Children:           make([]*Scope, 0),
	}
}

func (s *Scope) AddChild(child *Scope) {
	s.Children = append(s.Children, child)
	child.Parent = s
}

func (s *Scope) InsertByRange(v *Variable, R *Range) {
	i := 0
	for ; i < len(s.Var); i++ {
		if s.Var[i].r.CompareStart(R) > 0 {
			break
		}
	}
	s.Var = utils.InsertSliceItem(s.Var, item{v, R}, i)
}

func (s *Scope) PeekLexicalVariableByName(i string) (*Variable, error) {
	vals, _ := s.VarMap[i]
	if len(vals) > 0 {
		return vals[len(vals)-1], nil
	}
	if s.Parent == nil {
		return nil, fmt.Errorf("can't find variable %s", i)
	}
	return s.Parent.PeekLexicalVariableByName(i)
}

func (s *Scope) AddVariable(v *Variable, R *Range) {
	if R == nil {
		log.Errorf("scope(%d) variable %s range is nil", s.Id, v.Name)
	}
	name, ok := s.SymbolTableReverse[v.Name]
	if !ok {
		name = v.Name
	}
	v.Name = name
	{
		value := v.V
		value.GetProgram().SetInstructionWithName(name, value)
	}
	{
		varList, ok := s.VarMap[name]
		if !ok {
			varList = make([]*Variable, 0, 1)
		}
		varList = append(varList, v)
		s.VarMap[name] = varList
	}
	v.AddRange(R)
	s.InsertByRange(v, R)
}

func (s *Scope) SetLocalVariable(text string) string {
	newText := fmt.Sprintf("%s-%d", text, s.Id)
	s.SymbolTable[text] = newText
	s.SymbolTableReverse[newText] = text
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

func (s *Scope) String() string {
	ret := ""
	ret += fmt.Sprintf("Scope %d\n", s.Id)
	ret += fmt.Sprintf("symbolTable: %#v\n", s.SymbolTable)
	ret += fmt.Sprintln("Variable: ", s.VarMap)
	return ret
}

// block symbol-table stack
func (b *FunctionBuilder) ScopeStart() {
	newScope := NewScope(b.NewScopeId(), b.CurrentRange, b.Function)
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
