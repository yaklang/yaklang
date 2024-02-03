package ssautil

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type GlobalIndexFetcher func() int

type ScopedVersionedTable[T comparable] struct {
	offsetFetcher GlobalIndexFetcher // fetch the next global index
	newVersioned  func(versionIndex, globalIndex int, name string, scope *ScopedVersionedTable[T]) VersionedIF[T]

	// record the lexical variable
	values *omap.OrderedMap[string, *omap.OrderedMap[string, VersionedIF[T]]]

	// for closure function or block scope
	captured *omap.OrderedMap[string, VersionedIF[T]]

	incomingPhi *omap.OrderedMap[string, VersionedIF[T]]

	// global id to versioned variable
	table map[int]VersionedIF[T]

	spin           bool
	CreateEmptyPhi func(string) T

	// relations
	parent      *ScopedVersionedTable[T]
	finishChild []*ScopedVersionedTable[T]
	child       []*ScopedVersionedTable[T]
}

func NewScope[T comparable](fetcher func() int, newVersioned func(versionIndex, globalIndex int, name string, scope *ScopedVersionedTable[T]) VersionedIF[T], table map[int]VersionedIF[T], parent *ScopedVersionedTable[T]) *ScopedVersionedTable[T] {
	return &ScopedVersionedTable[T]{
		offsetFetcher: fetcher,
		newVersioned:  newVersioned,
		values:        omap.NewOrderedMap[string, *omap.OrderedMap[string, VersionedIF[T]]](map[string]*omap.OrderedMap[string, VersionedIF[T]]{}),
		captured:      omap.NewOrderedMap[string, VersionedIF[T]](map[string]VersionedIF[T]{}),
		incomingPhi:   omap.NewOrderedMap[string, VersionedIF[T]](map[string]VersionedIF[T]{}),
		table:         table,
		parent:        parent,
		finishChild:   make([]*ScopedVersionedTable[T], 0),
		child:         make([]*ScopedVersionedTable[T], 0),
	}
}

func NewRootVersionedTable[T comparable](newVersioned func(versionIndex, globalIndex int, name string, scope *ScopedVersionedTable[T]) VersionedIF[T], fetcher ...func() int) *ScopedVersionedTable[T] {
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
			id += 1
			return id
		}
	}

	return NewScope[T](finalFetcher, newVersioned, map[int]VersionedIF[T]{}, nil)
}

