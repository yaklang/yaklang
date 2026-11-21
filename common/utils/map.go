package utils

import "sync"

type SafeMapAction int

const (
	SafeMapKeep SafeMapAction = iota
	SafeMapStore
	SafeMapDelete
)

// SafeMapWithKey is a concurrency-safe generic map. Keys and values are stored
// directly in map[K]V to avoid boxing allocations on hot paths.
type SafeMapWithKey[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

// SafeMap is a string-keyed concurrency-safe map. Keeping its own fields
// instead of embedding *SafeMapWithKey[string, V] avoids promoted value- and
// pointer-receiver wrapper bloat on every generic instantiation.
type SafeMap[V any] struct {
	mu sync.RWMutex
	m  map[string]V
}

func NewSafeMap[V any]() *SafeMap[V] {
	return &SafeMap[V]{
		m: make(map[string]V),
	}
}

func NewSafeMapWithKey[K comparable, V any]() *SafeMapWithKey[K, V] {
	return &SafeMapWithKey[K, V]{
		m: make(map[K]V),
	}
}

func (sm *SafeMapWithKey[K, V]) Get(key K) (V, bool) {
	if sm == nil {
		var zero V
		return zero, false
	}
	sm.mu.RLock()
	val, ok := sm.m[key]
	sm.mu.RUnlock()
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
	_, ok := sm.m[key]
	sm.mu.RUnlock()
	return ok
}

func (sm *SafeMapWithKey[K, V]) Set(key K, value V) {
	sm.mu.Lock()
	sm.m[key] = value
	sm.mu.Unlock()
}

// Update runs fn while holding the write lock, so a read-and-replace of one
// key is atomic. store false leaves the map unchanged.
func (sm *SafeMapWithKey[K, V]) Update(key K, fn func(old V, loaded bool) (next V, store bool)) (prev V, loaded bool) {
	if sm == nil || fn == nil {
		var zero V
		return zero, false
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	prev, loaded = sm.m[key]
	next, store := fn(prev, loaded)
	if store {
		sm.m[key] = next
	}
	return prev, loaded
}

// UpdateAction runs fn while holding the write lock. Store replaces the key,
// Delete removes it, and Keep leaves it unchanged. The returned value is the
// value present before fn runs.
func (sm *SafeMapWithKey[K, V]) UpdateAction(key K, fn func(old V, loaded bool) (next V, action SafeMapAction)) (prev V, loaded bool, action SafeMapAction) {
	if sm == nil || fn == nil {
		var zero V
		return zero, false, SafeMapKeep
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	prev, loaded = sm.m[key]
	next, action := fn(prev, loaded)
	switch action {
	case SafeMapStore:
		sm.m[key] = next
	case SafeMapDelete:
		delete(sm.m, key)
	default:
		action = SafeMapKeep
	}
	return prev, loaded, action
}

func (sm *SafeMapWithKey[K, V]) Keys() []K {
	if sm == nil {
		return nil
	}
	sm.mu.RLock()
	keys := make([]K, 0, len(sm.m))
	for k := range sm.m {
		keys = append(keys, k)
	}
	sm.mu.RUnlock()
	return keys
}

func (sm *SafeMapWithKey[K, V]) Values() []V {
	if sm == nil {
		return nil
	}
	sm.mu.RLock()
	values := make([]V, 0, len(sm.m))
	for _, v := range sm.m {
		values = append(values, v)
	}
	sm.mu.RUnlock()
	return values
}

func (sm *SafeMapWithKey[K, V]) Delete(key K) {
	if sm == nil {
		return
	}
	sm.mu.Lock()
	delete(sm.m, key)
	sm.mu.Unlock()
}

func (sm *SafeMapWithKey[K, V]) ForEach(f func(key K, value V) bool) {
	if sm == nil || f == nil {
		return
	}
	sm.mu.RLock()
	keys := make([]K, 0, len(sm.m))
	vals := make([]V, 0, len(sm.m))
	for k, v := range sm.m {
		keys = append(keys, k)
		vals = append(vals, v)
	}
	sm.mu.RUnlock()
	for i, k := range keys {
		if !f(k, vals[i]) {
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
	for k, v := range sm.m {
		if match(k) {
			values = append(values, v)
		}
	}
	return values
}

func (sm *SafeMapWithKey[K, V]) GetAll() map[K]V {
	if sm == nil {
		return nil
	}
	sm.mu.RLock()
	ret := make(map[K]V, len(sm.m))
	for k, v := range sm.m {
		ret[k] = v
	}
	sm.mu.RUnlock()
	return ret
}

func (sm *SafeMapWithKey[K, V]) Count() int {
	if sm == nil {
		return 0
	}
	sm.mu.RLock()
	n := len(sm.m)
	sm.mu.RUnlock()
	return n
}

func (sm *SafeMapWithKey[K, V]) Clear() {
	sm.mu.Lock()
	sm.m = make(map[K]V)
	sm.mu.Unlock()
}

func (sm *SafeMap[V]) Get(key string) (V, bool) {
	if sm == nil {
		var zero V
		return zero, false
	}
	sm.mu.RLock()
	val, ok := sm.m[key]
	sm.mu.RUnlock()
	return val, ok
}

func (sm *SafeMap[V]) GetOrLoad(key string, f func() (V, error)) (V, error) {
	sm.mu.RLock()
	if val, ok := sm.m[key]; ok {
		sm.mu.RUnlock()
		return val, nil
	}
	sm.mu.RUnlock()

	sm.mu.Lock()
	defer sm.mu.Unlock()
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

func (sm *SafeMap[V]) Have(key string) bool {
	sm.mu.RLock()
	_, ok := sm.m[key]
	sm.mu.RUnlock()
	return ok
}

func (sm *SafeMap[V]) Set(key string, value V) {
	sm.mu.Lock()
	sm.m[key] = value
	sm.mu.Unlock()
}

func (sm *SafeMap[V]) Update(key string, fn func(old V, loaded bool) (next V, store bool)) (prev V, loaded bool) {
	if sm == nil || fn == nil {
		var zero V
		return zero, false
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	prev, loaded = sm.m[key]
	next, store := fn(prev, loaded)
	if store {
		sm.m[key] = next
	}
	return prev, loaded
}

func (sm *SafeMap[V]) UpdateAction(key string, fn func(old V, loaded bool) (next V, action SafeMapAction)) (prev V, loaded bool, action SafeMapAction) {
	if sm == nil || fn == nil {
		var zero V
		return zero, false, SafeMapKeep
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	prev, loaded = sm.m[key]
	next, action := fn(prev, loaded)
	switch action {
	case SafeMapStore:
		sm.m[key] = next
	case SafeMapDelete:
		delete(sm.m, key)
	default:
		action = SafeMapKeep
	}
	return prev, loaded, action
}

func (sm *SafeMap[V]) Keys() []string {
	if sm == nil {
		return nil
	}
	sm.mu.RLock()
	keys := make([]string, 0, len(sm.m))
	for k := range sm.m {
		keys = append(keys, k)
	}
	sm.mu.RUnlock()
	return keys
}

func (sm *SafeMap[V]) Values() []V {
	if sm == nil {
		return nil
	}
	sm.mu.RLock()
	values := make([]V, 0, len(sm.m))
	for _, v := range sm.m {
		values = append(values, v)
	}
	sm.mu.RUnlock()
	return values
}

func (sm *SafeMap[V]) Delete(key string) {
	if sm == nil {
		return
	}
	sm.mu.Lock()
	delete(sm.m, key)
	sm.mu.Unlock()
}

func (sm *SafeMap[V]) ForEach(f func(key string, value V) bool) {
	if sm == nil || f == nil {
		return
	}
	sm.mu.RLock()
	keys := make([]string, 0, len(sm.m))
	vals := make([]V, 0, len(sm.m))
	for k, v := range sm.m {
		keys = append(keys, k)
		vals = append(vals, v)
	}
	sm.mu.RUnlock()
	for i, k := range keys {
		if !f(k, vals[i]) {
			break
		}
	}
}

func (sm *SafeMap[V]) ValuesMatching(match func(string) bool) []V {
	if sm == nil || match == nil {
		return nil
	}
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	var values []V
	for k, v := range sm.m {
		if match(k) {
			values = append(values, v)
		}
	}
	return values
}

func (sm *SafeMap[V]) GetAll() map[string]V {
	if sm == nil {
		return nil
	}
	sm.mu.RLock()
	ret := make(map[string]V, len(sm.m))
	for k, v := range sm.m {
		ret[k] = v
	}
	sm.mu.RUnlock()
	return ret
}

func (sm *SafeMap[V]) Count() int {
	if sm == nil {
		return 0
	}
	sm.mu.RLock()
	n := len(sm.m)
	sm.mu.RUnlock()
	return n
}

func (sm *SafeMap[V]) Clear() {
	sm.mu.Lock()
	sm.m = make(map[string]V)
	sm.mu.Unlock()
}
