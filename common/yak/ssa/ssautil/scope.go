package ssautil

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"reflect"
	"sync"

	"github.com/yaklang/yaklang/common/utils/omap"
)

// for builder
type GlobalIndexFetcher func() int
type VersionedBuilder[T versionedValue] func(globalIndex int, name string, local bool, scope ScopedVersionedTableIF[T]) VersionedIF[T]

// capture variable
type CaptureVariableHandler[T versionedValue] func(string, VersionedIF[T])

// phi

// SpinHandle for Loop Spin Phi
type SpinHandle[T comparable] func(string, T, T, T) map[string]T

// MergeHandle handler Merge Value generate Phi
type MergeHandle[T comparable] func(string, []T) T

// ScopedVersionedTableIF is the interface for scope versioned table
type ScopedVersionedTableIF[T versionedValue] interface {
	// Read Variable by name
	ReadVariable(name string) VersionedIF[T]
	// read value by name
	ReadValue(name string) T

	// create variable, if isLocal is true, the variable is local
	CreateVariable(name string, isLocal bool) VersionedIF[T]

	// assign a value to the variable
	AssignVariable(VersionedIF[T], T)

	GetVariableFromValue(T) VersionedIF[T]

	// this scope
	SetThis(ScopedVersionedTableIF[T])
	GetThis() ScopedVersionedTableIF[T]

	// create sub scope
	CreateSubScope() ScopedVersionedTableIF[T]
	// get scope level, each scope has a level, the root scope is 0, the sub scope is {parent-scope.level + 1}
	GetScopeLevel() int
	GetParent() ScopedVersionedTableIF[T]
	SetParent(ScopedVersionedTableIF[T])

	IsSameOrSubScope(ScopedVersionedTableIF[T]) bool
	Compare(ScopedVersionedTableIF[T]) bool

	// use in ssautil, handle inner member
	ForEachCapturedVariable(CaptureVariableHandler[T])
	SetCapturedVariable(string, VersionedIF[T])

	// use in phi
	CoverBy(ScopedVersionedTableIF[T])
	Merge(bool, MergeHandle[T], ...ScopedVersionedTableIF[T])
	Spin(ScopedVersionedTableIF[T], ScopedVersionedTableIF[T], SpinHandle[T])
	SetSpin(func(string) T)

	// db
	SyncFromDatabase() error
	SaveToDatabase() error
	GetPersistentId() int64
}

func (s *ScopedVersionedTable[T]) GetPersistentId() int64 {
	return s.persistentId
}

type ScopedVersionedTable[T versionedValue] struct {
	persistentId   int64 // > 0 in db
	persistentNode *ssadb.ScopeNode

	level         int
	offsetFetcher GlobalIndexFetcher // fetch the next global index
	// new versioned variable
	newVersioned VersionedBuilder[T]

	// record the lexical variable
	values   *omap.OrderedMap[string, *omap.OrderedMap[string, VersionedIF[T]]] // from variable get value, assigned variable
	variable *omap.OrderedMap[T, []VersionedIF[T]]                              // from value get variable

	// for closure function or block scope
	captured *omap.OrderedMap[string, VersionedIF[T]]

	incomingPhi *omap.OrderedMap[string, VersionedIF[T]]

	// for loop
	spin           bool
	createEmptyPhi func(string) T

	// relations
	this     ScopedVersionedTableIF[T]
	parentId int64

	// do not use _parent direct access
	// use GetParent
	_parent ScopedVersionedTableIF[T]
}

