package main

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"unsafe"
)

func runtimeDispatchYaklibCall(args []uint64) (int64, error) {
	if len(args) < 2 {
		return 0, fmt.Errorf("yaklib call expects module and method name")
	}
	pkg := runtimeCStringToGoString(unsafe.Pointer(uintptr(args[0])))
	method := runtimeCStringToGoString(unsafe.Pointer(uintptr(args[1])))
	fn, ok := runtimeLookupYaklibCallable(pkg, method)
	if !ok || fn == nil {
		if pkg == "" {
			return 0, fmt.Errorf("yaklib global callable %q not found", method)
		}
		return 0, fmt.Errorf("yaklib export %q.%q not found", pkg, method)
	}
	return callRuntimeValue(reflect.ValueOf(fn), args[2:])
}

type runtimeIterSlot struct {
	iter      runtimeIterator
	exhausted bool
}

type runtimeIterKey struct {
	nextID int64
	iter   string
}

var runtimeIterStates sync.Map

type runtimeIterator interface {
	Next() (key any, field any, ok bool)
}

type runtimeSliceIterator struct {
	value  reflect.Value
	index  int
	inNext bool
}

func (it *runtimeSliceIterator) Next() (any, any, bool) {
	if it.index >= it.value.Len() {
		return nil, nil, false
	}
	current := it.index
	it.index++
	elem := it.value.Index(current).Interface()
	if it.inNext {
		return elem, nil, true
	}
	return current, elem, true
}

type runtimeMapIterator struct {
	keys   []reflect.Value
	index  int
	values reflect.Value
}

func (it *runtimeMapIterator) Next() (any, any, bool) {
	if it.index >= len(it.keys) {
		return nil, nil, false
	}
	key := it.keys[it.index]
	val := it.values.MapIndex(key)
	it.index++
	return key.Interface(), val.Interface(), true
}

func runtimeDecodeIterValue(raw uint64) any {
	decoded := decodeTaggedArg(raw)
	if decoded == nil {
		return nil
	}
	if ptr, ok := decoded.(int64); ok {
		if handle, ok := handleFromShadow(unsafe.Pointer(uintptr(ptr))); ok {
			return handle.Value()
		}
	}
	return decoded
}

func newRuntimeIterator(value any, inNext bool) (runtimeIterator, error) {
	if value == nil {
		return nil, fmt.Errorf("cannot iterate nil value")
	}
	v := reflect.ValueOf(value)
	for v.IsValid() && v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil, fmt.Errorf("cannot iterate nil interface")
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Slice, reflect.Array:
		return &runtimeSliceIterator{value: v, inNext: inNext}, nil
	case reflect.Map:
		return &runtimeMapIterator{keys: v.MapKeys(), values: v}, nil
	case reflect.String:
		runes := []rune(v.String())
		return &runtimeSliceIterator{
			value:  reflect.ValueOf(runes),
			inNext: true,
		}, nil
	default:
		return nil, fmt.Errorf("cannot iterate over %T", value)
	}
}

func runtimeDispatchNext(args []uint64) (int64, error) {
	if len(args) < 3 {
		return 0, fmt.Errorf("runtime next expects iter, inNext, and next id")
	}
	inNext := args[1] != 0
	nextID := int64(args[2])
	iterValue := runtimeDecodeIterValue(args[0])
	if iterValue == nil {
		result := map[string]any{"key": nil, "field": nil, "ok": false}
		return int64(uintptr(newRuntimeShadow(result))), nil
	}

	stateKey := runtimeIterKey{nextID: nextID, iter: fmt.Sprintf("%p", iterValue)}
	slotAny, _ := runtimeIterStates.Load(stateKey)
	slot, _ := slotAny.(*runtimeIterSlot)
	if slot == nil || slot.exhausted {
		iter, err := newRuntimeIterator(iterValue, inNext)
		if err != nil {
			return 0, err
		}
		slot = &runtimeIterSlot{iter: iter}
	}

	k, f, ok := slot.iter.Next()
	if !ok {
		slot.exhausted = true
	}
	runtimeIterStates.Store(stateKey, slot)

	result := map[string]any{
		"key":   k,
		"field": f,
		"ok":    ok,
	}
	return int64(uintptr(newRuntimeShadow(result))), nil
}

