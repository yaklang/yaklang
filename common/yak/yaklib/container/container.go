package container

import (
	"container/list"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/yaklang/yaklang/common/log"
)

type Set struct {
	mapset.Set[any]
}

// NewSet creates a new set
// Example:
// ```
// s = container.NewSet("1", "2")
// ```
func NewSet(vals ...any) (s *Set) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("NewSet panic: %v", err)
			s = nil
		}
	}()
	newSet := mapset.NewSet(vals...)
	return &Set{
		Set: newSet,
	}
}

// NewUnsafeSet creates a new set that is not thread-safe
// Example:
// ```
// s = container.NewUnsafeSet("1", "2")
// ```
func NewUnsafeSet(vals ...any) (s *Set) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("NewSet panic: %v", err)
			s = nil
		}
	}()
	newSet := mapset.NewThreadUnsafeSet(vals...)
	return &Set{
		Set: newSet,
	}
}

// Add add an element to set
// Example:
// ```
// s = container.NewSet()
// assert s.Add("1") == true
// ```
func (s *Set) Add(val any) bool {
	return s.Set.Add(val)
}

// Append add multiple elements to set
// Example:
// ```
// assert s.Append("1", "2", "3") == 3
// ```
func (s *Set) Append(vals ...any) int {
	return s.Set.Append(vals...)
}

// Len returns the number of elements in the set
// Example:
// ```
// s = container.NewSet("1", "2")
// assert s.Len() == 2
// ```
func (s *Set) Len() int {
	return s.Cardinality()
}

// Cap returns the number of elements in the set, same as Len
// Example:
// ```
// s = container.NewSet("1", "2")
// assert s.Cap() == 2
// ```
func (s *Set) Cap() int {
	return s.Set.Cardinality()
}

// Cardinality returns the number of elements in the set
// Example:
// ```
// s = container.NewSet("1", "2")
// assert s.Cardinality() == 2
// ```
func (s *Set) Cardinality() int {
	return s.Set.Cardinality()
}

// Clear removes all elements from the set
// Example:
// ```
// s = container.NewSet("1", "2")
// s.Clear()
// assert s.Len() == 0
// ```
func (s *Set) Clear() {
	s.Set.Clear()
}

// Clone returns a new set with the same elements as the original set
// Example:
// ```
// s = container.NewSet("1", "2")
// s2 = s.Clone()
// assert s2.Equal(s)
// ```
func (s *Set) Clone() *Set {
	return &Set{Set: s.Set.Clone()}
}

// Contains checks if the set contains the given elements
// Example:
// ```
// s = container.NewSet("1", "2")
// assert s.Contains("1", "2") == true
// assert s.Contains("3") == false
// ```
func (s *Set) Contains(vals ...any) bool {
	return s.Set.Contains(vals...)
}

// ContainsAny checks if the set contains any of the given elements
// Example:
// ```
// s = container.NewSet("1", "2")
// assert s.ContainsAny("2", "3") == true
// assert s.ContainsAny("1") == true
// assert s.ContainsAny("3", "4") == false
// ```
func (s *Set) ContainsAny(vals ...any) bool {
	return s.Set.ContainsAny(vals...)
}

// ContainsOne checks if the set contains the given element
// Example:
// ```
// s = container.NewSet("1", "2")
// assert s.ContainsOne("1") == true
// assert s.ContainsOne("3") == false
// ```
func (s *Set) ContainsOne(val any) bool {
	return s.Set.ContainsOne(val)
}

// Difference returns a new set with the elements that are in the original set but not in the other set
// Example:
// ```
// s = container.NewSet("1", "2")
// s2 = container.NewSet("2", "3")
// s3 = s.Difference(s2)  // ["1"]
// ```
func (s *Set) Difference(other *Set) *Set {
	return &Set{Set: s.Set.Difference(other.Set)}
}

// Each applies the given function to each element in the set
// Example:
// ```
// s = container.NewSet("1", "2")
// s.Each(func(val) {
// println(val)
// })
// ```
func (s *Set) Each(f func(any)) {
	s.Set.Each(func(v any) bool {
		f(v)
		return false
	})
}

// Equal checks if the set is equal to another set
// Example:
// ```
// s = container.NewSet("1", "2")
// s2 = container.NewSet("2", "1")
// s3 = container.NewSet("1", "2", "3")
// assert s.Equal(s2) == true
// assert s.Equal(s3) == false
// ```
func (s *Set) Equal(other *Set) bool {
	return s.Set.Equal(other.Set)
}

// IsEmpty checks if the set is empty
// Example:
// ```
// s = container.NewSet()
// assert s.IsEmpty() == true
// s2 = container.NewSet("1")
// assert s2.IsEmpty() == false
// ```
func (s *Set) IsEmpty() bool {
	return s.Set.IsEmpty()
}

// Iter returns a channel that can be used to iterate over the elements in the set
// Example:
// ```
// s = container.NewSet("1", "2")
// for val = range s.Iter() {
// println(val)
// }
// ```
func (s *Set) Iter() <-chan any {
	return s.Set.Iter()
}