func NewScope[T versionedValue](
	fetcher func() int,
	newVersioned VersionedBuilder[T],
	parent ScopedVersionedTableIF[T],
) *ScopedVersionedTable[T] {
	treeNodeId, treeNode := ssadb.RequireScopeNode()
	s := &ScopedVersionedTable[T]{
		persistentNode: treeNode,
		persistentId:   treeNodeId,
		offsetFetcher:  fetcher,
		newVersioned:   newVersioned,
		values:         omap.NewOrderedMap[string, *omap.OrderedMap[string, VersionedIF[T]]](map[string]*omap.OrderedMap[string, VersionedIF[T]]{}),
		variable:       omap.NewOrderedMap[T, []VersionedIF[T]](map[T][]VersionedIF[T]{}),
		captured:       omap.NewOrderedMap[string, VersionedIF[T]](map[string]VersionedIF[T]{}),
		incomingPhi:    omap.NewOrderedMap[string, VersionedIF[T]](map[string]VersionedIF[T]{}),
	}
	s.SetThis(s)
	if parent != nil {
		s.level = parent.GetScopeLevel() + 1
		//s.parent = parent.GetThis()
		s.SetParent(parent.GetThis())
	} else {
		s.level = 0
	}
	err := s.SaveToDatabase()
	if err != nil {
		log.Warnf("save to database failed: %s", err)
	}
	return s
}

func NewRootVersionedTable[T versionedValue](
	newVersioned VersionedBuilder[T],
	fetcher ...func() int,
) *ScopedVersionedTable[T] {
	var finalFetcher GlobalIndexFetcher
	for _, f := range fetcher {
		if f != nil {
			finalFetcher = f
			break
		}
	}

	if finalFetcher == nil {
		var id = 0
		var m = new(sync.Mutex)
		finalFetcher = func() int {
			m.Lock()
			defer m.Unlock()
			id++
			return id
		}
	}

	return NewScope[T](finalFetcher, newVersioned, nil)
}

func (v *ScopedVersionedTable[T]) CreateSubScope() ScopedVersionedTableIF[T] {
	sub := NewScope[T](v.offsetFetcher, v.newVersioned, v)
	return sub
}

func (v *ScopedVersionedTable[T]) GetParent() ScopedVersionedTableIF[T] {
	if v._parent == nil {
		if v.parentId <= 0 {
			return nil
		}
		panic("UNFINISHED for loading parent from database")
	}
	return v._parent
}

func (v *ScopedVersionedTable[T]) SetParent(parent ScopedVersionedTableIF[T]) {
	v._parent = parent
	v.parentId = parent.GetPersistentId()
}

func (v *ScopedVersionedTable[T]) IsRoot() bool {
	return v._parent == nil && v.parentId <= 0
}

func (v *ScopedVersionedTable[T]) SetThis(scope ScopedVersionedTableIF[T]) {
	v.this = scope
}

func (v *ScopedVersionedTable[T]) GetThis() ScopedVersionedTableIF[T] {
	return v.this
}

func isZeroValue(i any) bool {
	if i == nil {
		return true
	}

	rv := reflect.ValueOf(i)
	if !rv.IsValid() {
		return true
	}
	return reflect.ValueOf(i).IsZero()
}

// ---------------- read

func (v *ScopedVersionedTable[T]) getLatestVersionInCurrentLexicalScope(name string) VersionedIF[T] {
	if ret, ok := v.values.Get(name); !ok {
		return nil
	} else {
		var _, ver, _ = ret.Last()
		return ver
	}
}
func (scope *ScopedVersionedTable[T]) ReadVariable(name string) VersionedIF[T] {
	// var parent = v
	// for parent != nil {
	var ret VersionedIF[T]
	if result := scope.getLatestVersionInCurrentLexicalScope(name); result != nil {
		ret = result
	} else {
		if scope.GetParent() != nil {
			ret = scope.GetParent().ReadVariable(name)
		} else {
			ret = nil
		}
	}
	if ret != nil && !scope.Compare(ret.GetScope()) {
		// not in current scope
		if scope.spin {
			t := scope.CreateVariable(name, false)
			scope.AssignVariable(t, scope.createEmptyPhi(name))
			// t.origin = ret
			scope.incomingPhi.Set(name, t)
			if err := scope.SaveToDatabase(); err != nil {
				log.Warnf("save to database failed: %s", err)
			}
			ret = t
		}
	}

	return ret
}

