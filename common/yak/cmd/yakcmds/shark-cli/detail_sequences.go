package sharkcli

import "reflect"

// Primitive sequences have the same detail layout before and after a JSON
// worker. Binary slices retain their existing byte-source handling. Convert
// only the visible prefix, without mutating an event or allocating from an
// unbounded metadata length. A typed nil slice has JSON's null semantics.
func detailScalarSequence(value any, limit int) (any, bool) {
	if value == nil {
		return nil, false
	}
	v := reflect.ValueOf(value)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return value, false
	}
	kind := v.Type().Elem().Kind()
	if v.Kind() == reflect.Slice {
		if kind == reflect.Uint8 {
			return value, false
		}
		if v.IsNil() {
			return nil, false
		}
	}
	switch kind {
	case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
	default:
		return value, false
	}
	n := min(v.Len(), max(0, limit))
	items := make([]any, n)
	for i := range items {
		item := v.Index(i)
		switch kind {
		case reflect.String:
			items[i] = item.String()
		case reflect.Bool:
			items[i] = item.Bool()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			items[i] = item.Int()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			items[i] = item.Uint()
		case reflect.Float32:
			items[i] = float32(item.Float())
		case reflect.Float64:
			items[i] = item.Float()
		}
	}
	return items, n < v.Len()
}
