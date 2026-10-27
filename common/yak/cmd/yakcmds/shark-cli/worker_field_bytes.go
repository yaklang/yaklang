package sharkcli

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Type facts travel separately from captured map keys. Payload text that looks
// like base64, or a map that resembles a type marker, cannot create byte types.
// These paths describe only an owned parent's actual []byte values, never a
// wire offset, protocol identity, decryption proof or parsing instruction.
type workerByteField struct {
	Event  int
	Root   string
	Path   []string
	Length int
	Nil    bool `json:",omitempty"`
	// JSON replaces invalid UTF-8 inside Go strings. Preserve only that
	// otherwise irreversible case, without reclassifying valid text as bytes.
	StringBytes []byte `json:",omitempty"`
}

const workerByteFieldLimit = 16384

func eventByteFields(e *pcaputil.ProtocolEvent, index int) ([]workerByteField, error) {
	var fields []workerByteField
	nodes := 0
	var walk func(reflect.Value, string, []string, int) error
	walk = func(v reflect.Value, root string, path []string, depth int) error {
		if !v.IsValid() {
			return nil
		}
		for v.Kind() == reflect.Interface {
			if v.IsNil() {
				return nil
			}
			v = v.Elem()
		}
		nodes++
		if nodes > 65536 || depth > 32 {
			return fmt.Errorf("worker field-type traversal budget exceeded")
		}
		if v.CanInterface() {
			if b, ok := v.Interface().([]byte); ok {
				if len(fields) >= workerByteFieldLimit {
					return fmt.Errorf("worker byte-field count budget exceeded")
				}
				fields = append(fields, workerByteField{Event: index, Root: root, Path: append([]string(nil), path...), Length: len(b), Nil: b == nil})
				return nil
			}
			if text, ok := v.Interface().(string); ok && !utf8.ValidString(text) {
				if len(fields) >= workerByteFieldLimit {
					return fmt.Errorf("worker byte-field count budget exceeded")
				}
				fields = append(fields, workerByteField{Event: index, Root: root, Path: append([]string(nil), path...), Length: len(text), StringBytes: []byte(text)})
				return nil
			}
		}
		switch v.Kind() {
		case reflect.Map:
			if v.Type().Key().Kind() != reflect.String {
				return nil
			}
			keys := v.MapKeys()
			sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
			for _, key := range keys {
				if !utf8.ValidString(key.String()) {
					return fmt.Errorf("worker field key cannot be represented losslessly in JSON")
				}
				if err := walk(v.MapIndex(key), root, append(path, key.String()), depth+1); err != nil {
					return err
				}
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				if err := walk(v.Index(i), root, append(path, strconv.Itoa(i)), depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, root := range []struct {
		name  string
		value map[string]any
	}{{"Fields", e.Fields}, {"Session", e.Session}} {
		if err := walk(reflect.ValueOf(root.value), root.name, nil, 0); err != nil {
			return nil, err
		}
	}
	return fields, nil
}

func restoreWorkerByteFields(events []*pcaputil.ProtocolEvent, fields []workerByteField) error {
	if len(fields) > workerByteFieldLimit {
		return fmt.Errorf("worker byte-field count exceeds limit")
	}
	seen := make(map[string]bool, len(fields))
	for _, f := range fields {
		if f.Event < 0 || f.Event >= len(events) || events[f.Event] == nil || len(f.Path) == 0 || len(f.Path) > 32 || f.Length < 0 || f.Length > decoderInputLimit {
			return fmt.Errorf("worker byte-field descriptor outside bounds")
		}
		identity, _ := json.Marshal([]any{f.Event, f.Root, f.Path})
		if seen[string(identity)] {
			return fmt.Errorf("duplicate worker byte-field path")
		}
		seen[string(identity)] = true
		var current any
		switch f.Root {
		case "Fields":
			current = events[f.Event].Fields
		case "Session":
			current = events[f.Event].Session
		default:
			return fmt.Errorf("unknown worker byte-field root")
		}
		for i, key := range f.Path {
			var value any
			var assign func(any)
			switch container := current.(type) {
			case map[string]any:
				var present bool
				value, present = container[key]
				if !present {
					return fmt.Errorf("worker byte-field key absent")
				}
				assign = func(v any) { container[key] = v }
			case []any:
				n, err := strconv.Atoi(key)
				if err != nil || n < 0 || n >= len(container) || strconv.Itoa(n) != key {
					return fmt.Errorf("worker byte-field array index invalid")
				}
				value = container[n]
				assign = func(v any) { container[n] = v }
			default:
				return fmt.Errorf("worker byte-field path crosses a scalar")
			}
			if i+1 != len(f.Path) {
				current = value
				continue
			}
			if len(f.StringBytes) != 0 {
				text, ok := value.(string)
				if !ok || f.Nil || len(f.StringBytes) != f.Length || utf8.Valid(f.StringBytes) {
					return fmt.Errorf("worker string-byte descriptor disagrees with value")
				}
				encoded, err := json.Marshal(string(f.StringBytes))
				if err != nil {
					return err
				}
				var normalized string
				if err := json.Unmarshal(encoded, &normalized); err != nil || normalized != text {
					return fmt.Errorf("worker string-byte descriptor does not bind the encoded text")
				}
				assign(string(f.StringBytes))
				continue
			}
			if f.Nil {
				if f.Length != 0 || value != nil {
					return fmt.Errorf("worker nil-byte descriptor disagrees with value")
				}
				assign([]byte(nil))
				continue
			}
			encoded, ok := value.(string)
			if !ok {
				return fmt.Errorf("worker byte-field value is not encoded bytes")
			}
			data, err := base64.StdEncoding.Strict().DecodeString(encoded)
			if err != nil || len(data) != f.Length || base64.StdEncoding.EncodeToString(data) != encoded {
				return fmt.Errorf("worker byte-field length or encoding mismatch")
			}
			assign(data)
		}
	}
	return nil
}
