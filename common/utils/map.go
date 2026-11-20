package utils

import "sync"

// safeMapCore holds the non-generic implementation shared by every
// SafeMapWithKey[K, V] instantiation, preventing per-shape code duplication and
// .deferwrap closures across generic instantiations.
type safeMapCore struct {
	mu sync.RWMutex
	m  map[any]any
}

func (c *safeMapCore) get(key any) (any, bool) {
	c.mu.RLock()
	val, ok := c.m[key]
	c.mu.RUnlock()
	return val, ok
}

func (c *safeMapCore) getOrLoad(key any, f func() (any, error)) (any, error) {
	c.mu.RLock()
	if val, ok := c.m[key]; ok {
		c.mu.RUnlock()
		return val, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if val, ok := c.m[key]; ok {
		return val, nil
	}
	val, err := f()
	if err != nil {
		return nil, err
	}
	c.m[key] = val
	return val, nil
}

func (c *safeMapCore) have(key any) bool {
	c.mu.RLock()
	_, ok := c.m[key]
	c.mu.RUnlock()
	return ok
}

func (c *safeMapCore) set(key, value any) {
	c.mu.Lock()
	c.m[key] = value
	c.mu.Unlock()
}

func (c *safeMapCore) update(key any, fn func(old any, loaded bool) (next any, store bool)) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prev, loaded := c.m[key]
	next, store := fn(prev, loaded)
	if store {
		c.m[key] = next
	}
	return prev, loaded
}

func (c *safeMapCore) updateAction(key any, fn func(old any, loaded bool) (next any, action SafeMapAction)) (any, bool, SafeMapAction) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prev, loaded := c.m[key]
	next, action := fn(prev, loaded)
	switch action {
	case SafeMapStore:
		c.m[key] = next
	case SafeMapDelete:
		delete(c.m, key)
	default:
		action = SafeMapKeep
	}
	return prev, loaded, action
}

func (c *safeMapCore) deleteKey(key any) {
	c.mu.Lock()
	delete(c.m, key)
	c.mu.Unlock()
}

func (c *safeMapCore) snapshotPairs() ([]any, []any) {
	c.mu.RLock()
	keys := make([]any, 0, len(c.m))
	values := make([]any, 0, len(c.m))
	for k, v := range c.m {
		keys = append(keys, k)
		values = append(values, v)
	}
	c.mu.RUnlock()
	return keys, values
}

func (c *safeMapCore) valuesMatching(match func(any) bool) []any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var values []any
	for k, v := range c.m {
		if match(k) {
			values = append(values, v)
		}
	}
	return values
}

func (c *safeMapCore) count() int {
	c.mu.RLock()
	n := len(c.m)
	c.mu.RUnlock()
	return n
}

func (c *safeMapCore) clear() {
	c.mu.Lock()
	c.m = make(map[any]any)
	c.mu.Unlock()
}

type SafeMapWithKey[K comparable, V any] struct {
	core safeMapCore
}

type SafeMap[V any] struct {
	core safeMapCore
}

func NewSafeMap[V any]() *SafeMap[V] {
	return &SafeMap[V]{
		core: safeMapCore{m: make(map[any]any)},
	}
}

func (sm *SafeMap[V]) Get(key string) (V, bool) {
	if sm == nil {
		var zero V
		return zero, false
	}
	raw, ok := sm.core.get(key)
	if !ok {
		var zero V
		return zero, false
	}
	return castCacheValue[V](raw), true
}

func (sm *SafeMap[V]) GetOrLoad(key string, f func() (V, error)) (V, error) {
	raw, err := sm.core.getOrLoad(key, func() (any, error) {
		return f()
	})
	if err != nil {
		var zero V
		return zero, err
	}
	return castCacheValue[V](raw), nil
}

func (sm *SafeMap[V]) Have(key string) bool     { return sm.core.have(key) }
func (sm *SafeMap[V]) Set(key string, value V) { sm.core.set(key, value) }

func (sm *SafeMap[V]) Update(key string, fn func(old V, loaded bool) (next V, store bool)) (prev V, loaded bool) {
	if sm == nil || fn == nil {
		var zero V
		return zero, false
	}
	rawPrev, loaded := sm.core.update(key, func(old any, loaded bool) (any, bool) {
		return fn(castCacheValue[V](old), loaded)
	})
	return castCacheValue[V](rawPrev), loaded
}

func (sm *SafeMap[V]) UpdateAction(key string, fn func(old V, loaded bool) (next V, action SafeMapAction)) (prev V, loaded bool, action SafeMapAction) {
	if sm == nil || fn == nil {
		var zero V
		return zero, false, SafeMapKeep
	}
	rawPrev, loaded, action := sm.core.updateAction(key, func(old any, loaded bool) (any, SafeMapAction) {
		return fn(castCacheValue[V](old), loaded)
	})
	return castCacheValue[V](rawPrev), loaded, action
}

func (sm *SafeMap[V]) Keys() []string {
	if sm == nil {
		return nil
	}
	rawKeys, _ := sm.core.snapshotPairs()
	keys := make([]string, len(rawKeys))
	for i, k := range rawKeys {
		keys[i] = castCacheValue[string](k)
	}
	return keys
}

func (sm *SafeMap[V]) Values() []V {
	if sm == nil {
		return nil
	}
	_, rawVals := sm.core.snapshotPairs()
	values := make([]V, len(rawVals))
	for i, v := range rawVals {
		values[i] = castCacheValue[V](v)
	}
	return values
}

func (sm *SafeMap[V]) Delete(key string) {
	if sm == nil {
		return
	}
	sm.core.deleteKey(key)
}