func (v *ScopedVersionedTable[T]) ReadValue(name string) (t T) {
	if ret := v.ReadVariable(name); ret != nil {
		return ret.GetValue()
	}
	return
}

// ---------------- create

func (v *ScopedVersionedTable[T]) CreateVariable(name string, isLocal bool) VersionedIF[T] {
	return v.newVar(name, isLocal)
}

// ---------------- Assign
func (scope *ScopedVersionedTable[T]) AssignVariable(variable VersionedIF[T], value T) {
	defer func() {
		if err := scope.SaveToDatabase(); err != nil {
			log.Warnf("sync scope to database failed: %s", err)
		}
	}()

	ret, ok := scope.values.Get(variable.GetName())
	if !ok {
		scope.values.Set(variable.GetName(), omap.NewOrderedMap[string, VersionedIF[T]](map[string]VersionedIF[T]{}))
		scope.AssignVariable(variable, value)
		return
	}

	variable.Assign(value)

	variable.SetVersion(ret.Len())
	ret.Add(variable)

	{
		variables, ok := scope.variable.Get(value)
		if !ok {
			variables = make([]VersionedIF[T], 0, 1)
		}
		variables = append(variables, variable)
		scope.variable.Set(value, variables)
	}

	if !variable.GetLocal() && !scope.IsRoot() {
		scope.tryRegisterCapturedVariable(variable.GetName(), variable)
	}
}

func (scope *ScopedVersionedTable[T]) GetVariableFromValue(value T) VersionedIF[T] {
	variables, ok := scope.variable.Get(value)
	if ok {
		return variables[len(variables)-1]
	}
	return nil
}

func (ps *ScopedVersionedTable[T]) ForEachCapturedVariable(handler CaptureVariableHandler[T]) {
	ps.captured.ForEach(func(name string, ver VersionedIF[T]) bool {
		handler(name, ver)
		return true
	})
}

func (scope *ScopedVersionedTable[T]) SetCapturedVariable(name string, ver VersionedIF[T]) {
	scope.captured.Set(name, ver)
	if err := scope.SaveToDatabase(); err != nil {
		log.Warnf("save to database failed: %s", err)
	}
}

// CreateSymbolicVariable create a non-lexical and no named variable
// for example:
// for f() { // }
// the f()'s return value is a symbolic variable
// we can't trace its lexical name
// the symbol is not traced by some version.
// func (v *ScopedVersionedTable[T]) CreateSymbolicVariable(value T) VersionedIF[T] {
// 	verVar := v.newVar("", false)
// 	key := fmt.Sprintf("$%d$", verVar.GetGlobalIndex())
// 	table := omap.NewOrderedMap[string, VersionedIF[T]](map[string]VersionedIF[T]{})
// 	table.Add(verVar)
// 	v.values.Set(key, table)
// 	if !isZeroValue(value) {
// 		err := verVar.Assign(value)
// 		if err != nil {
// 			log.Errorf("assign failed: %s", err)
// 		}
// 	}
// 	return verVar
// }

// try register captured variable
func (v *ScopedVersionedTable[T]) tryRegisterCapturedVariable(name string, ver VersionedIF[T]) {
	if v.IsRoot() {
		return
	}
	// get variable from parent
	parentVariable := v.GetParent().ReadVariable(name)
	if parentVariable == nil {
		return
	}
	// mark original captured variable
	ver.SetCaptured(parentVariable)
	v.captured.Set(name, ver)
}

func (v *ScopedVersionedTable[T]) newVar(lexName string, local bool) VersionedIF[T] {
	global := v.offsetFetcher()
	varIns := v.newVersioned(
		global,
		lexName, local, v.GetThis(),
	)
	return varIns
}