func (v *ScopedVersionedTable[T]) IsRoot() bool {
	return v.parent == nil
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

func (v *ScopedVersionedTable[T]) CreateLexicalLocalVariable(name string, value T) VersionedIF[T] {
	return v.createLexicalVariableEx(name, value, true)
}

// CreateLexicalVariable create a root lexical variable
// the next versions will be named as "1", "2", "3"...
// the version index is auto set by the order of creation
// means:
//
//	x = 1
//	x = 2
//
// trans to:
//
//	x = 1; x1 = 2
func (v *ScopedVersionedTable[T]) CreateLexicalVariable(name string, value T) VersionedIF[T] {
	return v.createLexicalVariableEx(name, value, false)
}

func (v *ScopedVersionedTable[T]) createLexicalVariableEx(name string, value T, local bool) VersionedIF[T] {
	if ret, ok := v.values.Get(name); !ok {
		v.values.Set(name, omap.NewOrderedMap[string, VersionedIF[T]](map[string]VersionedIF[T]{}))
		return v.createLexicalVariableEx(name, value, local)
	} else {
		verIndex := ret.Len()
		verVar := v.newVar(name, verIndex)
		ret.Add(verVar)
		// register captured variable
		if !local && !v.IsRoot() {
			v.tryRegisterCapturedVariable(name, verVar)
		}
		if !isZeroValue(value) {
			err := verVar.Assign(value)
			if err != nil {
				log.Errorf("assign failed: %v", err)
			}
		}
		return verVar
	}
}

// CreateSymbolicVariable create a non-lexical and no named variable
// for example:
// for f() { // }
// the f()'s return value is a symbolic variable
// we can't trace its lexical name
// the symbol is not traced by some version.
func (v *ScopedVersionedTable[T]) CreateSymbolicVariable(value T) VersionedIF[T] {
	verVar := v.newVar("", 0)
	key := fmt.Sprintf("$%d$", verVar.GetGlobalIndex())
	table := omap.NewOrderedMap[string, VersionedIF[T]](map[string]VersionedIF[T]{})
	table.Add(verVar)
	v.values.Set(key, table)
	if !isZeroValue(value) {
		err := verVar.Assign(value)
		if err != nil {
			log.Errorf("assign failed: %s", err)
		}
	}
	return verVar
}

// try register captured variable
func (v *ScopedVersionedTable[T]) tryRegisterCapturedVariable(name string, ver VersionedIF[T]) {
	if v.IsRoot() {
		return
	}
	// get variable from parent
	parentVariable := v.parent.GetLatestVersionVersioned(name)
	if parentVariable == nil {
		return
	}
	// mark original captured variable
	ver.SetCaptured(parentVariable)
	v.captured.Set(name, ver)
}

func (v *ScopedVersionedTable[T]) newVar(lexName string, versionIndex int) VersionedIF[T] {
	global := v.offsetFetcher()
	varIns := v.newVersioned(
		versionIndex, global,
		lexName, v,
	)
	v.table[global] = varIns
	return varIns
}

// RenameAssociated rename the associated variable, helpful for tracing the object
// for example:
//
//	x = {}
//	a = x
//	a.b = 1
//
// trace:
// x = {}
// // (a.b -> x.b) = 1
// func (v *ScopedVersionedTable[T]) RenameAssociated(globalIdLeft int, globalIdRight int) error {
// 	if _, ok := v.table[globalIdLeft]; !ok {
// 		return fmt.Errorf("can't find variable %d", globalIdLeft)
// 	}
// 	if _, ok := v.table[globalIdRight]; !ok {
// 		return fmt.Errorf("can't find variable %d", globalIdRight)
// 	}

// 	left, right := v.table[globalIdLeft], v.table[globalIdRight]
// 	left.origin = right
// 	return nil
// }

// CreateStaticMemberCallVariable will need a trackable obj, and a trackable member access
func (v *ScopedVersionedTable[T]) CreateStaticMemberCallVariable(obj int, member any, val T) (VersionedIF[T], error) {
	name, err := v.ConvertStaticMemberCallToLexicalName(obj, member)
	if err != nil {
		return nil, err
	}
	return v.CreateLexicalVariable(name, val), nil
}

// CreateDynamicMemberCallVariable will need a trackable obj, and a trackable member access
// member should be a variable
func (v *ScopedVersionedTable[T]) CreateDynamicMemberCallVariable(obj int, member int, val T) (VersionedIF[T], error) {
	name, err := v.ConvertDynamicMemberCallToLexicalName(obj, member)
	if err != nil {
		return nil, err
	}
	return v.CreateLexicalVariable(name, val), nil
}

func (v *ScopedVersionedTable[T]) CreateSubScope() *ScopedVersionedTable[T] {
	sub := NewScope[T](v.offsetFetcher, v.newVersioned, v.table, v)
	v.child = append(v.child, sub)
	return sub
}

// InCurrentLexicalScope check if the variable is in current lexical scope
func (v *ScopedVersionedTable[T]) InCurrentLexicalScope(name string) bool {
	if _, ok := v.values.Get(name); ok {
		return true
	}
	return false
}

// GetLatestVersionInCurrentLexicalScope get the latest version of the variable
// in current scope, not trace to parent scope
func (v *ScopedVersionedTable[T]) GetLatestVersionInCurrentLexicalScope(name string) VersionedIF[T] {
	if ret, ok := v.values.Get(name); !ok {
		return nil
	} else {
		var _, ver, _ = ret.Last()
		return ver
	}
}
func (scope *ScopedVersionedTable[T]) GetLatestVersionVersioned(name string) VersionedIF[T] {
	// var parent = v
	// for parent != nil {
	var ret VersionedIF[T]
	if result := scope.GetLatestVersionInCurrentLexicalScope(name); result != nil {
		ret = result
	} else {
		if scope.parent != nil {
			ret = scope.parent.GetLatestVersionVersioned(name)
		} else {
			ret = nil
		}
	}
	if ret != nil && ret.GetScope() != scope && scope.spin {
		t := scope.CreateLexicalVariable(name, scope.CreateEmptyPhi(name))
		// t.origin = ret
		scope.incomingPhi.Set(name, t)
		ret = t
	}
	return ret
	// parent = parent.parent
	// }
	// return nil
}

func (v *ScopedVersionedTable[T]) GetLatestVersion(name string) (t T) {
	if ret := v.GetLatestVersionVersioned(name); ret != nil {
		return ret.GetValue()
	} else {
		return
	}
}

// GetVersions get all versions of the variable
// trace to parent scope if not found
func (v *ScopedVersionedTable[T]) GetVersions(name string) []VersionedIF[T] {
	var vers []VersionedIF[T]
	var parent = v
	for parent != nil {
		if ret, ok := parent.values.Get(name); ok {
			vers = append(vers, ret.Values()...)
		}
		parent = parent.parent
	}
	return vers
}

// IsCapturedByCurrentScope check if the variable is captured by current scope
// note: closure function and if/for or block scope will capture the variable
// it's useful for trace the phi or mask
// func (v *ScopedVersionedTable[T]) IsCapturedByCurrentScope(name string) bool {
// 	if v.IsRoot() {
// 		log.Warn("root scope can't capture any variable")
// 		return false
// 	}
// 	return v.parent.GetLatestVersionVersioned(name) != nil
// }

// GetAllCapturedVariableNames get the captured variable
func (v *ScopedVersionedTable[T]) GetAllCapturedVariableNames() []string {
	return v.captured.Keys()
}

func (v *ScopedVersionedTable[T]) ConvertStaticMemberCallToLexicalName(obj int, member any) (string, error) {
	left, ok := v.table[obj]
	if !ok {
		return "", nil
	}
	rootLeft := left.GetRootVersion()

	var suffix string
	switch member.(type) {
	case string, []byte, []rune:
		suffix = fmt.Sprintf(".%s", member)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		suffix = fmt.Sprintf("[%d]", member)
	default:
		return "", fmt.Errorf("invalid static member type %T", member)
	}

	return fmt.Sprintf("#%v%s", rootLeft.GetGlobalIndex(), suffix), nil
}

func (v *ScopedVersionedTable[T]) ConvertDynamicMemberCallToLexicalName(obj, member int) (string, error) {
	left, ok := v.table[obj]
	if !ok {
		return "", nil
	}
	right, ok := v.table[member]
	if !ok {
		return "", nil
	}
	rootLeft := left.GetRootVersion()
	rootRight := right.GetRootVersion()

	var suffix = fmt.Sprintf("#%v", rootRight.GetGlobalIndex())
	return fmt.Sprintf("#%v.$(%s)", rootLeft.GetGlobalIndex(), suffix), nil
}

// func (s *ScopedVersionedTable[T]) Merge(sub ...*ScopedVersionedTable[T]) {
// 	if len(sub) == 1 {
// 		// cover origin value
// 		sub[0].captured.ForEach(func(name string, ver VersionedIF[T]) bool {
// 			return true
// 		})
// 	} else {
// 		// merge, generate phi
// 	}
// }
