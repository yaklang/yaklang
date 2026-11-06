package main

import (
	"bytes"
	"encoding/json"
)

type runtimeStringMap interface {
	Get(string) (any, bool)
	Set(string, any)
}

type runtimeOrderedMap struct {
	keys   []string
	values map[string]any
}

func newRuntimeOrderedMap() *runtimeOrderedMap {
	return &runtimeOrderedMap{
		keys:   []string{},
		values: map[string]any{},
	}
}

func (m *runtimeOrderedMap) Get(key string) (any, bool) {
	if m == nil || m.values == nil {
		return nil, false
	}
	value, ok := m.values[key]
	return value, ok
}

func (m *runtimeOrderedMap) Set(key string, value any) {
	if m == nil {
		return
	}
	if m.values == nil {
		m.values = map[string]any{}
	}
	if _, exists := m.values[key]; !exists {
		m.keys = append(m.keys, key)
	}
	m.values[key] = value
}

func (m *runtimeOrderedMap) Len() int {
	if m == nil {
		return 0
	}
	return len(m.keys)
}

// Keys returns the map keys in insertion order.
func (m *runtimeOrderedMap) Keys() []string {
	if m == nil {
		return nil
	}
	return append([]string(nil), m.keys...)
}

// Values returns the map values in insertion order.
func (m *runtimeOrderedMap) Values() []any {
	if m == nil {
		return nil
	}
	out := make([]any, 0, len(m.keys))
	for _, k := range m.keys {
		out = append(out, m.values[k])
	}
	return out
}

// Has reports whether the map contains the key.
func (m *runtimeOrderedMap) Has(key string) bool {
	if m == nil {
		return false
	}
	_, ok := m.values[key]
	return ok
}

// Delete removes the key, preserving insertion order of the remaining keys.
func (m *runtimeOrderedMap) Delete(key string) {
	if m == nil {
		return
	}
	if _, ok := m.values[key]; !ok {
		return
	}
	delete(m.values, key)
	for i, k := range m.keys {
		if k == key {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			break
		}
	}
}

func (m *runtimeOrderedMap) marshalJSON(noEscape bool) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range m.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		var vb bytes.Buffer
		enc := json.NewEncoder(&vb)
		enc.SetEscapeHTML(!noEscape)
		if err := enc.Encode(m.values[key]); err != nil {
			return nil, err
		}
		v := vb.Bytes()
		if len(v) > 0 && v[len(v)-1] == '\n' {
			v = v[:len(v)-1]
		}
		buf.Write(v)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func (m *runtimeOrderedMap) MarshalJSON() ([]byte, error) {
	return m.marshalJSON(false)
}

// MarshalJSONNoEscape is used by the AOT json.dumps noEscapeHTML option;
// encoding/json cannot disable HTML escaping for custom MarshalJSON types.
func (m *runtimeOrderedMap) MarshalJSONNoEscape() ([]byte, error) {
	return m.marshalJSON(true)
}
