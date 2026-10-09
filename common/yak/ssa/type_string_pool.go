package ssa

import (
	"unsafe"

	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// Replace string headers on a fresh slice, preserving snapshots already held
// by readers. Mutable type identities and name lists are never shared.
func internTypeStrings(typ Type, pool *ssadb.NameCache) {
	if pool == nil {
		return
	}
	var base *baseType
	var names *[]string
	switch t := typ.(type) {
	case *BasicType:
		base, names = &t.baseType, &t.fullTypeName
	case *ObjectType:
		base, names = t.baseType, &t.fullTypeName
	case *FunctionType:
		base, names = t.baseType, &t.fullTypeName
	case *AliasType:
		base, names = t.baseType, &t.fullTypeName
	case *InterfaceType:
		base, names = t.baseType, &t.fullTypeName
	case *ChanType:
		base, names = t.baseType, &t.fullTypeName
	default:
		return
	}
	base.typeNameMu.Lock()
	defer base.typeNameMu.Unlock()
	var shared []string
	for i, name := range *names {
		canonical := pool.Intern(name)
		if unsafe.StringData(canonical) != unsafe.StringData(name) {
			if shared == nil {
				shared = append([]string(nil), (*names)...)
			}
			shared[i] = canonical
		}
	}
	if shared != nil {
		*names = shared
	}
}
