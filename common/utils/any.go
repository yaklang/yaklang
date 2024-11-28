package utils

import "reflect"

func IsNil(input any) bool {
	if input == nil {
		return true
	}
	if refValue, ok := input.(reflect.Value); ok {
		return refValue.IsNil()
	}
	ref := reflect.ValueOf(input)
	switch ref.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Chan:
		return reflect.ValueOf(input).IsNil()
	default:
		return false
	}
}

func IsMap(input any) bool {
	switch input.(type) {
	case map[string]any:
		return true
	default:
		reflectValue := reflect.ValueOf(input)
		if reflectValue.Kind() == reflect.Map {
			return true
		}
	}
	return false
}
