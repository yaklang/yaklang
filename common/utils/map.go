package utils

import "sync"

type SafeMapWithKey[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

type SafeMap[V any] struct {
	*SafeMapWithKey[string, V]
}

func NewSafeMap[V any]() *SafeMap[V] {
	return &SafeMap[V]{
		SafeMapWithKey: NewSafeMapWithKey[string, V](),
	}
}

func NewSafeMapWithKey[K comparable, V any]() *SafeMapWithKey[K, V] {
	return &SafeMapWithKey[K, V]{
		m: make(map[K]V),
	}
}

func (sm *SafeMapWithKey[K, V]) Get(key K) (V, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	val, ok := sm.m[key]
	return val, ok
}

func (sm *SafeMapWithKey[K, V]) Have(key K) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	_, ok := sm.m[key]
	return ok
}

func (sm *SafeMapWithKey[K, V]) Set(key K, value V) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.m[key] = value
}

func (sm *SafeMapWithKey[K, V]) Delete(key K) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.m, key)
}

func (sm *SafeMapWithKey[K, V]) ForEach(f func(key K, value V) bool) {
	sm.mu.RLock()
	keys := make([]K, 0, len(sm.m))
	values := make([]V, 0, len(sm.m))
	for k, v := range sm.m {
		keys = append(keys, k)
		values = append(values, v)
	}
	sm.mu.RUnlock()

	for i, k := range keys {
		if !f(k, values[i]) {
			break
		}
	}
}

func (sm *SafeMapWithKey[K, V]) Count() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.m)
}

func (sm *SafeMapWithKey[K, V]) Clear() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.m = make(map[K]V)
}
