package limitedmap

import (
	"github.com/yaklang/yaklang/common/utils"
	"sync"
)

type SafeMap[T any] struct {
	parent *SafeMap[T]
	m      map[string]T
	lock   *sync.RWMutex
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

func (sm *SafeMap[T]) Link(l map[string]T) *SafeMap[T] {
	if utils.IsNil(l) {
		return sm
	}

	sm.lock.RLock()
	defer sm.lock.RUnlock()
	return &SafeMap[T]{
		parent: sm,
		m:      l,
		lock:   sm.lock,
	}
}

func (sm *SafeMap[T]) Load(key string) (value T, ok bool) {
	sm.lock.RLock()
	defer sm.lock.RUnlock()
	value, ok = sm.m[key]
	if ok {
		return
	}
	if sm.parent != nil {
		value, ok = sm.parent.Load(key)
		return
	}
	return
}

func (sm *SafeMap[T]) ForEach(h func(m *SafeMap[T], key string, value T) error) {
	sm.lock.RLock()
	defer sm.lock.RUnlock()

	visited := map[string]struct{}{}
	wrapper := func(m *SafeMap[T], key string, value T) error {
		_, existed := visited[key]
		if existed {
			return nil
		}
		visited[key] = struct{}{}
		return h(m, key, value)
	}
	sm.foreach(wrapper)
}

func (sm *SafeMap[T]) foreach(h func(m *SafeMap[T], key string, value T) error) {
	for k, v := range sm.m {
		if err := h(sm, k, v); err != nil {
			return
		}
	}
	if sm.parent != nil {
		sm.parent.foreach(h)
	}
}
