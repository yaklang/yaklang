package utils

import (
	"reflect"
)

func IsNil(input any) bool {
	if input == nil {
		return true
	}
	if refValue, ok := input.(reflect.Value); ok {
		if !refValue.IsValid() {
			return true
		}
		return refValue.IsNil()
	}
	ref := reflect.ValueOf(input)
	if !ref.IsValid() {
		return true
	}
	// Peel interface wrappers so typed nil pointers (e.g. var v Value = (*T)(nil)) are detected.
	for ref.Kind() == reflect.Interface && !ref.IsNil() {
		ref = ref.Elem()
	}
	if !ref.IsValid() {
		return true
	}
	switch ref.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.UnsafePointer, reflect.Slice:
		return ref.IsNil()
	case reflect.Interface:
		return ref.IsNil()
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
