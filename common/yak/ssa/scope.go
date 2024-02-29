package ssa

import "github.com/yaklang/yaklang/common/yak/ssa/ssautil"

type Scope struct {
	*ssautil.ScopedVersionedTable[Value]
}

type ScopeIF ssautil.ScopedVersionedTableIF[Value]

var _ ssautil.ScopedVersionedTableIF[Value] = (*Scope)(nil)

func NewScope() *Scope {
	s := &Scope{
		ScopedVersionedTable: ssautil.NewRootVersionedTable[Value](NewVariable),
	}
	s.SetThis(s)
	return s
}

func (s *Scope) CreateSubScope() ssautil.ScopedVersionedTableIF[Value] {
	scope := &Scope{
		ScopedVersionedTable: s.ScopedVersionedTable.CreateSubScope().(*ssautil.ScopedVersionedTable[Value]),
	}
	scope.SetThis(scope)
	return scope
}
