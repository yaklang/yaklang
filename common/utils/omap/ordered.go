package omap

import (
	"reflect"
	"sort"
	"sync"
)

type OrderedMap[T comparable, V any] struct {
	lock     *sync.RWMutex
	m        map[T]V
	indexMap map[T]int
	keyChain []T
}

func NewEmptyOrderedMap[T comparable, V any]() *OrderedMap[T, V] {
	return &OrderedMap[T, V]{
		lock:     new(sync.RWMutex),
		m:        make(map[T]V),
		keyChain: make([]T, 0),
	}
}

func NewOrderedMap[T comparable, V any](m map[T]V, initOrder ...func(int, int) bool) *OrderedMap[T, V] {
	if m == nil {
		return NewEmptyOrderedMap[T, V]()
	}
	k := make([]T, 0)
	for key := range m {
		k = append(k, key)
	}
	for _, s := range initOrder {
		sort.SliceStable(k, s)
	}
	return &OrderedMap[T, V]{
		lock:     new(sync.RWMutex),
		m:        m,
		keyChain: k,
	}
}

func (o *OrderedMap[T, V]) Set(key T, v V) {
	o.lock.Lock()
	defer o.lock.Unlock()

	_, ok := o.m[key]
	if !ok {
		o.m[key] = v
		o.keyChain = append(o.keyChain, key)
		return
	}

	// existed
	o.m[key] = v
}

func (o *OrderedMap[T, V]) Get(key T) (V, bool) {
	o.lock.RLock()
	defer o.lock.RUnlock()

	v, ok := o.m[key]
	return v, ok
}

func (o *OrderedMap[T, V]) GetByIndex(index int) (V, bool) {
	o.lock.RLock()
	defer o.lock.RUnlock()

	if index < 0 || index >= len(o.keyChain) {
		var z V
		return z, false
	}

	return o.m[o.keyChain[index]], true
}

func (o *OrderedMap[T, V]) First() (T, V, bool) {
	o.lock.RLock()
	defer o.lock.RUnlock()

	if len(o.keyChain) == 0 {
		var z T
		var v V
		return z, v, false
	}

	return o.keyChain[0], o.m[o.keyChain[0]], true
}

func (o *OrderedMap[T, V]) Last() (T, V, bool) {
	o.lock.RLock()
	defer o.lock.RUnlock()

	if len(o.keyChain) == 0 {
		var z T
		var v V
		return z, v, false
	}

	mi := len(o.keyChain) - 1
	return o.keyChain[mi], o.m[o.keyChain[mi]], true
}

func (o *OrderedMap[T, V]) Len() int {
	o.lock.RLock()
	defer o.lock.RUnlock()

	return len(o.keyChain)
}

func (o *OrderedMap[T, V]) Delete(key T) {
	o.lock.Lock()
	defer o.lock.Unlock()

	delete(o.m, key)
	var index = -1
	for i, k := range o.keyChain {
		if k == key {
			index = i
			break
		}
	}
	if index == -1 {
		return
	}

	after := make([]T, len(o.keyChain)-1)
	copy(after, o.keyChain[:index])
	copy(after[index:], o.keyChain[index+1:])
	o.keyChain = after
}

func (o *OrderedMap[T, V]) Keys() []T {
	o.lock.RLock()
	defer o.lock.RUnlock()

	return o.keyChain
}

func (o *OrderedMap[T, V]) Values() []V {
	o.lock.RLock()
	defer o.lock.RUnlock()

	values := make([]V, len(o.keyChain))
	for i, k := range o.keyChain {
		values[i] = o.m[k]
	}
	return values
}

func (o *OrderedMap[T, V]) Have(i any) bool {
	o.lock.RLock()
	defer o.lock.RUnlock()

	switch i.(type) {
	case T:
		_, ok := o.m[i.(T)]
		return ok
	case V:
		for _, v := range o.m {
			if reflect.DeepEqual(i, v) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (o *OrderedMap[T, V]) Filter(f func(T, V) (bool, error)) *OrderedMap[T, V] {
	o.lock.Lock()
	defer o.lock.Unlock()

	m := make(map[T]V)
	k := make([]T, 0)
	for _, key := range o.keyChain {
		v, ok := o.m[key]
		if !ok {
			continue
		}
		ok, err := f(key, v)
		if err != nil {
			break
		}
		if ok {
			m[key] = v
			k = append(k, key)
		}
	}
	return &OrderedMap[T, V]{
		lock:     new(sync.RWMutex),
		m:        m,
		keyChain: k,
	}
}

func (o *OrderedMap[T, V]) Map(f func(T, V) (T, V, error)) *OrderedMap[T, V] {
	o.lock.Lock()
	defer o.lock.Unlock()

	m := make(map[T]V)
	k := make([]T, 0)
	for _, key := range o.keyChain {
		v, ok := o.m[key]
		if !ok {
			continue
		}
		nk, nv, err := f(key, v)
		if err != nil {
			break
		}
		m[nk] = nv
		k = append(k, nk)
	}
	return &OrderedMap[T, V]{
		lock:     new(sync.RWMutex),
		m:        m,
		keyChain: k,
	}
}

func (o *OrderedMap[T, V]) Flat(f func(T, V) (struct {
	Key   T
	Value V
}, error)) *OrderedMap[T, V] {
	o.lock.Lock()
	defer o.lock.Unlock()

	m := make(map[T]V)
	k := make([]T, 0)
	for _, key := range o.keyChain {
		v, ok := o.m[key]
		if !ok {
			continue
		}
		n, err := f(key, v)
		if err != nil {
			break
		}
		m[n.Key] = n.Value
		k = append(k, n.Key)
	}
	return &OrderedMap[T, V]{
		lock:     new(sync.RWMutex),
		m:        m,
		keyChain: k,
	}
}
