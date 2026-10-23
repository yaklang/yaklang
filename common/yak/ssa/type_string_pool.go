package ssa

import (
	"unsafe"

	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// Replace string headers on a fresh key chain, preserving snapshots already
// held by readers. Mutable type identities and name lists are never shared.
func internTypeStrings(typ Type, pool *ssadb.NameCache) {
	if pool == nil {
		return
	}
	var names *orderedTypeNames
	switch t := typ.(type) {
	case *BasicType:
		names = &t.fullTypeName
	case *ObjectType:
		names = &t.fullTypeName
	case *FunctionType:
		names = &t.fullTypeName
	case *AliasType:
		names = &t.fullTypeName
	case *InterfaceType:
		names = &t.fullTypeName
	case *ChanType:
		names = &t.fullTypeName
	default:
		return
	}
	names.rewrite(func(keys []string) []string {
		var shared []string
		for i, name := range keys {
			canonical := pool.Intern(name)
			if unsafe.StringData(canonical) != unsafe.StringData(name) {
				if shared == nil {
					shared = append([]string(nil), keys...)
				}
				shared[i] = canonical
			}
		}
		return shared
	})
}
