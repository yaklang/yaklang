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

func (sm *SafeMapWithKey[K, V]) GetOrLoad(key K, f func() (V, error)) (V, error) {
	sm.mu.RLock()
	if val, ok := sm.m[key]; ok {
		sm.mu.RUnlock()
		return val, nil
	}
	sm.mu.RUnlock()

	sm.mu.Lock()
	defer sm.mu.Unlock()
	// 3. 再次检查！
	//    (防止在等待写锁期间，已有其他协程完成了加载)
	if val, ok := sm.m[key]; ok {
		return val, nil
	}

	val, err := f()
	if err != nil {
		var zero V
		return zero, err
	}
	sm.m[key] = val
	return val, nil
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

func (sm *SafeMapWithKey[K, V]) Keys() []K {
	if sm == nil {
		return nil
	}
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	keys := make([]K, 0, len(sm.m))
	for k := range sm.m {
		keys = append(keys, k)
	}
	return keys
}

func (sm *SafeMapWithKey[K, V]) Values() []V {
	if sm == nil {
		return nil
	}
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	values := make([]V, 0, len(sm.m))
	for _, v := range sm.m {
		values = append(values, v)
	}
	return values
}

func (sm *SafeMapWithKey[K, V]) Delete(key K) {
	if sm == nil {
		return
	}
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

// ValuesMatching snapshots only values whose keys match. The predicate runs
// under the read lock and must be a pure key test. Values are copied out
// before return: the caller may re-enter this map, and a callback inside the
// lock would deadlock on that write.
func (sm *SafeMapWithKey[K, V]) ValuesMatching(match func(K) bool) []V {
	if sm == nil || match == nil {
		return nil
	}
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	var values []V
	for key, value := range sm.m {
		if match(key) {
			values = append(values, value)
		}
	}
	return values
}

func (sm *SafeMapWithKey[K, V]) GetAll() map[K]V {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	ret := make(map[K]V, len(sm.m))
	for k, v := range sm.m {
		ret[k] = v
	}
	return ret
}

func (sm *SafeMapWithKey[K, V]) Count() int {
	if sm == nil {
		return 0
	}
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.m)
}

func (sm *SafeMapWithKey[K, V]) Clear() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.m = make(map[K]V)
}
