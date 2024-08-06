package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
)

type ScopeInstance struct {
	*ssautil.ScopedVersionedTable[Value]
	fun *Function
}

type ScopeIF ssautil.ScopedVersionedTableIF[Value]

var _ ssautil.ScopedVersionedTableIF[Value] = (*ScopeInstance)(nil)

func NewScope(f *Function, progname string) *ScopeInstance {
	s := &ScopeInstance{
		ScopedVersionedTable: ssautil.NewRootVersionedTable(progname, NewVariable),
		fun:                  f,
	}
	s.SetName()
	s.SetThis(s)
	return s
}

func (s *ScopeInstance) CreateSubScope() ssautil.ScopedVersionedTableIF[Value] {
	scope := &ScopeInstance{
		ScopedVersionedTable: s.ScopedVersionedTable.CreateSubScope().(*ssautil.ScopedVersionedTable[Value]),
		fun:                  s.fun,
	}
	scope.SetName()
	scope.SetThis(scope)
	return scope
}

func (s *ScopeInstance) CreateShadowScope() ssautil.ScopedVersionedTableIF[Value] {
	scope := &ScopeInstance{
		ScopedVersionedTable: s.ScopedVersionedTable.CreateSubScope().(*ssautil.ScopedVersionedTable[Value]),
		fun:                  s.fun,
	}
	scope.SetName()
	scope.SetThis(scope)
	s.ForEachCapturedVariable(func(s string, vi ssautil.VersionedIF[Value]) {
		scope.SetCapturedVariable(s, vi)
	})
	return scope
}

func (s *ScopeInstance) SetName() {
	if s.fun == nil {
		return
	}
	s.SetScopeName(fmt.Sprintf("fun(%d)-%d", s.fun.GetId(), s.fun.scopeId))
	s.fun.scopeId++
}
