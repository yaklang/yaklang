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

func (m *runtimeOrderedMap) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	encoder := json.NewEncoder(&buf)
	for i, key := range m.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := encoder.Encode(key); err != nil {
			return nil, err
		}
		buf.WriteByte(':')
		if err := encoder.Encode(m.values[key]); err != nil {
			return nil, err
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
