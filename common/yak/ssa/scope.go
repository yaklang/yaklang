package ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
)

type Scope struct {
	*ssautil.ScopedVersionedTable[Value]
}

type ScopeIF ssautil.ScopedVersionedTableIF[Value]

var _ ssautil.ScopedVersionedTableIF[Value] = (*Scope)(nil)

func NewScope(name string) *Scope {
	s := &Scope{
		ScopedVersionedTable: ssautil.NewRootVersionedTable[Value](name, NewVariable),
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

func GetScopeFromIrScopeId(i int64) *Scope {
	node, err := ssadb.GetIrScope(i)
	if err != nil {
		log.Warnf("failed to get ir scope: %v", err)
		return nil
	}
	c := NewScope(node.ProgramName)
	c.SetPersistentId(i)
	err = c.SyncFromDatabase()
	if err != nil {
		log.Errorf("failed to sync from database: %v", err)
		return nil
	}
	return c
}
