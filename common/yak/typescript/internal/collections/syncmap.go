package collections

import "sync"

type SyncMap[K comparable, V any] struct {
	m sync.Map
}

func (s *SyncMap[K, V]) Load(key K) (value V, ok bool) {
	val, ok := s.m.Load(key)
	if !ok {
		return
	}
	return val.(V), true
}

func (s *SyncMap[K, V]) Store(key K, value V) {
	s.m.Store(key, value)
}

func (s *SyncMap[K, V]) LoadOrStore(key K, value V) (actual V, loaded bool) {
	actualAny, loaded := s.m.LoadOrStore(key, value)
	return actualAny.(V), loaded
}

func (s *SyncMap[K, V]) Delete(key K) {
	s.m.Delete(key)
}

func (s *SyncMap[K, V]) Clear() {
	s.m.Clear()
}

func (s *SyncMap[K, V]) Range(f func(key K, value V) bool) {
	s.m.Range(func(key, value any) bool {
		return f(key.(K), value.(V))
	})
}

// Size returns the approximate number of items in the map.
// Note that this is not a precise count, as the map may be modified
// concurrently while this method is running.
func (s *SyncMap[K, V]) Size() int {
	count := 0
	s.m.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

func (s *SyncMap[K, V]) ToMap() map[K]V {
	m := make(map[K]V, s.Size())
	s.m.Range(func(key, value any) bool {
		m[key.(K)] = value.(V)
		return true
	})
	return m
}
