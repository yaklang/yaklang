package collections

import (
	"bytes"
	"encoding"
	"encoding/json"
	"iter"
	"maps"
	"reflect"
	"slices"
	"strconv"
)

// OrderedMap is an insertion ordered map.
type OrderedMap[K comparable, V any] struct {
	keys []K
	mp   map[K]V
}

// NewOrderedMapWithSizeHint creates a new OrderedMap with a hint for the number of elements it will contain.
func NewOrderedMapWithSizeHint[K comparable, V any](hint int) *OrderedMap[K, V] {
	m := newMapWithSizeHint[K, V](hint)
	return &m
}

func newMapWithSizeHint[K comparable, V any](hint int) OrderedMap[K, V] {
	return OrderedMap[K, V]{
		keys: make([]K, 0, hint),
		mp:   make(map[K]V, hint),
	}
}

type MapEntry[K comparable, V any] struct {
	Key   K
	Value V
}

func NewOrderedMapFromList[K comparable, V any](items []MapEntry[K, V]) *OrderedMap[K, V] {
	mp := NewOrderedMapWithSizeHint[K, V](len(items))
	for _, item := range items {
		mp.Set(item.Key, item.Value)
	}
	return mp
}

// Set sets a key-value pair in the map.
func (m *OrderedMap[K, V]) Set(key K, value V) {
	if m.mp == nil {
		m.mp = make(map[K]V)
	}

	if _, ok := m.mp[key]; !ok {
		m.keys = append(m.keys, key)
	}
	m.mp[key] = value
}

// Get retrieves a value from the map.
func (m *OrderedMap[K, V]) Get(key K) (V, bool) {
	v, ok := m.mp[key]
	return v, ok
}

// GetOrZero retrieves a value from the map, or returns the zero value of the value type if the key is not present.
func (m *OrderedMap[K, V]) GetOrZero(key K) V {
	return m.mp[key]
}

// Has returns true if the map contains the key.
func (m *OrderedMap[K, V]) Has(key K) bool {
	_, ok := m.mp[key]
	return ok
}

// Delete removes a key-value pair from the map.
func (m *OrderedMap[K, V]) Delete(key K) (V, bool) {
	v, ok := m.mp[key]
	if !ok {
		var zero V
		return zero, false
	}

	delete(m.mp, key)
	i := slices.Index(m.keys, key)
	// If we're just removing the first or last element, avoid shifting everything around.
	if i == 0 {
		var zero K
		m.keys[0] = zero
		m.keys = m.keys[1:]
	} else if end := len(m.keys) - 1; i == end {
		var zero K
		m.keys[end] = zero
		m.keys = m.keys[:end]
	} else {
		m.keys = slices.Delete(m.keys, i, i+1)
	}

	return v, true
}

// Keys returns an iterator over the keys in the map.
// A slice of the keys can be obtained by calling `slices.Collect`.
func (m *OrderedMap[K, V]) Keys() iter.Seq[K] {
	return func(yield func(K) bool) {
		if m == nil {
			return
		}

		// We use a for loop here to ensure we enumerate new items added during iteration.
		//nolint:intrange
		for i := 0; i < len(m.keys); i++ {
			if !yield(m.keys[i]) {
				break
			}
		}
	}
}

// Values returns an iterator over the values in the map.
// A slice of the values can be obtained by calling `slices.Collect`.
func (m *OrderedMap[K, V]) Values() iter.Seq[V] {
	return func(yield func(V) bool) {
		if m == nil {
			return
		}

		// We use a for loop here to ensure we enumerate new items added during iteration.
		//nolint:intrange
		for i := 0; i < len(m.keys); i++ {
			if !yield(m.mp[m.keys[i]]) {
				break
			}
		}
	}
}

// Entries returns an iterator over the key-value pairs in the map.
func (m *OrderedMap[K, V]) Entries() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		if m == nil {
			return
		}

		// We use a for loop here to ensure we enumerate new items added during iteration.
		//nolint:intrange
		for i := 0; i < len(m.keys); i++ {
			key := m.keys[i]
			if !yield(key, m.mp[key]) {
				break
			}
		}
	}
}

// Clear removes all key-value pairs from the map.
// The space allocated for the map will be reused.
func (m *OrderedMap[K, V]) Clear() {
	clear(m.keys)
	m.keys = m.keys[:0]
	clear(m.mp)
}

// Size returns the number of key-value pairs in the map.
func (m *OrderedMap[K, V]) Size() int {
	if m == nil {
		return 0
	}

	return len(m.keys)
}

// Clone returns a shallow copy of the map.
func (m *OrderedMap[K, V]) Clone() *OrderedMap[K, V] {
	if m == nil {
		return nil
	}

	m2 := m.clone()
	return &m2
}

func (m *OrderedMap[K, V]) clone() OrderedMap[K, V] {
	return OrderedMap[K, V]{
		keys: slices.Clone(m.keys),
		mp:   maps.Clone(m.mp),
	}
}

func (m OrderedMap[K, V]) MarshalJSON() ([]byte, error) {
	if len(m.mp) == 0 {
		return []byte("{}"), nil
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)

	for i, k := range m.keys {
		if i > 0 {
			buf.WriteByte(',')
		}

		keyString, err := resolveKeyName(reflect.ValueOf(k))
		if err != nil {
			return nil, err
		}

		if err := enc.Encode(keyString); err != nil {
			return nil, err
		}

		buf.WriteByte(':')

		if err := enc.Encode(m.mp[k]); err != nil {
			return nil, err
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func resolveKeyName(k reflect.Value) (string, error) {
	if k.Kind() == reflect.String {
		return k.String(), nil
	}
	if tm, ok := k.Interface().(encoding.TextMarshaler); ok {
		if k.Kind() == reflect.Pointer && k.IsNil() {
			return "", nil
		}
		buf, err := tm.MarshalText()
		return string(buf), err
	}
	switch k.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(k.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(k.Uint(), 10), nil
	}
	panic("unexpected map key type")
}
