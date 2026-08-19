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
