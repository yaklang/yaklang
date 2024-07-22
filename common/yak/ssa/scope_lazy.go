package ssa

// import (
// 	"github.com/yaklang/yaklang/common/log"
// 	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
// 	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
// )

// var _ ssautil.ScopedVersionedTableIF[Value] = (*ScopeInstance)(nil)

// type LazyScope struct {
// 	notFound bool
// 	id       int64
// 	self     *ScopeInstance
// }

// func (l *LazyScope) ReadVariable(name string) ssautil.VersionedIF[Value] {
// 	if l.notFound {
// 		log.Warnf("scope[%v] is not found in db", l.id)
// 		return nil
// 	}
// 	if l.self == nil {
// 		l.self = GetScopeFromIrScopeId(l.id)
// 		if l.self == nil {
// 			l.notFound = true
// 			log.Warnf("failed to get scope from ir scope id: %v", l.id)
// 			return nil
// 		}
// 	}
// 	return l.self.ReadVariable(name)
// }

// func (l *LazyScope) GetPersistentProgramName() string {
// 	if l.notFound {
// 		log.Warnf("scope[%v] is not found in db", l.id)
// 		return ""
// 	}
// 	if l.self == nil {
// 		l.self = GetScopeFromIrScopeId(l.id)
// 		if l.self == nil {
// 			l.notFound = true
// 			log.Warnf("failed to get scope from ir scope id: %v", l.id)
// 			return ""
// 		}
// 	}
// 	// return l.self.GetPersistentProgramName()
// 	return ""
// }

// func (l *LazyScope) ReadValue(name string) Value {
// 	if l.notFound {
// 		log.Warnf("scope[%v] is not found in db", l.id)
// 		return nil
// 	}
// 	if l.self == nil {
// 		l.self = GetScopeFromIrScopeId(l.id)
// 		if l.self == nil {
// 			l.notFound = true
// 			log.Warnf("failed to get scope from ir scope id: %v", l.id)
// 			return nil
// 		}
// 	}
// 	return l.self.ReadValue(name)
// }

// func (l *LazyScope) CreateVariable(name string, isLocal bool) ssautil.VersionedIF[Value] {
// 	if l.notFound {
// 		log.Warnf("scope[%v] is not found in db", l.id)
// 		return nil
// 	}
// 	if l.self == nil {
// 		l.self = GetScopeFromIrScopeId(l.id)
// 		if l.self == nil {
// 			l.notFound = true
// 			log.Warnf("failed to get scope from ir scope id: %v", l.id)
// 			return nil
// 		}
// 	}
// 	return l.self.CreateVariable(name, isLocal)
// }

// func (l *LazyScope) AssignVariable(v ssautil.VersionedIF[Value], t Value) {
// 	if l.notFound {
// 		log.Warnf("scope[%v] is not found in db", l.id)
// 		return
// 	}
// 	if l.self == nil {
// 		l.self = GetScopeFromIrScopeId(l.id)
// 		if l.self == nil {
// 			l.notFound = true
// 			log.Warnf("failed to get scope from ir scope id: %v", l.id)
// 			return
// 		}
// 	}
// 	l.self.AssignVariable(v, t)
// }

// func (l *LazyScope) GetVariableFromValue(t Value) ssautil.VersionedIF[Value] {
// 	if l.notFound {
// 		log.Warnf("scope[%v] is not found in db", l.id)
// 		return nil
// 	}
// 	if l.self == nil {
// 		l.self = GetScopeFromIrScopeId(l.id)
// 		if l.self == nil {
// 			l.notFound = true
// 			log.Warnf("failed to get scope from ir scope id: %v", l.id)
// 			return nil
// 		}
// 	}
// 	return l.self.GetVariableFromValue(t)
// }

// func (l *LazyScope) SetThis(s ssautil.ScopedVersionedTableIF[Value]) {
// 	if l.notFound {
// 		log.Warnf("scope[%v] is not found in db", l.id)
// 		return
// 	}
// 	if l.self == nil {
// 		l.self = GetScopeFromIrScopeId(l.id)
// 		if l.self == nil {
// 			l.notFound = true
// 			log.Warnf("failed to get scope from ir scope id: %v", l.id)
// 			return
// 		}
// 	}
// 	l.self.SetThis(s)
// }

