package aitool

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// coerceParamsToSchema aligns weakly-typed model output with declared scalar
// JSON Schema types before validation. Models frequently send "30" for an
// integer or 0 for a string; rejecting those calls wastes retries without
// adding safety. Only unambiguous scalar conversions are applied, and the
// caller's map is updated so tool execution sees the normalized values.
func coerceParamsToSchema(schema any, params map[string]any) {
	schemaMap, _ := schema.(map[string]any)
	coerceObject(schemaMap, params)
}

func coerceObject(schema map[string]any, object map[string]any) {
	if schema == nil || object == nil {
		return
	}
	props, _ := schema["properties"].(map[string]any)
	for name, rawProp := range props {
		value, ok := object[name]
		if !ok {
			continue
		}
		prop, _ := rawProp.(map[string]any)
		if coerced, changed := coerceValue(prop, value); changed {
			object[name] = coerced
		}
	}
}

func coerceValue(prop map[string]any, value any) (any, bool) {
	if prop == nil {
		return value, false
	}
	for _, key := range []string{"oneOf", "anyOf", "allOf"} {
		if variants, ok := prop[key].([]any); ok {
			for _, raw := range variants {
				variant, _ := raw.(map[string]any)
				if coerced, changed := coerceValue(variant, value); changed {
					return coerced, true
				}
			}
		}
	}

	switch schemaTypeName(prop["type"]) {
	case "object":
		if obj, ok := value.(map[string]any); ok {
			coerceObject(prop, obj)
		}
	case "array":
		items, _ := prop["items"].(map[string]any)
		if arr, ok := value.([]any); ok && items != nil {
			changed := false
			for i, item := range arr {
				if coerced, c := coerceValue(items, item); c {
					arr[i] = coerced
					changed = true
				}
			}
			return arr, changed
		}
	case "integer":
		switch v := value.(type) {
		case string:
			s := strings.TrimSpace(v)
			if s == "" {
				return value, false
			}
			if i, err := strconv.ParseInt(s, 10, 64); err == nil {
				return int(i), true
			}
			if f, err := strconv.ParseFloat(s, 64); err == nil && isIntegralFloat(f) {
				return int(f), true
			}
		case float64:
			if isIntegralFloat(v) {
				return int(v), true
			}
		case float32:
			if isIntegralFloat(float64(v)) {
				return int(v), true
			}
		case json.Number:
			if i, err := v.Int64(); err == nil {
				return int(i), true
			}
			if f, err := v.Float64(); err == nil && isIntegralFloat(f) {
				return int(f), true
			}
		}
	case "number":
		switch v := value.(type) {
		case string:
			s := strings.TrimSpace(v)
			if s == "" {
				return value, false
			}
			if f, err := strconv.ParseFloat(s, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
				return f, true
			}
		case json.Number:
			if f, err := v.Float64(); err == nil {
				return f, true
			}
		}
	case "string":
		switch v := value.(type) {
		case bool:
			return strconv.FormatBool(v), true
		case int:
			return strconv.Itoa(v), true
		case int64:
			return strconv.FormatInt(v, 10), true
		case float64:
			if !math.IsNaN(v) && !math.IsInf(v, 0) {
				if math.Trunc(v) == v {
					return strconv.FormatInt(int64(v), 10), true
				}
				return strconv.FormatFloat(v, 'f', -1, 64), true
			}
		case float32:
			return strconv.FormatFloat(float64(v), 'f', -1, 32), true
		case json.Number:
			return v.String(), true
		}
	case "boolean":
		switch v := value.(type) {
		case string:
			if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
				return b, true
			}
		case float64:
			if v == 0 {
				return false, true
			}
			if v == 1 {
				return true, true
			}
		case int:
			if v == 0 {
				return false, true
			}
			if v == 1 {
				return true, true
			}
		case json.Number:
			if i, err := v.Int64(); err == nil {
				if i == 0 {
					return false, true
				}
				if i == 1 {
					return true, true
				}
			}
		}
	}
	return value, false
}

func schemaTypeName(raw any) string {
	switch v := raw.(type) {
	case string:
		return strings.ToLower(strings.TrimSpace(v))
	case []any:
		for _, item := range v {
			name := schemaTypeName(item)
			if name != "" && name != "null" {
				return name
			}
		}
	}
	return ""
}

func isIntegralFloat(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0) && math.Trunc(f) == f && f >= math.MinInt64 && f <= math.MaxInt64
}
