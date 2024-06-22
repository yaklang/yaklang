package ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
)

type ScopeInstance struct {
	*ssautil.ScopedVersionedTable[Value]
}

type ScopeIF ssautil.ScopedVersionedTableIF[Value]

var _ ssautil.ScopedVersionedTableIF[Value] = (*ScopeInstance)(nil)

func NewScope(name string) *ScopeInstance {
	s := &ScopeInstance{
		ScopedVersionedTable: ssautil.NewRootVersionedTable[Value](name, NewVariable),
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

func GetLazyScopeFromIrScopeId(i int64) ScopeIF {
	return &LazyScope{
		id: i,
	}
}

func GetScopeFromIrScopeId(i int64) *ScopeInstance {
	node, err := ssadb.GetIrScope(i)
	if err != nil {
		log.Warnf("failed to get ir scope: %v", err)
		return nil
	}
	c := NewScope(node.ProgramName)
	c.SetPersistentId(i)
	if err != nil {
		log.Errorf("failed to sync from database: %v", err)
		return nil
	}

	err = SyncFromDatabase(c)
	if err != nil {
		log.Errorf("failed to sync from database: %v", err)
	}
	return c
}
