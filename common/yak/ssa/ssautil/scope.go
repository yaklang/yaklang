package ssautil

import (
	"reflect"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

// for builder
type GlobalIndexFetcher func() int
type VersionedBuilder[T versionedValue] func(globalIndex int, name string, local bool, scope ScopedVersionedTableIF[T]) VersionedIF[T]

// capture variable
type VariableHandler[T versionedValue] func(string, VersionedIF[T])

// phi

// SpinHandle for Loop Spin Phi
type SpinHandle[T comparable] func(string, T, T, T) map[string]T

// MergeHandle handler Merge Value generate Phi
type MergeHandle[T comparable] func(string, []T) T

// ScopedVersionedTableIF is the interface for scope versioned table
type ScopedVersionedTableIF[T versionedValue] interface {
	// Read Variable by name
	ReadVariable(name string, iscurrent ...bool) VersionedIF[T]
	// read value by name
	ReadValue(name string) T

	// Read Variable from linkSideEffect
	ReadVariableFromLinkSideEffect(name string) (VersionedIF[T], VersionedIF[T])

	GetAllVariables() []VersionedIF[T]
	GetAllVariablesByName(name string, iscurrent ...bool) []VersionedIF[T]

	// create variable, if isLocal is true, the variable is local
	CreateVariable(name string, isLocal bool) VersionedIF[T]

	// assign a value to the variable
	AssignVariable(VersionedIF[T], T, ...bool)

	GetVariableFromValue(T) VersionedIF[T]

	// this scope
	SetThis(ScopedVersionedTableIF[T])
	GetThis() ScopedVersionedTableIF[T]
	SetForceCapture(bool)
	GetForceCapture() bool

	// create sub scope
	CreateSubScope() ScopedVersionedTableIF[T]
	CreateShadowScope() ScopedVersionedTableIF[T]
	// get scope level, each scope has a level, the root scope is 0, the sub scope is {parent-scope.level + 1}
	GetScopeLevel() int
	GetParent() ScopedVersionedTableIF[T]
	SetParent(ScopedVersionedTableIF[T])
	GetHead() ScopedVersionedTableIF[T]

	IsSameOrSubScope(ScopedVersionedTableIF[T]) bool
	Compare(ScopedVersionedTableIF[T]) bool

	// use in ssautil, handle inner member
	ForEachCapturedVariable(VariableHandler[T])
	ForEachCapturedSideEffect(func(string, []VersionedIF[T]))
	SetCapturedVariable(string, VersionedIF[T])
	SetCapturedSideEffect(string, VersionedIF[T], VersionedIF[T])
	ChangeCapturedSideEffect(string, VersionedIF[T])

	// use in phi
	CoverBy(ScopedVersionedTableIF[T])
	Merge(bool, bool, MergeHandle[T], ...ScopedVersionedTableIF[T])
	Spin(ScopedVersionedTableIF[T], ScopedVersionedTableIF[T], SpinHandle[T])
	SetSpin(func(string) T)

	// db
	// SaveToDatabase() error
	// SetCallback() error
	// GetPersistentId() int64
	// SetPersistentId(i int64)
	// SetPersistentNode(*ssadb.IrScopeNode)
	GetScopeName() string
	SetScopeName(string)

	// GetPersistentProgramName() string

	// for return phi
	GetAllVariableNames() map[string]struct{}

	SetExternInfo(string, any)
	GetExternInfo(string) any
}

func (s *ScopedVersionedTable[T]) GetScopeName() string {
	return s.ScopeName
}

func (s *ScopedVersionedTable[T]) SetScopeName(name string) {
	s.ScopeName = name
}
func (s *ScopedVersionedTable[T]) GetForceCapture() bool {
	return s.ForceCapture
}

func (s *ScopedVersionedTable[T]) GetScopeID() int64 {
	return s.ScopeId
}

func (s *ScopedVersionedTable[T]) SetScopeID(i int64) {
	s.ScopeId = i
}

// func (s *ScopedVersionedTable[T]) SetlinkIncomingPhi(name string, v VersionedIF[T]) {
// 	s.linkIncomingPhi[name] = v
// }

// // func (s *ScopedVersionedTable[T]) SetPersistentId(i int64) {
// // 	s.persistentId = i
// // }

// func (s *ScopedVersionedTable[T]) SetPersistentNode(i *ssadb.IrScopeNode) {
// 	s.persistentNode = i
// }

// func (s *ScopedVersionedTable[T]) GetPersistentProgramName() string {
// 	return s.persistentProgramName
// }

type ScopedVersionedTable[T versionedValue] struct {
	ProgramName  string
	ForceCapture bool
	ScopeName    string
	ScopeId      int64

	level         int
	offsetFetcher GlobalIndexFetcher // fetch the next global index
	// new versioned variable
	newVersioned VersionedBuilder[T]

	callback        func(VersionedIF[T])
	linkValues      linkNodeMap[T]
	linkVariable    map[T]VersionedIF[T]
	linkCaptured    map[string]VersionedIF[T]
	linkIncomingPhi map[string]VersionedIF[T]
	linkSideEffect  map[string][]VersionedIF[T]

	//// record the lexical variable
	//values   *omap.OrderedMap[string, *omap.OrderedMap[string, VersionedIF[T]]] // from variable get value, assigned variable
	//variable *omap.OrderedMap[T, []VersionedIF[T]]                              // from value get variable
	//
	//// for closure function or block scope
	//captured *omap.OrderedMap[string, VersionedIF[T]]
	//
	//incomingPhi *omap.OrderedMap[string, VersionedIF[T]]

	// for loop
	spin           bool
	createEmptyPhi func(string) T

	// relations
	this     ScopedVersionedTableIF[T]
	parentId int64

	// do not use _parent direct access
	// use GetParent
	_parent ScopedVersionedTableIF[T]

	externInfo *utils.SafeMap[any]
}

// func (s *ScopedVersionedTable[T]) ShouldSaveToDatabase() bool {
// return s.persistentProgramName != "" && s.persistentId > 0
// }

func (s *ScopedVersionedTable[T]) SetCallback(f func(VersionedIF[T])) {
	s.callback = f
}

func (s *ScopedVersionedTable[T]) SetForceCapture(b bool) {
	s.ForceCapture = b
}

func NewScope[T versionedValue](
	programName string,
	fetcher func() int,
	newVersioned VersionedBuilder[T],
	parent ScopedVersionedTableIF[T],
) *ScopedVersionedTable[T] {
	// var treeNodeId int64
	// var treeNode *ssadb.IrScopeNode
	// if programName != "" {
	// 	treeNodeId, treeNode = ssadb.RequireScopeNode()
	// }
	s := &ScopedVersionedTable[T]{
		ProgramName:   programName,
		offsetFetcher: fetcher,
		newVersioned:  newVersioned,

		callback: func(vi VersionedIF[T]) {},
		// linkValues:      newLinkNodeMap[T](callback),
		linkVariable:    make(map[T]VersionedIF[T]),
		linkCaptured:    make(map[string]VersionedIF[T]),
		linkSideEffect:  make(map[string][]VersionedIF[T]),
		linkIncomingPhi: make(map[string]VersionedIF[T]),
		externInfo:      utils.NewSafeMap[any](),
	}
	s.linkValues = newLinkNodeMap[T](func(i VersionedIF[T]) {
		// s.callback(i)
	})

	s.SetThis(s)
	if parent != nil {
		s.level = parent.GetScopeLevel() + 1
		s.SetParent(parent.GetThis())
	} else {
		s.level = 0
	}
	return s
}

func NewRootVersionedTable[T versionedValue](
	programName string,
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

	return NewScope[T](programName, finalFetcher, newVersioned, nil)
}

func (v *ScopedVersionedTable[T]) CreateSubScope() ScopedVersionedTableIF[T] {
	sub := NewScope[T](v.ProgramName, v.offsetFetcher, v.newVersioned, v)
	sub.SetForceCapture(v.GetForceCapture())
	v.ForEachCapturedSideEffect(func(s string, vi []VersionedIF[T]) {
		sub.SetCapturedSideEffect(s, vi[0], vi[1])
	})
	return sub
}

func (v *ScopedVersionedTable[T]) CreateShadowScope() ScopedVersionedTableIF[T] {
	sub := NewScope[T](v.ProgramName, v.offsetFetcher, v.newVersioned, v)
	sub.SetForceCapture(v.GetForceCapture())

	v.ForEachCapturedVariable(func(s string, vi VersionedIF[T]) {
		sub.SetCapturedVariable(s, vi)
	})
	v.ForEachCapturedSideEffect(func(s string, vi []VersionedIF[T]) {
		sub.SetCapturedSideEffect(s, vi[0], vi[1])
	})
	return sub
}

func (v *ScopedVersionedTable[T]) GetParent() ScopedVersionedTableIF[T] {
	if v._parent == nil {
		if v.parentId <= 0 {
			return nil
		}
		log.Errorf("UNFINISHED for loading parent from database")
		return nil
	}
	return v._parent
}

func (v *ScopedVersionedTable[T]) GetHead() ScopedVersionedTableIF[T] {
	var headScope ScopedVersionedTableIF[T]
	for headScope = v; headScope.GetParent() != nil; headScope = headScope.GetParent() {

	}
	return headScope
}

func (v *ScopedVersionedTable[T]) SetParent(parent ScopedVersionedTableIF[T]) {
	v._parent = parent
	// v.parentId = parent.GetPersistentId()
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
	return v.linkValues.Get(name)
}

func (v *ScopedVersionedTable[T]) getHeadVersionInCurrentLexicalScope(name string) VersionedIF[T] {
	return v.linkValues.GetHead(name)
}

func (v *ScopedVersionedTable[T]) getAllVersionInCurrentLexicalScope(name string) []VersionedIF[T] {
	return v.linkValues.GetAll(name)
}

func (scope *ScopedVersionedTable[T]) ReadVariableFromLinkSideEffect(name string) (VersionedIF[T], VersionedIF[T]) {
	var find, bind VersionedIF[T]
	scope.ForEachCapturedSideEffect(func(s string, vi []VersionedIF[T]) {
		if s == name {
			find = vi[0]
			bind = vi[1]
		}
	})
	return find, bind
}

func (scope *ScopedVersionedTable[T]) ReadVariable(name string, current ...bool) VersionedIF[T] {
	// var parent = v
	// for parent != nil {
	var ret VersionedIF[T]
	var isLocal bool = false
	isCurrent := false
	if len(current) > 0 {
		isCurrent = current[0]
	}

	if result := scope.getLatestVersionInCurrentLexicalScope(name); result != nil {
		ret = result
	} else {
		if scope.GetParent() != nil && !isCurrent {
			ret = scope.GetParent().ReadVariable(name, current...)
		} else {
			ret = nil
		}
	}
	if ret != nil && !scope.Compare(ret.GetScope()) {
		// not in current scope
		if scope.spin {
			if scope.GetParent() == ret.GetScope() {
				isLocal = ret.GetLocal()
			}
			t := scope.CreateVariable(name, isLocal)
			scope.AssignVariable(t, scope.createEmptyPhi(name))
			// t.origin = ret
			scope.linkIncomingPhi[name] = t
			ret = t
		}
	}

	return ret
}

func (scope *ScopedVersionedTable[T]) GetAllVariables() []VersionedIF[T] {
	var ret []VersionedIF[T]

	scope.linkValues.ForEach(func(s string, vi VersionedIF[T]) {
		if s == "" || s == "_" {
			return
		}
		ret = append(ret, vi)
	})

	return ret
}

func (scope *ScopedVersionedTable[T]) GetAllVariablesByName(name string, current ...bool) []VersionedIF[T] {
	var ret []VersionedIF[T]
	isCurrent := false
	if len(current) > 0 {
		isCurrent = current[0]
	}

	if result := scope.getAllVersionInCurrentLexicalScope(name); result != nil {
		ret = append(result, ret...)
	}
	if scope.GetParent() != nil && !isCurrent {
		ret = append(ret, scope.GetParent().GetAllVariablesByName(name, current...)...)
	}
	return ret
}

func (scope *ScopedVersionedTable[T]) GetCurrentVariables(name string) []VersionedIF[T] {
	var ret []VersionedIF[T]

	if result := scope.getAllVersionInCurrentLexicalScope(name); result != nil {
		ret = append(result, ret...)
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
func (scope *ScopedVersionedTable[T]) AssignVariable(variable VersionedIF[T], value T, updateLinks ...bool) {
	// assign
	err := variable.Assign(value)
	if err != nil {
		log.Warnf("BUG: variable.Assign error: %v", err)
		return
	}

	updata := true
	if len(updateLinks) > 0 {
		updata = updateLinks[0]
	}

	if updata {
		// variable to value
		scope.linkValues.Append(variable.GetName(), variable)
		// value to variable
		scope.linkVariable[value] = variable
	}

	// capture variable
	if !variable.GetLocal() && !scope.IsRoot() {
		for _, find := range scope.GetCurrentVariables(variable.GetName()) {
			if find.GetLocal() && variable.GetCaptured().GetGlobalIndex() == find.GetGlobalIndex() {
				return
			}
		}
		scope.tryRegisterCapturedVariable(variable.GetName(), variable)
	}
}

func (scope *ScopedVersionedTable[T]) GetVariableFromValue(value T) VersionedIF[T] {
	if res, ok := scope.linkVariable[value]; ok {
		return res
	}
	return nil
}

func (ps *ScopedVersionedTable[T]) ForEachCapturedVariable(handler VariableHandler[T]) {
	for name, ver := range ps.linkCaptured {
		handler(name, ver)
	}
}

func (ps *ScopedVersionedTable[T]) ForEachCapturedSideEffect(handler func(string, []VersionedIF[T])) {
	for name, ver := range ps.linkSideEffect {
		handler(name, ver)
	}
}

func (scope *ScopedVersionedTable[T]) SetCapturedVariable(name string, ver VersionedIF[T]) {
	scope.linkCaptured[name] = ver
}

func (scope *ScopedVersionedTable[T]) SetCapturedSideEffect(name string, ver, bind VersionedIF[T]) {
	scope.linkSideEffect[name] = []VersionedIF[T]{ver, bind}
}

func (scope *ScopedVersionedTable[T]) ChangeCapturedSideEffect(name string, ver VersionedIF[T]) {
	if vers, ok := scope.linkSideEffect[name]; ok {
		vers[0] = ver
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
	if parentVariable == nil && !v.ForceCapture {
		return
	}
	if parentVariable != nil && ver.GetCaptured().GetGlobalIndex() == ver.GetGlobalIndex() {
		ver.SetCaptured(parentVariable)
	} else {
		// variable := v.GetParent().CreateVariable(name, false)
		// v.GetParent().AssignVariable(variable, ver.GetValue())
	}
	// mark original captured variable
	v.linkCaptured[name] = ver
	//v.captured.Set(name, ver)
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

func (s *ScopedVersionedTable[T]) GetAllVariableNames() map[string]struct{} {
	var names map[string]struct{} = make(map[string]struct{})

	s.linkValues.ForEach(func(s string, vi VersionedIF[T]) {
		if s == "" || s == "_" {
			return
		}

		// TODO: 多值返回时生成的member导致phi值重复，这里暂时先跳过
		if s[0] == '#' {
			return
		}
		names[s] = struct{}{}
	})

	parent := s.GetParent()
	if parent != nil {
		namesParent := parent.GetAllVariableNames()
		for n := range namesParent {
			names[n] = struct{}{}
		}
	}

	return names
}

func (s *ScopedVersionedTable[T]) SetExternInfo(key string, value any) {
	if s == nil {
		return
	}
	s.externInfo.Set(key, value)
}

func (s *ScopedVersionedTable[T]) GetExternInfo(key string) any {
	if s == nil {
		return nil
	}
	if v, ok := s.externInfo.Get(key); ok {
		return v
	}
	return nil
}
