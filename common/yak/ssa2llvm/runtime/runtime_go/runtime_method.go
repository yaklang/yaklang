package main

import (
	"fmt"
	"reflect"
	"unsafe"
)

func stdlibRuntimeShadowMethod(args []uint64) int64 {
	if len(args) < 2 {
		return 0
	}

	methodNamePtr := unsafe.Pointer(uintptr(args[1]))
	if methodNamePtr == nil {
		return 0
	}

	objPtr := unsafe.Pointer(uintptr(args[0]))
	if objPtr == nil {
		return 0
	}

	ret, err := callRuntimeShadowMethod(objPtr, cStringToGoString(methodNamePtr), args[2:])
	if err != nil {
		panic(err)
	}
	return ret
}

func resolveRuntimeMethod(obj any, name string) (reflect.Value, error) {
	value := reflect.ValueOf(obj)
	if !value.IsValid() {
		return reflect.Value{}, fmt.Errorf("invalid object while resolving method %q", name)
	}

	if method := value.MethodByName(name); method.IsValid() {
		return method, nil
	}

	for value.IsValid() && value.Kind() == reflect.Interface {
		if value.IsNil() {
			break
		}
		value = value.Elem()
		if method := value.MethodByName(name); method.IsValid() {
			return method, nil
		}
	}

	if value.IsValid() && value.Kind() != reflect.Ptr && value.CanAddr() {
		if method := value.Addr().MethodByName(name); method.IsValid() {
			return method, nil
		}
	}

	return reflect.Value{}, fmt.Errorf("method %q not found", name)
}

func decodeRuntimeArg(raw uint64, targetType reflect.Type) (reflect.Value, error) {
	if targetType == nil {
		return reflect.Value{}, fmt.Errorf("missing target type")
	}

	decoded := decodeTaggedArg(raw)
	if decoded == nil {
		return reflect.Zero(targetType), nil
	}

	value := reflect.ValueOf(decoded)
	if value.IsValid() {
		if value.Type().AssignableTo(targetType) {
			return value, nil
		}
		if value.Type().ConvertibleTo(targetType) {
			return value.Convert(targetType), nil
		}
		if targetType.Kind() == reflect.Interface && value.Type().Implements(targetType) {
			return value, nil
		}
		if targetType.Kind() == reflect.Ptr && value.Kind() != reflect.Ptr && value.CanAddr() && value.Addr().Type().AssignableTo(targetType) {
			return value.Addr(), nil
		}
	}

	if intValue, ok := decoded.(int64); ok {
		if converted, ok := valueForSet(targetType, intValue); ok {
			return converted, nil
		}
	}

	return reflect.Value{}, fmt.Errorf("cannot use %T as %s", decoded, targetType)
}

func decodeRuntimeMethodArgs(method reflect.Value, rawArgs []uint64) ([]reflect.Value, error) {
	methodType := method.Type()
	if !methodType.IsVariadic() && len(rawArgs) != methodType.NumIn() {
		return nil, fmt.Errorf("method expects %d args, got %d", methodType.NumIn(), len(rawArgs))
	}
	if methodType.IsVariadic() && len(rawArgs) < methodType.NumIn()-1 {
		return nil, fmt.Errorf("method expects at least %d args, got %d", methodType.NumIn()-1, len(rawArgs))
	}

	args := make([]reflect.Value, 0, len(rawArgs))
	for index, raw := range rawArgs {
		targetType := methodType.In(index)
		if methodType.IsVariadic() && index >= methodType.NumIn()-1 {
			targetType = targetType.Elem()
		}

		arg, err := decodeRuntimeArg(raw, targetType)
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
	}
	return args, nil
}

func runtimeMethodReturnValue(results []reflect.Value) int64 {
	if len(results) == 0 {
		return 0
	}

	errorType := reflect.TypeOf((*error)(nil)).Elem()
	last := results[len(results)-1]
	if last.IsValid() && last.Type().Implements(errorType) && !last.IsNil() {
		panic(last.Interface().(error))
	}

	return runtimeValueToInt64(results[0])
}

func callRuntimeShadowMethod(objPtr unsafe.Pointer, methodName string, rawArgs []uint64) (int64, error) {
	handle, ok := handleFromShadow(objPtr)
	if !ok {
		return 0, fmt.Errorf("invalid shadow object for method %q", methodName)
	}

	method, err := resolveRuntimeMethod(handle.Value(), methodName)
	if err != nil {
		return 0, err
	}

	args, err := decodeRuntimeMethodArgs(method, rawArgs)
	if err != nil {
		return 0, err
	}

	return runtimeMethodReturnValue(method.Call(args)), nil
}
