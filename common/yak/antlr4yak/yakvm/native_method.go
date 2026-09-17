package yakvm

import (
	"reflect"
	"sync"
	"sync/atomic"
)

const nativeMethodTypeLimit = 256

// Cache only immutable exported method indices, never bound receivers or
// values. All names for an admitted type are recorded at once: arbitrary
// member names cannot grow the cache. New types beyond the budget retain the
// ordinary reflection path, including its visibility and nil semantics.
var nativeMethodTypes sync.Map // reflect.Type -> map[string]int
var nativeMethodTypeCount atomic.Uint32

func nativeMethodByName(receiver reflect.Value, name string) reflect.Value {
	typ := receiver.Type()
	if typ.NumMethod() == 0 {
		return reflect.Value{}
	}
	indices, found := nativeMethodTypes.Load(typ)
	if !found {
		for {
			n := nativeMethodTypeCount.Load()
			if n >= nativeMethodTypeLimit {
				return receiver.MethodByName(name)
			}
			if nativeMethodTypeCount.CompareAndSwap(n, n+1) {
				break
			}
		}
		methods := make(map[string]int, typ.NumMethod())
		for i := 0; i < typ.NumMethod(); i++ {
			method := typ.Method(i)
			methods[method.Name] = method.Index
		}
		var loaded bool
		indices, loaded = nativeMethodTypes.LoadOrStore(typ, methods)
		if loaded {
			nativeMethodTypeCount.Add(^uint32(0))
		}
	}
	if index, ok := indices.(map[string]int)[name]; ok {
		return receiver.Method(index)
	}
	return reflect.Value{}
}