// func (l *LazyScope) GetThis() ssautil.ScopedVersionedTableIF[Value] {
// 	if l.notFound {
// 		log.Warnf("scope[%v] is not found in db", l.id)
// 		return nil
// 	}
// 	if l.self == nil {
// 		l.self = GetScopeFromIrScopeId(l.id)
// 		if l.self == nil {
// 			l.notFound = true
// 			log.Warnf("failed to get scope from ir scope id: %v", l.id)
// 			return nil
// 		}
// 	}
// 	return l.self.GetThis()
// }

// func (l *LazyScope) CreateSubScope() ssautil.ScopedVersionedTableIF[Value] {
// 	if l.notFound {
// 		log.Warnf("scope[%v] is not found in db", l.id)
// 		return nil
// 	}
// 	if l.self == nil {
// 		l.self = GetScopeFromIrScopeId(l.id)
// 		if l.self == nil {
// 			l.notFound = true
// 			log.Warnf("failed to get scope from ir scope id: %v", l.id)
// 			return nil
// 		}
// 	}
// 	return l.self.CreateSubScope()
// }

// func (l *LazyScope) GetScopeLevel() int {
// 	if l.notFound {
// 		log.Warnf("scope[%v] is not found in db", l.id)
// 		return 0
// 	}
// 	if l.self == nil {
// 		l.self = GetScopeFromIrScopeId(l.id)
// 		if l.self == nil {
// 			l.notFound = true
// 			log.Warnf("failed to get scope from ir scope id: %v", l.id)
// 			return 0
// 		}
// 	}
// 	return l.self.GetScopeLevel()
// }

// func (l *LazyScope) GetParent() ssautil.ScopedVersionedTableIF[Value] {
// 	if l.notFound {
// 		log.Warnf("scope[%v] is not found in db", l.id)
// 		return nil
// 	}
// 	if l.self == nil {
// 		l.self = GetScopeFromIrScopeId(l.id)
// 		if l.self == nil {
// 			l.notFound = true
// 			log.Warnf("failed to get scope from ir scope id: %v", l.id)
// 			return nil
// 		}
// 	}
// 	return l.self.GetParent()
// }

// func (l *LazyScope) SetParent(s ssautil.ScopedVersionedTableIF[Value]) {
// 	if l.notFound {
// 		log.Warnf("scope[%v] is not found in db", l.id)
// 		return
// 	}
// 	if l.self == nil {
// 		l.self = GetScopeFromIrScopeId(l.id)
// 		if l.self == nil {
// 			l.notFound = true
// 			log.Warnf("failed to get scope from ir scope id: %v", l.id)
// 			return
// 		}
// 	}
// 	l.self.SetParent(s)
// }

// func (l *LazyScope) IsSameOrSubScope(s ssautil.ScopedVersionedTableIF[Value]) bool {
// 	if l.notFound {
// 		log.Warnf("scope[%v] is not found in db", l.id)
// 		return false
// 	}
// 	if l.self == nil {
// 		l.self = GetScopeFromIrScopeId(l.id)
// 		if l.self == nil {
// 			l.notFound = true
// 			log.Warnf("failed to get scope from ir scope id: %v", l.id)
// 			return false
// 		}
// 	}
// 	return l.self.IsSameOrSubScope(s)
// }

// func (l *LazyScope) Compare(s ssautil.ScopedVersionedTableIF[Value]) bool {
// 	if l.notFound {
// 		log.Warnf("scope[%v] is not found in db", l.id)
// 		return false
// 	}
// 	if l.self == nil {
// 		l.self = GetScopeFromIrScopeId(l.id)
// 		if l.self == nil {
// 			l.notFound = true
// 			log.Warnf("failed to get scope from ir scope id: %v", l.id)
// 			return false
// 		}
// 	}
// 	return l.self.Compare(s)
// }