// // RenameAssociated rename the associated variable, helpful for tracing the object
// // for example:
// //
// //	x = {}
// //	a = x
// //	a.b = 1
// //
// // trace:
// // x = {}
// // // (a.b -> x.b) = 1
// // func (v *ScopedVersionedTable[T]) RenameAssociated(globalIdLeft int, globalIdRight int) error {
// // 	if _, ok := v.table[globalIdLeft]; !ok {
// // 		return fmt.Errorf("can't find variable %d", globalIdLeft)
// // 	}
// // 	if _, ok := v.table[globalIdRight]; !ok {
// // 		return fmt.Errorf("can't find variable %d", globalIdRight)
// // 	}

// // 	left, right := v.table[globalIdLeft], v.table[globalIdRight]
// // 	left.origin = right
// // 	return nil
// // }

// // CreateStaticMemberCallVariable will need a trackable obj, and a trackable member access
// func (v *ScopedVersionedTable[T]) CreateStaticMemberCallVariable(obj int, member any, val T) (VersionedIF[T], error) {
// 	name, err := v.ConvertStaticMemberCallToLexicalName(obj, member)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return v.writeVariable(name, val), nil
// }

// // CreateDynamicMemberCallVariable will need a trackable obj, and a trackable member access
// // member should be a variable
// func (v *ScopedVersionedTable[T]) CreateDynamicMemberCallVariable(obj int, member int, val T) (VersionedIF[T], error) {
// 	name, err := v.ConvertDynamicMemberCallToLexicalName(obj, member)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return v.writeVariable(name, val), nil
// }

// // InCurrentLexicalScope check if the variable is in current lexical scope
// func (v *ScopedVersionedTable[T]) InCurrentLexicalScope(name string) bool {
// 	if _, ok := v.values.Get(name); ok {
// 		return true
// 	}
// 	return false
// }

// GetLatestVersionInCurrentLexicalScope get the latest version of the variable
// in current scope, not trace to parent scope

// // GetVersions get all versions of the variable
// // trace to parent scope if not found
// func (v *ScopedVersionedTable[T]) GetVersions(name string) []VersionedIF[T] {
// 	var vers []VersionedIF[T]
// 	var parent = v
// 	for parent != nil {
// 		if ret, ok := parent.values.Get(name); ok {
// 			vers = append(vers, ret.Values()...)
// 		}
// 		parent = parent.parent
// 	}
// 	return vers
// }

// // IsCapturedByCurrentScope check if the variable is captured by current scope
// // note: closure function and if/for or block scope will capture the variable
// // it's useful for trace the phi or mask
// // func (v *ScopedVersionedTable[T]) IsCapturedByCurrentScope(name string) bool {
// // 	if v.IsRoot() {
// // 		log.Warn("root scope can't capture any variable")
// // 		return false
// // 	}
// // 	return v.parent.ReadVariable(name) != nil
// // }

// // GetAllCapturedVariableNames get the captured variable
// func (v *ScopedVersionedTable[T]) GetAllCapturedVariableNames() []string {
// 	return v.captured.Keys()
// }

// use for up lever
func (s *ScopedVersionedTable[T]) GetScopeLevel() int {
	return s.level
}

// IsSubScope check if the scope is sub scope of the other
func (s *ScopedVersionedTable[T]) IsSameOrSubScope(other ScopedVersionedTableIF[T]) bool {
	// if scope level lower, scope will in top than other

	var scope ScopedVersionedTableIF[T] = s

	for ; scope != nil; scope = scope.GetParent() {
		// if scope level is lower than other, break
		if scope.GetScopeLevel() < other.GetScopeLevel() {
			break
		}

		//  only scope == other, return true,
		// scope is sub-scope of other
		if scope.Compare(other) {
			return true
		}
	}

	return false
}

func (s *ScopedVersionedTable[T]) Compare(other ScopedVersionedTableIF[T]) bool {
	return s.GetThis() == other.GetThis()
}
