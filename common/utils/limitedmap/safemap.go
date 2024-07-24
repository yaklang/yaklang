package limitedmap

import (
	"github.com/yaklang/yaklang/common/utils"
	"sync"
)

type SafeMap[T any] struct {
	m    map[string]T
	lock *sync.RWMutex
}

func NewSafeMap[T any](a map[string]T) *SafeMap[T] {
	if utils.IsNil(a) {
		a = make(map[string]T)
	}
	return &SafeMap[T]{
		m:    a,
		lock: new(sync.RWMutex),
	}
}

func (sm *SafeMap[T]) Append(l map[string]T) *SafeMap[T] {
	if utils.IsNil(l) {
		return sm
	}

	sm.lock.RLock()
	defer sm.lock.RUnlock()

	res := make(map[string]T)

	// Function to merge maps
	mergeMaps := func(src map[string]T) {
		for k, vT := range src {
			existedRaw, ok := res[k]
			if ok {
				var existed any = existedRaw
				lib, ok := existed.(map[string]T)
				if ok {
					var v any = vT
					if newLib, ok := v.(map[string]T); ok {
						for k1, v1 := range newLib {
							lib[k1] = v1
						}
						continue
					}
				}
			}
			res[k] = vT
		}
	}

	// Merge all maps in the SafeMap chain
	origin := sm
	for origin != nil {
		mergeMaps(origin.m)
	}

	// Merge the new map
	mergeMaps(l)

	return &SafeMap[T]{
		m:    res,
		lock: sm.lock,
	}
}

func (sm *SafeMap[T]) Load(key string) (value T, ok bool) {
	sm.lock.RLock()
	defer sm.lock.RUnlock()
	value, ok = sm.m[key]
	if ok {
		return
	}
	return
}

func (sm *SafeMap[T]) ForEach(h func(m *SafeMap[T], key string, value T) error) {
	sm.lock.RLock()
	defer sm.lock.RUnlock()

	for k, v := range sm.m {
		if err := h(sm, k, v); err != nil {
			return
		}
	}
}

func (sm *SafeMap[T]) Raw() map[string]T {
	return sm.m
}
