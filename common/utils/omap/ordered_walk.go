package omap

import (
	"reflect"
	"sync"
)

func Walkable(i any) bool {
	_, ok := i.(walkif)
	if ok {
		return true
	}

	switch reflect.TypeOf(i).Kind() {
	case reflect.Map, reflect.Slice, reflect.Array:
		return true
	case reflect.Pointer:
		return Walkable(reflect.ValueOf(i).Elem().Interface())
	default:
		return false
	}
}

func (m *OrderedMap[T, V]) WalkMap(visited *sync.Map, f func(self, key, value any) bool) {
	for _, k := range m.Keys() {
		v, ok := m.Get(k)
		if !ok {
			continue
		}
		var self any = m
		var key any = k
		var val any = v
		if f(self, key, val) {
			walk(visited, val, f)
		}
	}
}

func Walk(m any, f func(parent any, key any, value any) bool) {
	walk(nil, m, f)
}

type walkif interface {
	WalkMap(*sync.Map, func(self, key, value any) bool)
}

func walk(visited *sync.Map, m any, f func(parent any, key any, value any) bool) {
	if visited == nil {
		visited = new(sync.Map)
	}

	mKind := reflect.TypeOf(m).Kind()
	switch mKind {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		// It panics if v's Kind is not Chan, Func, Map, Pointer, Slice, or UnsafePointer.
		mPtr := reflect.ValueOf(m).Pointer()
		if _, ok := visited.Load(mPtr); ok {
			// prevent infinite loop
			return
		}
		visited.Store(mPtr, true)
	default:

	}

	switch mKind {
	case reflect.Map:
		for _, k := range reflect.ValueOf(m).MapKeys() {
			v := reflect.ValueOf(m).MapIndex(k).Interface()
			if f(m, k.Interface(), v) {
				walk(visited, v, f)
			}
		}
	case reflect.Slice:
		for i := 0; i < reflect.ValueOf(m).Len(); i++ {
			v := reflect.ValueOf(m).Index(i).Interface()
			if f(m, i, v) {
				walk(visited, v, f)
			}
		}
	case reflect.Array:
		for i := 0; i < reflect.ValueOf(m).Len(); i++ {
			v := reflect.ValueOf(m).Index(i).Interface()
			if f(m, i, v) {
				walk(visited, v, f)
			}
		}
	case reflect.Ptr:
		ptrTo := reflect.ValueOf(m).Elem()
		switch ptrTo.Kind() {
		case reflect.Map, reflect.Slice, reflect.Array:
			walk(visited, reflect.ValueOf(m).Elem().Interface(), f)
		default:
			// do nothing
			if result, ok := m.(walkif); ok {
				result.WalkMap(visited, f)
			} else if result, ok := ptrTo.Interface().(walkif); ok {
				result.WalkMap(visited, f)
			}
		}
	default:
		if result, ok := m.(walkif); ok {
			result.WalkMap(visited, f)
		}
	}
}
