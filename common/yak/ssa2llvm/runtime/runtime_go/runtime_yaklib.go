package main

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"unsafe"
)

// runtimeBuiltinRetry mirrors yak's global retry(times, handler): call handler
// up to times times; handler returning false (or panicking) stops the loop.
func runtimeBuiltinRetry(maxTimes int, handler func() bool) {
	for i := 0; i < maxTimes; i++ {
		var ret bool
		func() {
			defer func() { _ = recover() }()
			ret = handler()
		}()
		if !ret {
			return
		}
	}
}

// runtimeBuiltinParam mirrors yak's global param(name, defaults...): read a
// CLI/environment parameter. In AOT binaries parameters come from the
// environment, with an optional default value.
func runtimeBuiltinParam(name string, defaults ...string) any {
	if name != "" {
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
	}
	if len(defaults) > 0 {
		return defaults[0]
	}
	return nil
}

func runtimeDispatchYaklibCall(args []uint64, ellipsis bool) (int64, error) {
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
	return callRuntimeValue(reflect.ValueOf(fn), args[2:], ellipsis)
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

type runtimeIntIterator struct {
	index int64
	limit int64
}

func (it *runtimeIntIterator) Next() (any, any, bool) {
	if it.index >= it.limit {
		return nil, nil, false
	}
	cur := it.index
	it.index++
	return cur, nil, true
}

type runtimeChanIterator struct {
	value reflect.Value
}

func (it *runtimeChanIterator) Next() (any, any, bool) {
	if it == nil || !it.value.IsValid() || it.value.Kind() != reflect.Chan {
		return nil, nil, false
	}
	recv, ok := it.value.Recv()
	if !ok {
		return nil, nil, false
	}
	return recv.Interface(), nil, true
}

type runtimeMapIterator struct {
	keys   []reflect.Value
	index  int
	values reflect.Value
}

type runtimeOrderedMapIterator struct {
	m     *runtimeOrderedMap
	index int
}

func (it *runtimeOrderedMapIterator) Next() (any, any, bool) {
	if it == nil || it.m == nil || it.index >= len(it.m.keys) {
		return nil, nil, false
	}
	key := it.m.keys[it.index]
	val := it.m.values[key]
	it.index++
	return key, val, true
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
	// The AOT runtime represents yak maps as runtimeOrderedMap; iterate its
	// string keys in insertion order.
	if om, ok := value.(*runtimeOrderedMap); ok && om != nil {
		return &runtimeOrderedMapIterator{m: om}, nil
	}
	v := reflect.ValueOf(value)
	for v.IsValid() && v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil, fmt.Errorf("cannot iterate nil interface")
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil, fmt.Errorf("cannot iterate nil pointer")
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		limit := v.Int()
		if limit < 0 {
			limit = 0
		}
		return &runtimeIntIterator{limit: limit}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return &runtimeIntIterator{limit: int64(v.Uint())}, nil
	case reflect.Slice, reflect.Array:
		return &runtimeSliceIterator{value: v, inNext: inNext}, nil
	case reflect.Map:
		return &runtimeMapIterator{keys: v.MapKeys(), values: v}, nil
	case reflect.Chan:
		return &runtimeChanIterator{value: v}, nil
	case reflect.String:
		runes := []rune(v.String())
		return &runtimeSliceIterator{
			value:  reflect.ValueOf(runes),
			inNext: true,
		}, nil
	default:
		// container.Set / container.LinkedList (and similar wrappers) expose
		// ToSlice(); make them iterable like yakvm does. Try the original
		// receiver first (pointer methods) and the dereferenced value after.
		for _, candidate := range []reflect.Value{reflect.ValueOf(value), v} {
			if !candidate.IsValid() {
				continue
			}
			if m := candidate.MethodByName("ToSlice"); m.IsValid() && m.Type().NumIn() == 0 && m.Type().NumOut() == 1 && m.Type().Out(0).Kind() == reflect.Slice {
				res := m.Call(nil)
				return &runtimeSliceIterator{value: res[0], inNext: inNext}, nil
			}
		}
		return nil, fmt.Errorf("cannot iterate over %T", value)
	}
}

func runtimeDispatchNext(args []uint64, _ bool) (int64, error) {
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

func runtimeDispatchChanRecv(args []uint64, _ bool) (int64, error) {
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

func runtimeDispatchIn(args []uint64, _ bool) (int64, error) {
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
		if looksLikeCStringPointer(raw) {
			return runtimeCStringToGoString(ptr)
		}
		// Negative integers (e.g. -6 = 0xfffffffffffffffa) set the tag bit but
		// are not pointers. Restore the tag bit so the signed word survives
		// numeric comparison.
		return int64(raw | yakTaggedPointerMask)
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
	// yak semantics: an empty slice/map equals nil (and any other empty
	// container of the same kind): make([]string) == nil is true, while a
	// non-empty container is never nil.
	if runtimeContainerIsEmpty(left) && runtimeContainerIsEmpty(right) {
		return true
	}
	return false
}

// runtimeContainerIsEmpty reports whether a value is nil or an empty
// slice/map (including AOT's pointer-shadow representation of them).
func runtimeContainerIsEmpty(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	for rv.IsValid() && rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return true
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return true
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Array:
		return rv.Len() == 0
	case reflect.Ptr:
		if rv.IsNil() {
			return true
		}
		elem := rv.Elem()
		for elem.IsValid() && elem.Kind() == reflect.Interface {
			if elem.IsNil() {
				return true
			}
			elem = elem.Elem()
		}
		if elem.IsValid() {
			switch elem.Kind() {
			case reflect.Slice, reflect.Map, reflect.Array:
				return elem.Len() == 0
			case reflect.Struct:
				// AOT's ordered map shadow.
				if om, ok := elem.Interface().(runtimeOrderedMap); ok {
					return len(om.keys) == 0
				}
			}
		}
	}
	return false
}

func runtimeDispatchEq(args []uint64, _ bool) (int64, error) {
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

func runtimeDecodeChannel(raw uint64) (reflect.Value, bool) {
	value := runtimeDecodeIterValue(raw)
	rv := reflect.ValueOf(value)
	for rv.IsValid() && rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return reflect.Value{}, false
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() || rv.Kind() != reflect.Chan {
		return reflect.Value{}, false
	}
	return rv, true
}

func runtimeDispatchMakeChan(args []uint64, _ bool) (int64, error) {
	size := 0
	if len(args) > 0 {
		size = int(int64(args[0]))
	}
	if size < 0 {
		size = 0
	}
	ch := make(chan any, size)
	return int64(uintptr(newRuntimeShadow(ch))), nil
}

func runtimeDispatchChanSend(args []uint64, _ bool) (int64, error) {
	if len(args) < 2 {
		return 0, fmt.Errorf("chan send expects channel and value")
	}
	rv, ok := runtimeDecodeChannel(args[0])
	if !ok {
		return 0, fmt.Errorf("chan send on non-channel %T", runtimeDecodeIterValue(args[0]))
	}
	value := runtimeDecodeIterValue(args[1])
	rv.Send(reflect.ValueOf(value))
	return 0, nil
}
