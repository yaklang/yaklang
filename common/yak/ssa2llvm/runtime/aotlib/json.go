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
		vb, err := json.Marshal(m.values[key])
		if err != nil {
			return nil, err
		}
		buf.Write(vb)
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

// jsonDumpOption is the AOT representation of json.dumps options. The
// option functions return these values; JsonDumps inspects them by type.
type jsonDumpOption interface{ apply(*jsonDumpConfig) }

type jsonDumpConfig struct {
	indent       string
	noEscapeHTML bool
}

type jsonIndentOption string

func (o jsonIndentOption) apply(c *jsonDumpConfig) { c.indent = string(o) }

type jsonNoEscapeHTMLOption struct{}

func (jsonNoEscapeHTMLOption) apply(c *jsonDumpConfig) { c.noEscapeHTML = true }

// JsonWithIndent returns a json.dumps option that sets the indent string.
func JsonWithIndent(indent interface{}) interface{} {
	return jsonIndentOption(fmt.Sprint(indent))
}

// JsonNoEscapeHTML returns a json.dumps option that disables HTML escaping.
func JsonNoEscapeHTML() interface{} {
	return jsonNoEscapeHTMLOption{}
}

// JsonDumps serializes a value to a JSON string. Shadow objects (e.g. the
// runtime's ordered map or this package's orderedMap) implement MarshalJSON,
// so nested yak objects keep their key order.
func JsonDumps(raw interface{}, opts ...interface{}) string {
	cfg := jsonDumpConfig{indent: "  "}
	for _, opt := range opts {
		if o, ok := opt.(jsonDumpOption); ok {
			o.apply(&cfg)
		}
	}
	if cfg.noEscapeHTML {
		if om, ok := raw.(*orderedMap); ok {
			return marshalOrderedMapNoEscape(om)
		}
		if ne, ok := raw.(interface{ MarshalJSONNoEscape() ([]byte, error) }); ok {
			if b, err := ne.MarshalJSONNoEscape(); err == nil {
				return string(b)
			}
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if cfg.indent != "" {
			enc.SetIndent("", cfg.indent)
		}
		if err := enc.Encode(raw); err != nil {
			return ""
		}
		result := buf.String()
		if len(result) > 0 && result[len(result)-1] == '\n' {
			result = result[:len(result)-1]
		}
		return result
	}
	var b []byte
	var err error
	if cfg.indent == "" {
		// json.Marshal keeps custom MarshalJSON output compact; MarshalIndent
		// with an empty indent still re-indents every line.
		b, err = json.Marshal(raw)
	} else {
		b, err = json.MarshalIndent(raw, "", cfg.indent)
	}
	if err != nil {
		return ""
	}
	return string(b)
}

func marshalOrderedMapNoEscape(m *orderedMap) string {
	if m == nil {
		return "{}"
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range m.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(key)
		if err != nil {
			return ""
		}
		buf.Write(kb)
		buf.WriteByte(':')
		var vb bytes.Buffer
		enc := json.NewEncoder(&vb)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(m.values[key]); err != nil {
			return ""
		}
		s := vb.String()
		if len(s) > 0 && s[len(s)-1] == '\n' {
			s = s[:len(s)-1]
		}
		buf.WriteString(s)
	}
	buf.WriteByte('}')
	return buf.String()
}

// JsonExports mirrors the json module's export table (the AOT-supported
// subset). Entries match common/yak/yaklib.JsonExports signatures.
var JsonExports = map[string]any{
	"loads":        JsonLoads,
	"dumps":        JsonDumps,
	"withIndent":   JsonWithIndent,
	"noEscapeHTML": JsonNoEscapeHTML,
}
