//go:build ssa2llvm_pruned_runtime

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
	return reflect.ValueOf(v).Len()
}

func runtimeYakBuiltinCap(v any) int {
	if v == nil {
		return 0
	}
	if canCap, ok := v.(runtimeYakCapper); ok {
		return canCap.Cap()
	}
	return reflect.ValueOf(v).Cap()
}