func runtimeDispatchChanRecv(args []uint64) (int64, error) {
	if len(args) < 1 {
		return 0, fmt.Errorf("chan recv expects channel argument")
	}
	value := runtimeDecodeIterValue(args[0])
	rv := reflect.ValueOf(value)
	for rv.IsValid() && rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return 0, nil
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() || rv.Kind() != reflect.Chan {
		return 0, fmt.Errorf("chan recv on non-channel %T", value)
	}
	recv, ok := rv.Recv()
	if !ok {
		return 0, nil
	}
	return runtimeValueToInt64(recv), nil
}

func runtimeMatchInContainer(left, right any) bool {
	if left == nil || right == nil {
		return false
	}
	if s, ok := right.(string); ok {
		if ls, ok := left.(string); ok {
			return strings.Contains(s, ls)
		}
	}

	rv := reflect.ValueOf(right)
	for rv.IsValid() && rv.Kind() == reflect.Interface {
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return false
	}

	switch rv.Kind() {
	case reflect.Map:
		lv := reflect.ValueOf(left)
		for lv.IsValid() && lv.Kind() == reflect.Interface {
			lv = lv.Elem()
		}
		if !lv.IsValid() {
			return false
		}
		if lv.Type().ConvertibleTo(rv.Type().Key()) {
			lv = lv.Convert(rv.Type().Key())
		} else if rv.Type().Key().Kind() == reflect.String {
			lv = reflect.ValueOf(fmt.Sprint(left))
		} else {
			return false
		}
		return rv.MapIndex(lv).IsValid()
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			if reflect.DeepEqual(rv.Index(i).Interface(), left) {
				return true
			}
		}
		return false
	case reflect.String:
		return strings.Contains(rv.String(), fmt.Sprint(left))
	default:
		return false
	}
}

func runtimeDispatchIn(args []uint64) (int64, error) {
	if len(args) < 2 {
		return 0, fmt.Errorf("runtime in expects left and right operands")
	}
	left := decodeTaggedArg(args[0])
	right := decodeTaggedArg(args[1])
	if runtimeMatchInContainer(left, right) {
		return 1, nil
	}
	return 0, nil
}

func runtimeDecodeEqValue(raw uint64) any {
	if (raw & yakTaggedPointerMask) != 0 {
		raw &^= yakTaggedPointerMask
		ptr := unsafe.Pointer(uintptr(raw))
		if ptr == nil {
			return nil
		}
		if h, ok := handleFromShadow(ptr); ok {
			return h.Value()
		}
		return runtimeCStringToGoString(ptr)
	}

	if raw == 0 {
		return nil
	}
	if h, ok := handleFromShadow(unsafe.Pointer(uintptr(raw))); ok {
		return h.Value()
	}
	return int64(raw)
}

func runtimeNumericValue(v any) (float64, bool) {
	switch value := v.(type) {
	case int:
		return float64(value), true
	case int8:
		return float64(value), true
	case int16:
		return float64(value), true
	case int32:
		return float64(value), true
	case int64:
		return float64(value), true
	case uint:
		return float64(value), true
	case uint8:
		return float64(value), true
	case uint16:
		return float64(value), true
	case uint32:
		return float64(value), true
	case uint64:
		return float64(value), true
	case uintptr:
		return float64(value), true
	case float32:
		return float64(value), true
	case float64:
		return value, true
	default:
		return 0, false
	}
}

func runtimeValuesEqual(left, right any) bool {
	if reflect.DeepEqual(left, right) {
		return true
	}
	if ln, ok := runtimeNumericValue(left); ok {
		if rn, ok := runtimeNumericValue(right); ok {
			return ln == rn
		}
	}
	return false
}

func runtimeDispatchEq(args []uint64) (int64, error) {
	if len(args) < 2 {
		return 0, fmt.Errorf("runtime eq expects left and right operands")
	}
	equal := runtimeValuesEqual(runtimeDecodeEqValue(args[0]), runtimeDecodeEqValue(args[1]))
	if len(args) > 2 && args[2] != 0 {
		equal = !equal
	}
	if equal {
		return 1, nil
	}
	return 0, nil
}