// Iterator returns a channel that can be used to iterate over the elements in the set
// Example:
// ```
// s = container.NewSet("1", "2")
// for val = range s.Iterator() {
// println(val)
// }
// ```
func (s *Set) Iterator() <-chan any {
	return s.Set.Iter()
}

// Intersect returns a new set with the elements that are in both sets
// Example:
// ```
// s = container.NewSet("1", "2")
// s2 = container.NewSet("2", "3")
// s3 = s.Intersect(s2)  // ["2"]
// ```
func (s *Set) Intersect(other *Set) *Set {
	return &Set{Set: s.Set.Intersect(other.Set)}
}

// IsProperSubset checks if the set is a proper subset of another set
// Example:
// ```
// s = container.NewSet("1", "2")
// s2 = container.NewSet("1", "2", "3")
// assert s.IsProperSubset(s2) == true
// assert s.IsProperSubset(s) == false
// ```
func (s *Set) IsProperSubset(other *Set) bool {
	return s.Set.IsProperSubset(other.Set)
}

// IsProperSuperset checks if the set is a proper superset of another set
// Example:
// ```
// s = container.NewSet("1", "2")
// s2 = container.NewSet("1", "2", "3")
// assert s.IsProperSuperset(s2) == false
// assert s.IsProperSuperset(s) == false
// assert s2.IsProperSuperset(s) == true
// ```
func (s *Set) IsProperSuperset(other *Set) bool {
	return s.Set.IsProperSuperset(other.Set)
}

// IsSubset checks if the set is a subset of another set
// Example:
// ```
// s = container.NewSet("1", "2")
// s2 = container.NewSet("1", "2", "3")
// s3 = container.NewSet("2", "3")
// assert s.IsSubset(s2) == true
// assert s.IsSubset(s) == true
// assert s.IsSubset(s3) == false
// ```
func (s *Set) IsSubset(other *Set) bool {
	return s.Set.IsSubset(other.Set)
}

// IsSuperset checks if the set is a superset of another set
// Example:
// ```
// s = container.NewSet("1", "2")
// s2 = container.NewSet("1", "2", "3")
// assert s.IsSuperset(s2) == false
// assert s.IsSuperset(s) == true
// assert s2.IsSuperset(s) == true
// ```
func (s *Set) IsSuperset(other *Set) bool {
	return s.Set.IsSuperset(other.Set)
}

// Union returns a new set with the elements that are in either set
// Example:
// ```
// s = container.NewSet("1", "2")
// s2 = container.NewSet("2", "3")
// s3 = s.Union(s2) // ["1", "2", "3"]
// ```
func (s *Set) Union(other *Set) *Set {
	return &Set{Set: s.Set.Union(other.Set)}
}

// SymmetricDifference returns a new set with the elements that are in either set but not in both
// Example:
// ```
// s = container.NewSet("1", "2")
// s2 = container.NewSet("2", "3")
// s3 = s.SymmetricDifference(s2)  // ["1", "3"]
// ```
func (s *Set) SymmetricDifference(other *Set) *Set {
	return &Set{Set: s.Set.SymmetricDifference(other.Set)}
}

func (s *Set) MarshalJSON() ([]byte, error) {
	return s.Set.MarshalJSON()
}

func (s *Set) UnmarshalJSON(data []byte) error {
	return s.Set.UnmarshalJSON(data)
}

// Pop removes and returns an arbitrary element from the set
// Example:
// ```
// s = container.NewSet("1", "2")
// v, ok = s.Pop()
// assert ok == true
// // v maybe 1 or 2
// ```
func (s *Set) Pop() (any, bool) {
	return s.Set.Pop()
}

// Remove removes the given element from the set
// Example:
// ```
// s = container.NewSet("1", "2")
// s.Remove("1")
// assert s.Len() == 1
// ```
func (s *Set) Remove(i any) {
	s.Set.Remove(i)
}

// RemoveAll removes the given elements from the set
// Example:
// ```
// s = container.NewSet("1", "2")
// s.RemoveAll("1", "2")
// assert s.Len() == 0
// ```
func (s *Set) RemoveAll(i ...any) {
	s.Set.RemoveAll(i...)
}

// ToSlice returns the elements of the set as a slice
// Example:
// ```
// s = container.NewSet("1", "2")
// s.ToSlice() // ["1", "2"]
// ```
func (s *Set) ToSlice() []any {
	return s.Set.ToSlice()
}

type LinkedList struct {
	*list.List
}

func NewLinkedList() *LinkedList {
	return &LinkedList{List: list.New()}
}

func (l *LinkedList) ToSlice() []any {
	slice := make([]any, 0, l.List.Len())
	for e := l.List.Front(); e != nil; e = e.Next() {
		slice = append(slice, e.Value)
	}
	return slice
}

var ContainerExports = map[string]interface{}{
	"NewSet":        NewSet,
	"NewUnsafeSet":  NewUnsafeSet,
	"NewLinkedList": NewLinkedList,
}
