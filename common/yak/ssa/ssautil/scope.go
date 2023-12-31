package ssautil

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"sync"
)

type GlobalIndexFetcher func() int

type ScopedVersionedTable[T any] struct {
	offsetFetcher GlobalIndexFetcher // fetch the next global index

	// record the lexical variable
	values *omap.OrderedMap[string, *omap.OrderedMap[string, *Versioned[T]]]

	// for closure function or block scope
	captured *omap.OrderedMap[string, *Versioned[T]]

	// global id to versioned variable
	table map[int]*Versioned[T]

	// relations
	parent *ScopedVersionedTable[T]
}

func NewRootVersionedTable[T any](fetcher ...func() int) *ScopedVersionedTable[T] {
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

	return &ScopedVersionedTable[T]{
		offsetFetcher: finalFetcher,
		values:        omap.NewOrderedMap[string, *omap.OrderedMap[string, *Versioned[T]]](map[string]*omap.OrderedMap[string, *Versioned[T]]{}),
		captured:      omap.NewOrderedMap[string, *Versioned[T]](map[string]*Versioned[T]{}),
		table:         map[int]*Versioned[T]{},
	}
}

func (v *ScopedVersionedTable[T]) IsRoot() bool {
	return v.parent == nil
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
func (v *ScopedVersionedTable[T]) CreateLexicalVariable(name string, value T) *Versioned[T] {
	if ret, ok := v.values.Get(name); !ok {
		v.values.Set(name, omap.NewOrderedMap[string, *Versioned[T]](map[string]*Versioned[T]{}))
		return v.CreateLexicalVariable(name, value)
	} else {
		verIndex := ret.Len()
		verVar := v.newVar(name, verIndex)
		ret.Add(verVar)
		// register captured variable
		if !v.IsRoot() && v.IsCapturedByCurrentScope(name) {
			v.registerCapturedVariable(name, verVar)
		}
		if value == nil {
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
func (v *ScopedVersionedTable[T]) CreateSymbolicVariable(value T) *Versioned[T] {
	verVar := v.newVar("", 0)
	key := fmt.Sprintf("$%d$", verVar.globalIndex)
	table := omap.NewOrderedMap[string, *Versioned[T]](map[string]*Versioned[T]{})
	table.Add(verVar)
	v.values.Set(key, table)
	if value != nil {
		err := verVar.Assign(value)
		if err != nil {
			log.Errorf("assign failed: %s", err)
		}
	}
	return verVar
}

func (v *ScopedVersionedTable[T]) registerCapturedVariable(name string, ver *Versioned[T]) {
	v.captured.Set(name, ver)
}

func (v *ScopedVersionedTable[T]) newVar(lexName string, versionIndex int) *Versioned[T] {
	global := v.offsetFetcher()
	varIns := &Versioned[T]{
		versionIndex: versionIndex,
		globalIndex:  global,
		lexicalName:  lexName,
		scope:        v,
		isAssigned:   utils.NewBool(false),
	}
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
// (a.b -> x.b) = 1
func (v *ScopedVersionedTable[T]) RenameAssociated(globalIdLeft int, globalIdRight int) error {
	if _, ok := v.table[globalIdLeft]; !ok {
		return fmt.Errorf("can't find variable %d", globalIdLeft)
	}
	if _, ok := v.table[globalIdRight]; !ok {
		return fmt.Errorf("can't find variable %d", globalIdRight)
	}

	left, right := v.table[globalIdLeft], v.table[globalIdRight]
	left.origin = right
	return nil
}

// CreateStaticMemberCallVariable will need a trackable obj, and a trackable member access
func (v *ScopedVersionedTable[T]) CreateStaticMemberCallVariable(obj int, member any, val T) (*Versioned[T], error) {
	name, err := v.ConvertStaticMemberCallToLexicalName(obj, member)
	if err != nil {
		return nil, err
	}
	return v.CreateLexicalVariable(name, val), nil
}

// CreateDynamicMemberCallVariable will need a trackable obj, and a trackable member access
// member should be a variable
func (v *ScopedVersionedTable[T]) CreateDynamicMemberCallVariable(obj int, member int, val T) (*Versioned[T], error) {
	name, err := v.ConvertDynamicMemberCallToLexicalName(obj, member)
	if err != nil {
		return nil, err
	}
	return v.CreateLexicalVariable(name, val), nil
}

func (v *ScopedVersionedTable[T]) CreateSubScope() *ScopedVersionedTable[T] {
	return &ScopedVersionedTable[T]{
		offsetFetcher: v.offsetFetcher,
		values:        omap.NewOrderedMap[string, *omap.OrderedMap[string, *Versioned[T]]](map[string]*omap.OrderedMap[string, *Versioned[T]]{}),
		captured:      omap.NewOrderedMap[string, *Versioned[T]](map[string]*Versioned[T]{}),
		table:         v.table,
		parent:        v,
	}
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
func (v *ScopedVersionedTable[T]) GetLatestVersionInCurrentLexicalScope(name string) *Versioned[T] {
	if ret, ok := v.values.Get(name); !ok {
		return nil
	} else {
		var _, ver, _ = ret.Last()
		return ver
	}
}

func (v *ScopedVersionedTable[T]) GetLatestVersion(name string) *Versioned[T] {
	var parent = v
	for parent != nil {
		result := parent.GetLatestVersionInCurrentLexicalScope(name)
		if result != nil {
			return result
		}
		parent = parent.parent
	}
	return nil
}

// GetVersions get all versions of the variable
// trace to parent scope if not found
func (v *ScopedVersionedTable[T]) GetVersions(name string) []*Versioned[T] {
	var vers []*Versioned[T]
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
func (v *ScopedVersionedTable[T]) IsCapturedByCurrentScope(name string) bool {
	if v.IsRoot() {
		log.Warn("root scope can't capture any variable")
		return false
	}
	return v.parent.GetLatestVersion(name) != nil
}

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

	return fmt.Sprintf("#%v%s", rootLeft.GetId(), suffix), nil
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

	var suffix = fmt.Sprintf("#%v", rootRight.GetId())
	return fmt.Sprintf("#%v.$(%s)", rootLeft.GetId(), suffix), nil
}
