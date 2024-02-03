package ssautil

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/yaklang/yaklang/common/utils/omap"
)

type GlobalIndexFetcher func() int

type VersionedBuilder[T comparable] func(globalIndex int, name string, local bool, scope *ScopedVersionedTable[T]) VersionedIF[T]

type ScopedVersionedTable[T comparable] struct {
	offsetFetcher GlobalIndexFetcher // fetch the next global index
	// new versioned variable
	newVersioned VersionedBuilder[T]

	// record the lexical variable
	values   *omap.OrderedMap[string, *omap.OrderedMap[string, VersionedIF[T]]] // from variable get value, assigned variable
	variable *omap.OrderedMap[T, []VersionedIF[T]]                              // from value get variable

	// for closure function or block scope
	captured *omap.OrderedMap[string, VersionedIF[T]]

	incomingPhi *omap.OrderedMap[string, VersionedIF[T]]

	// global id to versioned variable
	table map[int]VersionedIF[T]

	// for loop
	spin           bool
	CreateEmptyPhi func(string) T

	// relations
	parent *ScopedVersionedTable[T]
}

func NewScope[T comparable](fetcher func() int, newVersioned VersionedBuilder[T], table map[int]VersionedIF[T], parent *ScopedVersionedTable[T]) *ScopedVersionedTable[T] {
	return &ScopedVersionedTable[T]{
		offsetFetcher: fetcher,
		newVersioned:  newVersioned,
		values:        omap.NewOrderedMap[string, *omap.OrderedMap[string, VersionedIF[T]]](map[string]*omap.OrderedMap[string, VersionedIF[T]]{}),
		variable:      omap.NewOrderedMap[T, []VersionedIF[T]](map[T][]VersionedIF[T]{}),
		captured:      omap.NewOrderedMap[string, VersionedIF[T]](map[string]VersionedIF[T]{}),
		incomingPhi:   omap.NewOrderedMap[string, VersionedIF[T]](map[string]VersionedIF[T]{}),
		table:         table,
		parent:        parent,
	}
}

func NewRootVersionedTable[T comparable](newVersioned VersionedBuilder[T], fetcher ...func() int) *ScopedVersionedTable[T] {
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

	return NewScope[T](finalFetcher, newVersioned, map[int]VersionedIF[T]{}, nil)
}

func (v *ScopedVersionedTable[T]) CreateSubScope() *ScopedVersionedTable[T] {
	sub := NewScope[T](v.offsetFetcher, v.newVersioned, v.table, v)
	return sub
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
		if scope.parent != nil {
			ret = scope.parent.ReadVariable(name)
		} else {
			ret = nil
		}
	}
	if ret != nil && ret.GetScope() != scope {
		// not in current scope
		if scope.spin {
			t := scope.writeVariable(name, scope.CreateEmptyPhi(name))
			// t.origin = ret
			scope.incomingPhi.Set(name, t)
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

/// ---------------- write

func (v *ScopedVersionedTable[T]) writeVariable(name string, value T) VersionedIF[T] {
	ret := v.CreateVariable(name, false)
	v.AssignVariable(ret, value)
	return ret
}

func (v *ScopedVersionedTable[T]) writeLocalVariable(name string, value T) VersionedIF[T] {
	ret := v.CreateVariable(name, true)
	v.AssignVariable(ret, value)
	return ret
}

// ---------------- create

func (v *ScopedVersionedTable[T]) CreateVariable(name string, isLocal bool) VersionedIF[T] {
	return v.newVar(name, isLocal)
}

// ---------------- Assign
func (scope *ScopedVersionedTable[T]) AssignVariable(variable VersionedIF[T], value T) {
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
	parentVariable := v.parent.ReadVariable(name)
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
		lexName, local, v,
	)
	v.table[global] = varIns
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

func (scope *ScopedVersionedTable[T]) CoverNumberMemberCall(variable VersionedIF[T], index int) string {
	return fmt.Sprintf("#%d[%d]", variable.GetGlobalIndex(), index)
}

func (socep *ScopedVersionedTable[T]) CoverStringMemberCall(variable VersionedIF[T], key string) string {
	return fmt.Sprintf("#%d.%s", variable.GetGlobalIndex(), key)
}

func (scope *ScopedVersionedTable[T]) CoverDynamicMemberCall(variable, key VersionedIF[T]) string {
	return fmt.Sprintf("#%d.$(%d)", variable.GetGlobalIndex(), key.GetGlobalIndex())
}