func (sm *SafeMap[V]) ForEach(f func(key string, value V) bool) {
	if sm == nil || f == nil {
		return
	}
	rawKeys, rawVals := sm.core.snapshotPairs()
	for i, k := range rawKeys {
		if !f(castCacheValue[string](k), castCacheValue[V](rawVals[i])) {
			break
		}
	}
}

func (sm *SafeMap[V]) ValuesMatching(match func(string) bool) []V {
	if sm == nil || match == nil {
		return nil
	}
	rawVals := sm.core.valuesMatching(func(k any) bool {
		return match(castCacheValue[string](k))
	})
	if len(rawVals) == 0 {
		return nil
	}
	values := make([]V, len(rawVals))
	for i, v := range rawVals {
		values[i] = castCacheValue[V](v)
	}
	return values
}

func (sm *SafeMap[V]) GetAll() map[string]V {
	if sm == nil {
		return nil
	}
	rawKeys, rawVals := sm.core.snapshotPairs()
	ret := make(map[string]V, len(rawKeys))
	for i, k := range rawKeys {
		ret[castCacheValue[string](k)] = castCacheValue[V](rawVals[i])
	}
	return ret
}

func (sm *SafeMap[V]) Count() int {
	if sm == nil {
		return 0
	}
	return sm.core.count()
}

func (sm *SafeMap[V]) Clear() {
	sm.core.clear()
}

func NewSafeMapWithKey[K comparable, V any]() *SafeMapWithKey[K, V] {
	return &SafeMapWithKey[K, V]{
		core: safeMapCore{m: make(map[any]any)},
	}
}

func (sm *SafeMapWithKey[K, V]) Get(key K) (V, bool) {
	if sm == nil {
		var zero V
		return zero, false
	}
	raw, ok := sm.core.get(key)
	if !ok {
		var zero V
		return zero, false
	}
	return castCacheValue[V](raw), true
}

func (sm *SafeMapWithKey[K, V]) GetOrLoad(key K, f func() (V, error)) (V, error) {
	raw, err := sm.core.getOrLoad(key, func() (any, error) {
		return f()
	})
	if err != nil {
		var zero V
		return zero, err
	}
	return castCacheValue[V](raw), nil
}

func (sm *SafeMapWithKey[K, V]) Have(key K) bool {
	return sm.core.have(key)
}

func (sm *SafeMapWithKey[K, V]) Set(key K, value V) {
	sm.core.set(key, value)
}

// Update runs fn while holding the write lock, so a read-and-replace of one
// key is atomic. store false leaves the map unchanged.
func (sm *SafeMapWithKey[K, V]) Update(key K, fn func(old V, loaded bool) (next V, store bool)) (prev V, loaded bool) {
	if sm == nil || fn == nil {
		var zero V
		return zero, false
	}
	rawPrev, loaded := sm.core.update(key, func(old any, loaded bool) (any, bool) {
		return fn(castCacheValue[V](old), loaded)
	})
	return castCacheValue[V](rawPrev), loaded
}

type SafeMapAction int

const (
	SafeMapKeep SafeMapAction = iota
	SafeMapStore
	SafeMapDelete
)

// UpdateAction runs fn while holding the write lock. Store replaces the key,
// Delete removes it, and Keep leaves it unchanged. The returned value is the
// value present before fn runs.
func (sm *SafeMapWithKey[K, V]) UpdateAction(key K, fn func(old V, loaded bool) (next V, action SafeMapAction)) (prev V, loaded bool, action SafeMapAction) {
	if sm == nil || fn == nil {
		var zero V
		return zero, false, SafeMapKeep
	}
	rawPrev, loaded, action := sm.core.updateAction(key, func(old any, loaded bool) (any, SafeMapAction) {
		return fn(castCacheValue[V](old), loaded)
	})
	return castCacheValue[V](rawPrev), loaded, action
}

func (sm *SafeMapWithKey[K, V]) Keys() []K {
	if sm == nil {
		return nil
	}
	rawKeys, _ := sm.core.snapshotPairs()
	keys := make([]K, len(rawKeys))
	for i, k := range rawKeys {
		keys[i] = castCacheValue[K](k)
	}
	return keys
}

func (sm *SafeMapWithKey[K, V]) Values() []V {
	if sm == nil {
		return nil
	}
	_, rawVals := sm.core.snapshotPairs()
	values := make([]V, len(rawVals))
	for i, v := range rawVals {
		values[i] = castCacheValue[V](v)
	}
	return values
}

func (sm *SafeMapWithKey[K, V]) Delete(key K) {
	if sm == nil {
		return
	}
	sm.core.deleteKey(key)
}

func (sm *SafeMapWithKey[K, V]) ForEach(f func(key K, value V) bool) {
	if sm == nil || f == nil {
		return
	}
	rawKeys, rawVals := sm.core.snapshotPairs()
	for i, k := range rawKeys {
		if !f(castCacheValue[K](k), castCacheValue[V](rawVals[i])) {
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
	rawVals := sm.core.valuesMatching(func(k any) bool {
		return match(castCacheValue[K](k))
	})
	if len(rawVals) == 0 {
		return nil
	}
	values := make([]V, len(rawVals))
	for i, v := range rawVals {
		values[i] = castCacheValue[V](v)
	}
	return values
}

func (sm *SafeMapWithKey[K, V]) GetAll() map[K]V {
	if sm == nil {
		return nil
	}
	rawKeys, rawVals := sm.core.snapshotPairs()
	ret := make(map[K]V, len(rawKeys))
	for i, k := range rawKeys {
		ret[castCacheValue[K](k)] = castCacheValue[V](rawVals[i])
	}
	return ret
}

func (sm *SafeMapWithKey[K, V]) Count() int {
	if sm == nil {
		return 0
	}
	return sm.core.count()
}

func (sm *SafeMapWithKey[K, V]) Clear() {
	sm.core.clear()
}
