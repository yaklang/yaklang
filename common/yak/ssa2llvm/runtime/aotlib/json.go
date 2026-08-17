package aotlib

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// orderedMap is the AOT json module's object representation. It implements
// Get/Set so the runtime's field access treats it like its own ordered map
// (preserving value flags such as bool/string), and MarshalJSON keeps key
// insertion order for json.dumps.
type orderedMap struct {
	keys   []string
	values map[string]any
}

func newOrderedMap() *orderedMap {
	return &orderedMap{values: map[string]any{}}
}

func (m *orderedMap) Get(key string) (any, bool) {
	if m == nil || m.values == nil {
		return nil, false
	}
	value, ok := m.values[key]
	return value, ok
}

func (m *orderedMap) Set(key string, value any) {
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

// UnmarshalJSON decodes a JSON object into the ordered map, preserving key
// insertion order via the streaming decoder.
func (m *orderedMap) UnmarshalJSON(data []byte) error {
	if m == nil {
		return fmt.Errorf("nil orderedMap")
	}
	if m.values == nil {
		m.values = map[string]any{}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return fmt.Errorf("expected JSON object")
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("expected string key")
		}
		var value any
		if err := dec.Decode(&value); err != nil {
			return err
		}
		m.Set(key, value)
	}
	_, err = dec.Token()
	return err
}

func (m *orderedMap) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	enc := json.NewEncoder(&buf)
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
		if err := enc.Encode(m.values[key]); err != nil {
			return nil, err
		}
		// Encoder appends a newline; trim it.
		b := buf.Bytes()
		if len(b) > 0 && b[len(b)-1] == '\n' {
			buf.Truncate(len(b) - 1)
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// JsonLoads parses a JSON document into an ordered map for objects, []any for
// arrays, and plain values otherwise. The AOT runtime wraps non-primitive
// results in shadows, so member reads/writes on the result go through the
// runtime's reflect-based field access.
func JsonLoads(raw interface{}) interface{} {
	str := ""
	switch v := raw.(type) {
	case string:
		str = v
	case []byte:
		str = string(v)
	default:
		str = strings.TrimSpace(fmt.Sprint(v))
	}
	str = strings.TrimSpace(str)

	obj := newOrderedMap()
	if err := json.Unmarshal([]byte(str), obj); err == nil {
		return obj
	}
	var arr []any
	if err := json.Unmarshal([]byte(str), &arr); err == nil {
		return arr
	}
	var v any
	if err := json.Unmarshal([]byte(str), &v); err == nil {
		return v
	}
	return newOrderedMap()
}

// JsonDumps serializes a value to a JSON string. Shadow objects (e.g. the
// runtime's ordered map or this package's orderedMap) implement MarshalJSON,
// so nested yak objects keep their key order.
func JsonDumps(raw interface{}) string {
	// Match common/yak/yaklib._jsonDumps default config: indent "  ".
	b, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

// JsonExports mirrors the json module's export table (the AOT-supported
// subset). Entries match common/yak/yaklib.JsonExports signatures.
var JsonExports = map[string]any{
	"loads": JsonLoads,
	"dumps": JsonDumps,
}
