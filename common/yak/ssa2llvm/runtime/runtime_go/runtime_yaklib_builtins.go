package main

import (
	"reflect"
	"unicode/utf8"
)

type runtimeYakLenner interface {
	Len() int
}

type runtimeYakCapper interface {
	Cap() int
}

func runtimeYakBuiltinLen(v any) int {
	if v == nil {
		return 0
	}
	if canLen, ok := v.(runtimeYakLenner); ok {
		return canLen.Len()
	}
	if s, ok := v.(string); ok {
		return utf8.RuneCountInString(s)
	}
	rv := reflect.ValueOf(v)
	for rv.IsValid() && rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return 0
		}
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return 0
		}
		rv = rv.Elem()
	}
	return rv.Len()
}

func runtimeYakBuiltinCap(v any) int {
	if v == nil {
		return 0
	}
	if canCap, ok := v.(runtimeYakCapper); ok {
		return canCap.Cap()
	}
	rv := reflect.ValueOf(v)
	for rv.IsValid() && rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return 0
		}
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return 0
		}
		rv = rv.Elem()
	}
	return rv.Cap()
}
