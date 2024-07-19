package ssa

import (
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
)

type ScopeInstance struct {
	*ssautil.ScopedVersionedTable[Value]
}

type ScopeIF ssautil.ScopedVersionedTableIF[Value]

var _ ssautil.ScopedVersionedTableIF[Value] = (*ScopeInstance)(nil)

func NewScope(prog string) *ScopeInstance {
	s := &ScopeInstance{
		ScopedVersionedTable: ssautil.NewRootVersionedTable(prog, NewVariable),
	}
	s.SetThis(s)
	return s
}

func (s *ScopeInstance) CreateSubScope() ssautil.ScopedVersionedTableIF[Value] {
	scope := &ScopeInstance{
		ScopedVersionedTable: s.ScopedVersionedTable.CreateSubScope().(*ssautil.ScopedVersionedTable[Value]),
	}
	scope.SetThis(scope)
	return scope
}