// func (l *LazyScope) ForEachCapturedVariable(c ssautil.VariableHandler[Value]) {
// 	if l.notFound {
// 		log.Warnf("scope[%v] is not found in db", l.id)
// 		return
// 	}
// 	if l.self == nil {
// 		l.self = GetScopeFromIrScopeId(l.id)
// 		if l.self == nil {
// 			l.notFound = true
// 			log.Warnf("failed to get scope from ir scope id: %v", l.id)
// 			return
// 		}
// 	}
// 	l.self.ForEachCapturedVariable(c)
// }

// func (l *LazyScope) SetCapturedVariable(s string, v ssautil.VersionedIF[Value]) {
// 	if l.notFound {
// 		log.Warnf("scope[%v] is not found in db", l.id)
// 		return
// 	}
// 	if l.self == nil {
// 		l.self = GetScopeFromIrScopeId(l.id)
// 		if l.self == nil {
// 			l.notFound = true
// 			log.Warnf("failed to get scope from ir scope id: %v", l.id)
// 			return
// 		}
// 	}
// 	l.self.SetCapturedVariable(s, v)
// }

// func (l *LazyScope) CoverBy(s ssautil.ScopedVersionedTableIF[Value]) {
// 	if l.notFound {
// 		log.Warnf("scope[%v] is not found in db", l.id)
// 		return
// 	}
// 	if l.self == nil {
// 		l.self = GetScopeFromIrScopeId(l.id)
// 		if l.self == nil {
// 			l.notFound = true
// 			log.Warnf("failed to get scope from ir scope id: %v", l.id)
// 			return
// 		}
// 	}
// 	l.self.CoverBy(s)
// }

// func (l *LazyScope) Merge(b bool, m ssautil.MergeHandle[Value], s ...ssautil.ScopedVersionedTableIF[Value]) {
// 	if l.notFound {
// 		log.Warnf("scope[%v] is not found in db", l.id)
// 		return
// 	}
// 	if l.self == nil {
// 		l.self = GetScopeFromIrScopeId(l.id)
// 		if l.self == nil {
// 			l.notFound = true
// 			log.Warnf("failed to get scope from ir scope id: %v", l.id)
// 			return
// 		}
// 	}
// 	l.self.Merge(b, m, s...)
// }

// func (l *LazyScope) Spin(s ssautil.ScopedVersionedTableIF[Value], s2 ssautil.ScopedVersionedTableIF[Value], s3 ssautil.SpinHandle[Value]) {
// 	if l.notFound {
// 		log.Warnf("scope[%v] is not found in db", l.id)
// 		return
// 	}
// 	if l.self == nil {
// 		l.self = GetScopeFromIrScopeId(l.id)
// 		if l.self == nil {
// 			l.notFound = true
// 			log.Warnf("failed to get scope from ir scope id: %v", l.id)
// 			return
// 		}
// 	}
// 	l.self.Spin(s, s2, s3)
// }

// func (l *LazyScope) SetSpin(f func(string) Value) {
// 	if l.notFound {
// 		log.Warnf("scope[%v] is not found in db", l.id)
// 		return
// 	}
// 	if l.self == nil {
// 		l.self = GetScopeFromIrScopeId(l.id)
// 		if l.self == nil {
// 			l.notFound = true
// 			log.Warnf("failed to get scope from ir scope id: %v", l.id)
// 			return
// 		}
// 	}
// 	l.self.SetSpin(f)
// }

// func (l *LazyScope) GetPersistentId() int64 {
// 	return l.id
// }

// func (l *LazyScope) SetPersistentId(i int64) {
// 	if l.id == i {
// 		return
// 	}
// 	l.id = i
// 	l.notFound = false
// 	l.self = nil
// }

// func (l *LazyScope) SetPersistentNode(node *ssadb.IrScopeNode) {
// 	if l.notFound {
// 		log.Warnf("scope[%v] is not found in db", l.id)
// 		return
// 	}
// 	if l.self == nil {
// 		l.self = GetScopeFromIrScopeId(l.id)
// 		if l.self == nil {
// 			l.notFound = true
// 			log.Warnf("failed to get scope from ir scope id: %v", l.id)
// 			return
// 		}
// 	}
// 	// l.self.SetPersistentNode(node)
// }

// var _ ssautil.ScopedVersionedTableIF[Value] = (*LazyScope)(nil)
