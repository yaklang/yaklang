package utils

import (
	"reflect"
	"unsafe"
)

func GetUnexportedField(field reflect.Value) interface{} {
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface()
}

func SetUnexportedField(field reflect.Value, value interface{}) {
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).
		Elem().
		Set(reflect.ValueOf(value))
}

func UnsafeStringToBytes(s string) []byte {
	return *(*[]byte)(unsafe.Pointer(&s))
}

func UnsafeBytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
