package utils

import (
	"sync"
)

type Set struct {
	m map[string]bool
	sync.RWMutex
}

func New() *Set {
	return &Set{
		m: make(map[string]bool),
	}
}

//func main() {
//	// Initialize our Set
//	s := New()
//
//	// Add example items
//	s.Add("item1")
//	s.Add("item1") // duplicate item
//	s.Add("item2")
//	fmt.Printf("%d items\n", s.Len())
//
//	// Clear all items
//	s.Clear()
//	if s.IsEmpty() {
//		fmt.Printf("0 items\n")
//	}
//
//	s.Add("item2")
//	s.Add("item3")
//	s.Add("item4")
//
//	// Check for existence
//	if s.Has("item2") {
//		fmt.Println("item2 does exist")
//	}
//
//	// Remove some of our items
//	s.Remove("item2")
//	s.Remove("item4")
//	fmt.Println("list of all items:", s.List())
//}

// Add add
func (s *Set) Add(item string) {
	s.Lock()
	defer s.Unlock()
	s.m[item] = true
}

// Remove deletes the specified item from the map
func (s *Set) Remove(item string) {
	s.Lock()
	defer s.Unlock()
	delete(s.m, item)
}

// Has looks for the existence of an item
func (s *Set) Has(item string) bool {
	s.RLock()
	defer s.RUnlock()
	_, ok := s.m[item]
	return ok
}

// Len returns the number of items in a set.
func (s *Set) Len() int {
	return len(s.List())
}

// Clear removes all items from the set
func (s *Set) Clear() {
	s.Lock()
	defer s.Unlock()
	s.m = make(map[string]bool)
}

// IsEmpty checks for emptiness
func (s *Set) IsEmpty() bool {
	if s.Len() == 0 {
		return true
	}
	return false
}

// Set returns a slice of all items
func (s *Set) List() []string {
	s.RLock()
	defer s.RUnlock()
	list := make([]string, 0)
	for item := range s.m {
		list = append(list, item)
	}
	return list
}
