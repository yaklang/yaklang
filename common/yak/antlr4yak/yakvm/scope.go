package yakvm

import "sync"

type Scope struct {
	verbose string
	mu      sync.Mutex
	symtbl  *SymbolTable

	parent    *Scope
	idToValue map[int]*Value
}

// NewScope Create Root Scope
// NewVirtualMachine Called Generally
func NewScope(table *SymbolTable) *Scope {
	return &Scope{
		verbose:   table.Verbose,
		symtbl:    table,
		parent:    nil,
		idToValue: make(map[int]*Value),
	}
}
func (s *Scope) SetVerbose(v string) {
	s.verbose = v
}

func (s *Scope) GetSymTable() *SymbolTable {
	return s.symtbl
}

func (s *Scope) SetSymTable(symtbl *SymbolTable) {
	s.symtbl = symtbl
}

func (s *Scope) CreateSubScope(table *SymbolTable) *Scope {
	return &Scope{
		verbose:   table.Verbose,
		symtbl:    table,
		parent:    s,
		idToValue: make(map[int]*Value),
	}
}

func (s *Scope) IsRoot() bool {
	return s.parent == nil
}

func (s *Scope) Len() int {
	return len(s.idToValue)
}

func (s *Scope) GetValueByName(name string) (*Value, bool) {
	if s == nil {
		return nil, false
	}
	id, ok := s.symtbl.GetSymbolByVariableName(name)
	if !ok {
		return nil, false
	}
	val, ok := s.GetValueByID(id)
	if val == nil {
		val = undefined
	}
	return val, ok
}

func (s *Scope) GetNameById(id int) string {
	tbl := s.GetSymTable()
	if tbl == nil {
		return ""
	}
	raw, _ := tbl.GetNameByVariableId(id)
	return raw
}

func (s *Scope) GetValueByID(id int) (*Value, bool) {
	if s == nil {
		return nil, false
	}

	s.mu.Lock()
	raw, ok := s.idToValue[id]
	s.mu.Unlock()
	if ok {
		return raw, true
	}

	return s.parent.GetValueByID(id)
}

func (s *Scope) NewValueByID(id int, v *Value) {
	s.mu.Lock()
	s.idToValue[id] = v
	s.mu.Unlock()
}

func (s *Scope) InCurrentScope(id int) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	_, ok := s.idToValue[id]
	s.mu.Unlock()
	return ok
}

func (s *Scope) GetAllIdInScopes() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []int
	for i := range s.idToValue {
		ids = append(ids, i)
	}
	return ids
}

func (s *Scope) GetAllNameAndValueInScopes() (results map[string]*Value) {
	results = make(map[string]*Value)
	for id, value := range s.idToValue {
		name := s.GetNameById(id)
		if name == "" || name == "_" {
			continue
		}
		results[name] = value
	}
	return
}

func (s *Scope) GetAllNameAndValueInAllScopes() (results map[string]*Value) {
	results = make(map[string]*Value)
	scope := s
	for scope != nil {
		for id, value := range scope.idToValue {
			name := scope.GetNameById(id)
			if name == "" || name == "_" {
				continue
			}
			if _, ok := results[name]; !ok {
				results[name] = value
			}
		}
		scope = scope.parent
	}

	return
}
