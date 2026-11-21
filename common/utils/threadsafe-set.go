package utils

import (
	"sync"
)

type Set[T comparable] struct {
	m  map[T]struct{}
	mu sync.RWMutex
}

func NewSet[T comparable](list ...[]T) *Set[T] {
	s := &Set[T]{
		m: make(map[T]struct{}),
	}
	if len(list) > 0 {
		s.AddList(list[0])
	}
	return s
}

// Add inserts item into the set.
func (s *Set[T]) Add(item T) {
	s.mu.Lock()
	s.m[item] = struct{}{}
	s.mu.Unlock()
}

func (s *Set[T]) AddList(items []T) {
	s.mu.Lock()
	for _, item := range items {
		s.m[item] = struct{}{}
	}
	s.mu.Unlock()
}

// Remove deletes the specified item from the map.
func (s *Set[T]) Remove(item T) {
	s.mu.Lock()
	delete(s.m, item)
	s.mu.Unlock()
}

// Has looks for the existence of an item.
func (s *Set[T]) Has(item T) bool {
	s.mu.RLock()
	_, ok := s.m[item]
	s.mu.RUnlock()
	return ok
}

// Len returns the number of items in a set.
func (s *Set[T]) Len() int {
	s.mu.RLock()
	n := len(s.m)
	s.mu.RUnlock()
	return n
}

// Clear removes all items from the set.
func (s *Set[T]) Clear() {
	s.mu.Lock()
	s.m = make(map[T]struct{})
	s.mu.Unlock()
}

// IsEmpty checks for emptiness.
func (s *Set[T]) IsEmpty() bool {
	return s.Len() == 0
}

// List returns a slice of all items.
func (s *Set[T]) List() []T {
	s.mu.RLock()
	list := make([]T, 0, len(s.m))
	for item := range s.m {
		list = append(list, item)
	}
	s.mu.RUnlock()
	return list
}

func (s *Set[T]) ForEach(h func(T)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for item := range s.m {
		h(item)
	}
}

func (s *Set[T]) Diff(other *Set[T]) *Set[T] {
	diff := NewSet[T]()
	if s == other {
		return diff
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if other == nil {
		for item := range s.m {
			diff.m[item] = struct{}{}
		}
		return diff
	}
	other.mu.RLock()
	defer other.mu.RUnlock()
	for item := range s.m {
		if _, ok := other.m[item]; !ok {
			diff.m[item] = struct{}{}
		}
	}
	for item := range other.m {
		if _, ok := s.m[item]; !ok {
			diff.m[item] = struct{}{}
		}
	}
	return diff
}

func (s *Set[T]) And(other *Set[T]) *Set[T] {
	intersection := NewSet[T]()
	if other == nil {
		return intersection
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s == other {
		for item := range s.m {
			intersection.m[item] = struct{}{}
		}
		return intersection
	}
	other.mu.RLock()
	defer other.mu.RUnlock()
	for item := range s.m {
		if _, ok := other.m[item]; ok {
			intersection.m[item] = struct{}{}
		}
	}
	return intersection
}

func (s *Set[T]) Or(other *Set[T]) *Set[T] {
	union := NewSet[T]()
	s.mu.RLock()
	defer s.mu.RUnlock()
	for item := range s.m {
		union.m[item] = struct{}{}
	}
	if other == nil || s == other {
		return union
	}
	other.mu.RLock()
	defer other.mu.RUnlock()
	for item := range other.m {
		union.m[item] = struct{}{}
	}
	return union
}
