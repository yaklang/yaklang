package main

/*
#include <stdint.h>
void yak_invoke_callable(uintptr_t fn, void* ctx);
*/
import "C"

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unsafe"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func runtimeCStringToGoString(ptr unsafe.Pointer) string {
	if ptr == nil {
		return ""
	}
	// to_cstring returns a shadow, not a C string, when the text contains a
	// NUL. Field and method names normally have none, but the same helper
	// reads both.
	if s, ok := tryResolveShadowString(ptr); ok {
		return s
	}
	s, ok := tryCString(uint64(uintptr(ptr)))
	if !ok {
		return ""
	}
	return s
}

func runtimeDispatchShadowMethod(args []uint64, ellipsis bool) (int64, error) {
	if len(args) < 2 {
		return 0, fmt.Errorf("runtime shadow method expects at least 2 args, got %d", len(args))
	}

	methodNamePtr := unsafe.Pointer(uintptr(args[1]))
	if methodNamePtr == nil {
		return 0, fmt.Errorf("runtime shadow method missing method name")
	}

	objPtr := unsafe.Pointer(uintptr(args[0]))
	if objPtr == nil {
		// A nil receiver (e.g. a dropped error left the object nil) reads as
		// nil instead of panicking, matching yak's weak-typed method calls.
		return 0, nil
	}

	return callRuntimeShadowMethod(objPtr, runtimeCStringToGoString(methodNamePtr), args[2:], ellipsis)
}

func runtimeResolveMethod(obj any, name string) (reflect.Value, error) {
	value := reflect.ValueOf(obj)
	if !value.IsValid() {
		return reflect.Value{}, fmt.Errorf("invalid object while resolving method %q", name)
	}

	if method := value.MethodByName(name); method.IsValid() {
		return method, nil
	}

	if f, ok := obj.(*os.File); ok {
		if method, ok := runtimeResolveOSFileMethod(f, name); ok {
			return method, nil
		}
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

	if value.IsValid() && value.Kind() == reflect.String {
		if method, ok := runtimeResolveStringMethod(value.String(), name); ok {
			return method, nil
		}
	}

	if value.IsValid() && (value.Kind() == reflect.Slice || value.Kind() == reflect.Array) {
		if method, ok := runtimeResolveSliceMethod(value, name); ok {
			return method, nil
		}
	}
	if value.IsValid() && value.Kind() == reflect.Ptr && !value.IsNil() {
		elem := value.Elem()
		if elem.IsValid() && (elem.Kind() == reflect.Slice || elem.Kind() == reflect.Array) {
			if method, ok := runtimeResolveSliceMethod(value, name); ok {
				return method, nil
			}
		}
	}

	// Yak calls string methods on []byte (poc.HTTP returns the request as
	// bytes, then the script does req.HasSuffix(...)). Slice methods win when
	// the name is one of theirs; everything else uses the string table.
	if text, ok := runtimeBytesAsString(value); ok {
		if method, ok := runtimeResolveStringMethod(text, name); ok {
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

// runtimeResolveOSFileMethod implements the yak file methods that os.File
// does not provide natively (WriteLine/ReadLines/Tell/...).
func runtimeResolveOSFileMethod(f *os.File, name string) (reflect.Value, bool) {
	if f == nil {
		return reflect.Value{}, false
	}
	switch name {
	case "WriteLine":
		return reflect.ValueOf(func(line any) (int, error) {
			return f.WriteString(fmt.Sprintf("%v\n", line))
		}), true
	case "ReadLines":
		return reflect.ValueOf(func() []string {
			lines := make([]string, 0)
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				lines = append(lines, sc.Text())
			}
			return lines
		}), true
	case "ReadLine":
		return reflect.ValueOf(func() (string, error) {
			sc := bufio.NewScanner(f)
			if sc.Scan() {
				return sc.Text(), nil
			}
			if err := sc.Err(); err != nil {
				return "", err
			}
			return "", io.EOF
		}), true
	case "Seek":
		return reflect.ValueOf(func(offset int64, whence int) (int64, error) {
			return f.Seek(offset, whence)
		}), true
	case "Tell":
		return reflect.ValueOf(func() (int64, error) {
			return f.Seek(0, io.SeekCurrent)
		}), true
	case "ReadAll":
		return reflect.ValueOf(func() ([]byte, error) { return io.ReadAll(f) }), true
	case "ReadString":
		return reflect.ValueOf(func() (string, error) {
			b, err := io.ReadAll(f)
			return string(b), err
		}), true
	case "Write":
		return reflect.ValueOf(func(data any) (int, error) {
			switch d := data.(type) {
			case string:
				return f.WriteString(d)
			case []byte:
				return f.Write(d)
			default:
				return f.WriteString(fmt.Sprint(d))
			}
		}), true
	case "WriteString":
		return reflect.ValueOf(func(s string) (int, error) { return f.WriteString(s) }), true
	case "Name":
		return reflect.ValueOf(func() string { return f.Name() }), true
	}
	return reflect.Value{}, false
}

// runtimeResolveSliceMethod implements the yak slice/array methods that the
// AOT runtime must provide (Go slices have no methods of their own). The
// receiver may be a *[]T shadow (yak reference semantics) or a plain []T.
func runtimeResolveSliceMethod(v reflect.Value, name string) (reflect.Value, bool) {
	ptr := reflect.Value{}
	for v.IsValid() && v.Kind() == reflect.Interface {
		if v.IsNil() {
			return reflect.Value{}, false
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return reflect.Value{}, false
		}
		ptr = v
		v = v.Elem()
	}
	if !v.IsValid() || (v.Kind() != reflect.Slice && v.Kind() != reflect.Array) {
		return reflect.Value{}, false
	}
	switch name {
	case "Append", "Push":
		return reflect.ValueOf(func(items ...any) any {
			elems := make([]reflect.Value, 0, len(items))
			for _, item := range items {
				elems = append(elems, runtimeSliceAppendValue(v.Type().Elem(), item))
			}
			appended := reflect.Append(v, elems...)
			if ptr.IsValid() {
				ptr.Elem().Set(appended)
				return ptr.Interface()
			}
			return appended.Interface()
		}), true
	case "Map":
		return reflect.ValueOf(func(fn any) []any {
			out := make([]any, 0, v.Len())
			for i := 0; i < v.Len(); i++ {
				out = append(out, runtimeCallMappedElement(fn, v.Index(i).Interface()))
			}
			return out
		}), true
	case "Length", "Len":
		return reflect.ValueOf(func() int { return v.Len() }), true
	case "Get", "At":
		return reflect.ValueOf(func(i int) any {
			if i < 0 || i >= v.Len() {
				return nil
			}
			return v.Index(i).Interface()
		}), true
	case "Set":
		return reflect.ValueOf(func(i int, item any) {
			if i < 0 || i >= v.Len() {
				return
			}
			converted, ok := valueForSet(v.Type().Elem(), int64(0))
			_ = converted
			_ = ok
			// Use the same conversion path as append for consistency.
			rv := runtimeSliceAppendValue(v.Type().Elem(), item)
			v.Index(i).Set(rv)
		}), true
	case "Contains":
		return reflect.ValueOf(func(item any) bool {
			// []byte.Contains("...") matches bytes.Contains semantics.
			if v.IsValid() && v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 {
				raw := v.Bytes()
				switch sub := item.(type) {
				case string:
					return bytes.Contains(raw, []byte(sub))
				case []byte:
					return bytes.Contains(raw, sub)
				}
			}
			for i := 0; i < v.Len(); i++ {
				if runtimeValuesEqual(v.Index(i).Interface(), item) {
					return true
				}
			}
			return false
		}), true
	}
	return reflect.Value{}, false
}

func runtimeResolveStringMethod(s string, name string) (reflect.Value, bool) {
	switch name {
	case "Trim":
		return reflect.ValueOf(func(cutset ...string) string {
			if len(cutset) == 0 {
				return strings.TrimSpace(s)
			}
			return strings.Trim(s, strings.Join(cutset, ""))
		}), true
	case "TrimLeft":
		return reflect.ValueOf(func(cutset ...string) string {
			if len(cutset) == 0 {
				return strings.TrimLeftFunc(s, unicode.IsSpace)
			}
			return strings.TrimLeft(s, strings.Join(cutset, ""))
		}), true
	case "TrimRight":
		return reflect.ValueOf(func(cutset ...string) string {
			if len(cutset) == 0 {
				return strings.TrimRightFunc(s, unicode.IsSpace)
			}
			return strings.TrimRight(s, strings.Join(cutset, ""))
		}), true
	case "Lower":
		return reflect.ValueOf(func() string { return strings.ToLower(s) }), true
	case "Upper":
		return reflect.ValueOf(func() string { return strings.ToUpper(s) }), true
	case "Contains":
		return reflect.ValueOf(func(substr string) bool { return substr == "" || strings.Contains(s, substr) }), true
	case "ReplaceAll", "Replace":
		return reflect.ValueOf(func(old, new string) string { return strings.ReplaceAll(s, old, new) }), true
	case "ReplaceN":
		return reflect.ValueOf(func(old, new string, n int) string { return strings.Replace(s, old, new, n) }), true
	case "HasPrefix", "StartsWith":
		return reflect.ValueOf(func(prefix string) bool { return strings.HasPrefix(s, prefix) }), true
	case "HasSuffix", "EndsWith":
		return reflect.ValueOf(func(suffix string) bool { return strings.HasSuffix(s, suffix) }), true
	case "RemovePrefix":
		return reflect.ValueOf(func(prefix string) string {
			return strings.TrimPrefix(s, prefix)
		}), true
	case "RemoveSuffix":
		return reflect.ValueOf(func(suffix string) string {
			return strings.TrimSuffix(s, suffix)
		}), true
	case "Split":
		return reflect.ValueOf(func(sep string) []string { return strings.Split(s, sep) }), true
	case "SplitN":
		return reflect.ValueOf(func(sep string, n int) []string { return strings.SplitN(s, sep, n) }), true
	case "Count":
		return reflect.ValueOf(func(substr string) int { return strings.Count(s, substr) }), true
	case "Find", "IndexOf":
		return reflect.ValueOf(func(substr string) int { return strings.Index(s, substr) }), true
	case "Rfind", "LastIndexOf":
		return reflect.ValueOf(func(substr string) int { return strings.LastIndex(s, substr) }), true
	case "Join":
		return reflect.ValueOf(func(items []any) string {
			strs := make([]string, 0, len(items))
			for _, item := range items {
				strs = append(strs, fmt.Sprint(item))
			}
			return strings.Join(strs, s)
		}), true
	default:
		return reflect.Value{}, false
	}
}

type runtimeCallableClosure struct {
	fn               uint64
	paramMemberCount int
	freeValues       []uint64
}

func runtimeDecodeArg(raw uint64, targetType reflect.Type) (reflect.Value, error) {
	if targetType == nil {
		return reflect.Value{}, fmt.Errorf("missing target type")
	}

	// Numeric targets: the raw word is the value itself. decodeTaggedArg's
	// C-string fallback would misread a large integer (e.g. a nanosecond
	// duration like 5e9) as a pointer and crash.
	switch targetType.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return reflect.ValueOf(int64(raw &^ yakTaggedPointerMask)).Convert(targetType), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return reflect.ValueOf(raw &^ yakTaggedPointerMask).Convert(targetType), nil
	case reflect.Float32, reflect.Float64:
		// The compiler encodes float constants as float64 bit patterns and int
		// constants as int64 words. Reinterpret the ORIGINAL word when it forms
		// a plausible float: clearing the tag bit first corrupts a real value
		// (3.14 becomes a denormal, and negatives become NaN). A float64 always
		// sets bit 62 in its exponent except for tiny magnitudes, so the tagged
		// branch is the common case and the untagged branch covers the rest.
		if f, ok := plausibleFloat(raw); ok {
			return reflect.ValueOf(f).Convert(targetType), nil
		}
		if f, ok := plausibleFloat(raw &^ yakTaggedPointerMask); ok {
			return reflect.ValueOf(f).Convert(targetType), nil
		}
		return reflect.ValueOf(float64(int64(raw &^ yakTaggedPointerMask))).Convert(targetType), nil
	case reflect.Bool:
		if v, ok := runtimeShadowValue(raw); ok {
			switch b := v.(type) {
			case bool:
				return reflect.ValueOf(b).Convert(targetType), nil
			default:
				if isRuntimeNilBox(v) {
					return reflect.ValueOf(false).Convert(targetType), nil
				}
			}
		}
		return reflect.ValueOf(raw&^yakTaggedPointerMask != 0).Convert(targetType), nil
	}

	// nil (0) is a valid value for nullable container targets. Interfaces are
	// intentionally excluded: a zero raw for an interface target must still go
	// through decodeTaggedArg so nil/error semantics stay unchanged.
	if raw == 0 {
		switch targetType.Kind() {
		case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Func, reflect.Chan:
			return reflect.Zero(targetType), nil
		}
	}

	if targetType.Kind() == reflect.Func {
		if fn, ok := runtimeDecodeCallableArg(raw, targetType); ok {
			return fn, nil
		}
	}

	decoded := decodeTaggedArg(raw)
	if decoded == nil {
		return reflect.Zero(targetType), nil
	}

	if intValue, ok := decoded.(int64); ok {
		if shadowValue, ok := runtimeDecodeShadowArg(raw, targetType); ok {
			return shadowValue, nil
		}
		if converted, ok := valueForSet(targetType, intValue); ok {
			return converted, nil
		}
	}

	value := reflect.ValueOf(decoded)
	if value.IsValid() {
		if targetType.Kind() == reflect.Interface {
			// Yak maps are AOT-internal runtimeOrderedMap values; yaklib
			// expects real Go maps (utils.IsMap / range), so convert before
			// crossing the interface boundary.
			if om, ok := decoded.(*runtimeOrderedMap); ok && om != nil {
				if converted, ok := convertMapValue(reflect.ValueOf(om), reflect.TypeOf(map[string]any{})); ok {
					return converted, nil
				}
			}
			// A closure passed as any stays the runtime struct. append stores
			// that struct in the slice and yak calls it later. Wrapping it as
			// a Go func here breaks per-iteration loop closures. Map elements
			// are wrapped in convertMapValue, which is what the sandbox calls.
			// AOT slice/map shadows are pointers; yak code sees them as
			// values, so pass the dereferenced container to yaklib before the
			// generic assignable-to-interface path.
			if value.Kind() == reflect.Ptr && !value.IsNil() {
				elem := value.Elem()
				if elem.IsValid() && elem.Kind() == reflect.Slice {
					return runtimeMaterializeSliceElements(elem), nil
				}
				if elem.IsValid() && elem.Kind() == reflect.Map {
					return elem, nil
				}
			}
			if value.Type().Implements(targetType) {
				return value, nil
			}
		}
		if value.Type().AssignableTo(targetType) {
			return value, nil
		}
		// Integers are convertible to string, but that conversion is a Unicode
		// code point: 123 becomes "{". Yak wants the decimal text. Do that
		// before Convert so floats (which are not convertible) and integers
		// share one path.
		if targetType.Kind() == reflect.String {
			if text, ok := runtimeWeakString(decoded); ok {
				return reflect.ValueOf(text).Convert(targetType), nil
			}
		}
		if value.Type().ConvertibleTo(targetType) {
			return value.Convert(targetType), nil
		}
		if targetType.Kind() == reflect.Ptr && value.Kind() != reflect.Ptr && value.CanAddr() && value.Addr().Type().AssignableTo(targetType) {
			return value.Addr(), nil
		}
		if value.Kind() == reflect.Ptr && !value.IsNil() && value.Elem().Type().AssignableTo(targetType) {
			return value.Elem(), nil
		}
		if targetType.Kind() == reflect.Map {
			if converted, ok := convertMapValue(value, targetType); ok {
				return converted, nil
			}
		}
		if targetType.Kind() == reflect.Slice {
			if converted, ok := convertSliceValue(runtimeSliceForConvert(value), targetType); ok {
				return converted, nil
			}
		}
	}

	return reflect.Value{}, fmt.Errorf("cannot use %T as %s", decoded, targetType)
}

// runtimeBytesAsString views a byte slice or byte array as text so string
// methods apply. Non-byte containers return false and keep their own methods.
func runtimeBytesAsString(value reflect.Value) (string, bool) {
	if !value.IsValid() {
		return "", false
	}
	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return "", false
		}
		value = value.Elem()
	}
	if !value.IsValid() || (value.Kind() != reflect.Slice && value.Kind() != reflect.Array) {
		return "", false
	}
	if value.Type().Elem().Kind() != reflect.Uint8 {
		return "", false
	}
	buf := make([]byte, value.Len())
	reflect.Copy(reflect.ValueOf(buf), value)
	return string(buf), true
}

// runtimeWeakString is the yak coercion into a string parameter: decimals for
// integers, the shortest fixed-point form for floats, and the raw bytes of a
// byte slice. Other values stay typed so a real mismatch still errors.
func runtimeWeakString(decoded any) (string, bool) {
	switch v := decoded.(type) {
	case string:
		return v, true
	case []byte:
		return string(v), true
	case bool:
		return strconv.FormatBool(v), true
	case int:
		return strconv.FormatInt(int64(v), 10), true
	case int8:
		return strconv.FormatInt(int64(v), 10), true
	case int16:
		return strconv.FormatInt(int64(v), 10), true
	case int32:
		return strconv.FormatInt(int64(v), 10), true
	case int64:
		return strconv.FormatInt(v, 10), true
	case uint:
		return strconv.FormatUint(uint64(v), 10), true
	case uint8:
		return strconv.FormatUint(uint64(v), 10), true
	case uint16:
		return strconv.FormatUint(uint64(v), 10), true
	case uint32:
		return strconv.FormatUint(uint64(v), 10), true
	case uint64:
		return strconv.FormatUint(v, 10), true
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32), true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	default:
		return "", false
	}
}

func runtimeDecodeCallableArg(raw uint64, targetType reflect.Type) (reflect.Value, bool) {
	if targetType == nil || targetType.Kind() != reflect.Func {
		return reflect.Value{}, false
	}
	if (raw & yakTaggedPointerMask) != 0 {
		raw &^= yakTaggedPointerMask
	}
	ptr := unsafe.Pointer(uintptr(raw))
	if ptr == nil {
		return reflect.Zero(targetType), true
	}
	if handle, ok := handleFromShadow(ptr); ok {
		handleValue := handle.Value()
		if closure, ok := runtimeCallableClosureValue(handleValue); ok {
			return runtimeMakeCallableWrapper(closure.fn, closure.paramMemberCount, closure.freeValues, targetType), true
		}
		value := reflect.ValueOf(handleValue)
		if value.IsValid() && value.Type().AssignableTo(targetType) {
			return value, true
		}
		if value.IsValid() && value.Type().ConvertibleTo(targetType) {
			return value.Convert(targetType), true
		}
		// The shadow is a heap object, not machine code. Jumping to its
		// address (the previous fallthrough) is the crawlerx SIGSEGV.
		return reflect.Value{}, false
	}
	if raw == abi.YaklibExportCallableMarker || runtimeAddrExecutable(uintptr(raw)) {
		return runtimeMakeCallableWrapper(raw, 0, nil, targetType), true
	}
	return reflect.Value{}, false
}

type runtimeExecRange struct {
	start uintptr
	end   uintptr
}

var runtimeExecRanges struct {
	once        sync.Once
	unavailable bool
	ranges      []runtimeExecRange
}

// runtimeAddrExecutable reports whether addr is in an executable mapping.
// A yak function pointer is in the binary text; a shadow or Go heap pointer
// is not, and yak_invoke_callable would SIGSEGV there.
func runtimeAddrExecutable(addr uintptr) bool {
	if addr < 4096 {
		return false
	}
	runtimeExecRanges.once.Do(runtimeLoadExecRanges)
	if runtimeExecRanges.unavailable {
		return true
	}
	for _, r := range runtimeExecRanges.ranges {
		if addr >= r.start && addr < r.end {
			return true
		}
	}
	return false
}

func runtimeLoadExecRanges() {
	data, err := os.ReadFile("/proc/self/maps")
	if err != nil {
		runtimeExecRanges.unavailable = true
		return
	}
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		fields := bytes.Fields(line)
		if len(fields) < 2 || !bytes.Contains(fields[1], []byte{'x'}) {
			continue
		}
		bounds := fields[0]
		dash := bytes.IndexByte(bounds, '-')
		if dash <= 0 || dash+1 >= len(bounds) {
			continue
		}
		start, err1 := strconv.ParseUint(string(bounds[:dash]), 16, 64)
		end, err2 := strconv.ParseUint(string(bounds[dash+1:]), 16, 64)
		if err1 != nil || err2 != nil || end <= start {
			continue
		}
		runtimeExecRanges.ranges = append(runtimeExecRanges.ranges, runtimeExecRange{
			start: uintptr(start),
			end:   uintptr(end),
		})
	}
}

func runtimeCallableClosureValue(value any) (runtimeCallableClosure, bool) {
	switch closure := value.(type) {
	case runtimeCallableClosure:
		return closure, true
	case *runtimeCallableClosure:
		if closure != nil {
			return *closure, true
		}
	}
	return runtimeCallableClosure{}, false
}

func runtimeMakeCallableWrapper(raw uint64, paramMemberCount int, freeValues []uint64, targetType reflect.Type) reflect.Value {
	captures := append([]uint64(nil), freeValues...)
	return reflect.MakeFunc(targetType, func(args []reflect.Value) []reflect.Value {
		if raw == abi.YaklibExportCallableMarker && len(captures) >= 2 {
			callArgs := make([]uint64, 0, len(args)+2)
			callArgs = append(callArgs, captures[0], captures[1])
			for _, arg := range args {
				callArgs = append(callArgs, uint64(runtimeValueToInt64(arg)))
			}
			ret, err := runtimeDispatchYaklibCall(callArgs, false)
			if err != nil {
				panic(err)
			}
			return runtimeDecodeYaklibExportReturns(ret, targetType)
		}
		paramc := len(args)
		argc := paramc + paramMemberCount + len(captures)
		words := make([]uint64, abi.HeaderWords+argc*2)
		ctx := unsafe.Pointer(&words[0])
		ctxInit(ctx, abi.KindCallable, raw, argc)
		for i, arg := range args {
			rawArg := uint64(runtimeValueToInt64(arg))
			runtimeStoreCallableContextArg(ctx, argc, i, rawArg)
		}
		for i := 0; i < paramMemberCount; i++ {
			runtimeStoreCallableContextArg(ctx, argc, paramc+i, 0)
		}
		for i, capture := range captures {
			runtimeStoreCallableContextArg(ctx, argc, paramc+paramMemberCount+i, capture)
		}

		C.yak_invoke_callable(C.uintptr_t(raw), ctx)
		return runtimeDecodeCallableReturns(ctx, targetType)
	})
}

func runtimeStoreCallableContextArg(ctx unsafe.Pointer, argc int, index int, raw uint64) {
	ctxStoreWord(ctx, abi.HeaderWords+index, raw)
	ctxStoreWord(ctx, abi.HeaderWords+argc+index, raw&^yakTaggedPointerMask)
}

// runtimeDecodeYaklibExportReturns decodes a yaklib export return value into
// the Go func wrapper's result values. A multi-return is one tuple word.
func runtimeDecodeYaklibExportReturns(ret int64, targetType reflect.Type) []reflect.Value {
	if targetType == nil || targetType.NumOut() == 0 {
		return nil
	}
	if targetType.NumOut() > 1 {
		if elems, abiWords, ok := runtimeReturnTuple(uint64(ret)); ok {
			return runtimeFillMultiReturns(elems, abiWords, targetType)
		}
	}
	out := make([]reflect.Value, targetType.NumOut())
	for i := range out {
		if i == 0 {
			if value, err := runtimeDecodeArg(uint64(ret), targetType.Out(i)); err == nil {
				out[i] = value
				continue
			}
		}
		out[i] = reflect.Zero(targetType.Out(i))
	}
	return out
}

func runtimeDecodeCallableReturns(ctx unsafe.Pointer, targetType reflect.Type) []reflect.Value {
	if targetType == nil || targetType.NumOut() == 0 {
		return nil
	}

	out := make([]reflect.Value, targetType.NumOut())
	ret := ctxLoadWord(ctx, abi.WordRet)
	// A yak `return a, b` is one *[]any word. Filling only result 0 leaves the
	// Go error (or the second any) nil, so `assert err.Error() != nil` fails
	// even though the script returned a message.
	if targetType.NumOut() > 1 {
		if elems, abiWords, ok := runtimeReturnTuple(ret); ok {
			return runtimeFillMultiReturns(elems, abiWords, targetType)
		}
	}
	for i := range out {
		if i == 0 {
			if value, err := runtimeDecodeArg(ret, targetType.Out(i)); err == nil {
				out[i] = value
				continue
			}
		}
		out[i] = reflect.Zero(targetType.Out(i))
	}
	return out
}

func runtimeReturnTuple(raw uint64) (elems []any, abiWords bool, ok bool) {
	if raw == 0 {
		return nil, false, false
	}
	ptr := unsafe.Pointer(uintptr(raw &^ yakTaggedPointerMask))
	h, found := handleFromShadow(ptr)
	if !found {
		return nil, false, false
	}
	return runtimeReturnTupleValue(h.Value())
}

// runtimeReturnTupleValue splits a return tuple. Compiler returns are *[]any
// whose elements are ABI words (set_field into interface{}). Go multi-returns
// are []any of already-decoded values.
func runtimeReturnTupleValue(v any) (elems []any, abiWords bool, ok bool) {
	switch t := v.(type) {
	case []any:
		return t, false, true
	case *[]any:
		if t == nil {
			return nil, false, false
		}
		return *t, true, true
	default:
		return nil, false, false
	}
}

func runtimeFillMultiReturns(elems []any, abiWords bool, targetType reflect.Type) []reflect.Value {
	out := make([]reflect.Value, targetType.NumOut())
	for i := range out {
		var elem any
		if i < len(elems) {
			elem = elems[i]
		}
		value, err := runtimeDecodeReturnElement(elem, targetType.Out(i), abiWords)
		if err != nil {
			out[i] = reflect.Zero(targetType.Out(i))
			continue
		}
		out[i] = value
	}
	return out
}

func runtimeDecodeReturnElement(elem any, targetType reflect.Type, abiWords bool) (reflect.Value, error) {
	if elem == nil {
		return reflect.Zero(targetType), nil
	}
	if abiWords {
		if word, ok := runtimeAnyABIWord(elem); ok {
			if word == 0 {
				return reflect.Zero(targetType), nil
			}
			errType := reflect.TypeOf((*error)(nil)).Elem()
			if targetType == errType {
				decoded := decodeTaggedArg(word)
				if decoded == nil {
					return reflect.Zero(targetType), nil
				}
				switch v := decoded.(type) {
				case error:
					if v == nil {
						return reflect.Zero(targetType), nil
					}
					return reflect.ValueOf(v), nil
				case string:
					return reflect.ValueOf(fmt.Errorf("%s", v)), nil
				}
			}
			return runtimeDecodeArg(word, targetType)
		}
	}
	return runtimeAssignReturnValue(elem, targetType)
}

func runtimeAnyABIWord(v any) (uint64, bool) {
	switch n := v.(type) {
	case int:
		return uint64(n), true
	case int64:
		return uint64(n), true
	case uint64:
		return n, true
	case uintptr:
		return uint64(n), true
	default:
		return 0, false
	}
}

func runtimeAssignReturnValue(elem any, targetType reflect.Type) (reflect.Value, error) {
	if elem == nil || targetType == nil {
		return reflect.Zero(targetType), nil
	}
	value := reflect.ValueOf(elem)
	if !value.IsValid() {
		return reflect.Zero(targetType), nil
	}
	if value.Kind() == reflect.Ptr && !value.IsNil() {
		inner := value.Elem()
		if inner.IsValid() && inner.Kind() == reflect.Slice {
			inner = runtimeMaterializeSliceElements(inner)
			if targetType.Kind() == reflect.Interface || inner.Type().AssignableTo(targetType) {
				return inner, nil
			}
			if converted, ok := convertSliceValue(inner, targetType); ok {
				return converted, nil
			}
			value = inner
		}
	}
	errType := reflect.TypeOf((*error)(nil)).Elem()
	if targetType == errType {
		switch v := elem.(type) {
		case error:
			if v == nil {
				return reflect.Zero(targetType), nil
			}
			return reflect.ValueOf(v), nil
		case string:
			return reflect.ValueOf(fmt.Errorf("%s", v)), nil
		}
	}
	if value.Type().AssignableTo(targetType) {
		return value, nil
	}
	if targetType.Kind() == reflect.Interface && value.Type().Implements(targetType) {
		return value, nil
	}
	if value.Type().ConvertibleTo(targetType) {
		return value.Convert(targetType), nil
	}
	return reflect.Value{}, fmt.Errorf("cannot use %T as %s", elem, targetType)
}

// runtimeMaterializeSliceElements decodes ABI words stored in a []any so a Go
// caller sees the string or slice, not the pointer's integer spelling.
func runtimeMaterializeSliceElements(slice reflect.Value) reflect.Value {
	if !slice.IsValid() || slice.Kind() != reflect.Slice {
		return slice
	}
	if slice.Type().Elem().Kind() != reflect.Interface {
		return slice
	}
	out := reflect.MakeSlice(slice.Type(), slice.Len(), slice.Len())
	changed := false
	for i := 0; i < slice.Len(); i++ {
		elem := slice.Index(i)
		if !elem.IsValid() || (elem.Kind() == reflect.Interface && elem.IsNil()) {
			continue
		}
		concrete := elem
		if concrete.Kind() == reflect.Interface {
			concrete = concrete.Elem()
		}
		word, isWord := uint64(0), false
		if concrete.IsValid() {
			switch concrete.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				word = uint64(concrete.Int())
				isWord = true
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
				word = concrete.Uint()
				isWord = true
			}
		}
		if isWord && word != 0 {
			decoded := decodeTaggedArg(word)
			if decodedInt, ok := decoded.(int64); !ok || uint64(decodedInt) != word {
				if decoded != nil {
					out.Index(i).Set(reflect.ValueOf(decoded))
					changed = true
					continue
				}
			}
		}
		if elem.Type().AssignableTo(out.Type().Elem()) {
			out.Index(i).Set(elem)
		}
	}
	if !changed {
		return slice
	}
	return out
}

func runtimeSliceForConvert(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	if value.Kind() == reflect.Ptr && !value.IsNil() {
		elem := value.Elem()
		if elem.IsValid() && elem.Kind() == reflect.Slice {
			return runtimeMaterializeSliceElements(elem)
		}
	}
	if value.Kind() == reflect.Slice {
		return runtimeMaterializeSliceElements(value)
	}
	return value
}

func runtimeTupleTailIsError(last any, abiWords bool) (error, bool) {
	if !abiWords {
		if last == nil {
			return nil, true
		}
		err, ok := last.(error)
		return err, ok
	}
	word, ok := runtimeAnyABIWord(last)
	if !ok {
		if last == nil {
			return nil, true
		}
		err, isErr := last.(error)
		return err, isErr
	}
	if word == 0 {
		return nil, true
	}
	decoded := decodeTaggedArg(word)
	if decoded == nil {
		return nil, true
	}
	if err, isErr := decoded.(error); isErr {
		return err, true
	}
	return nil, false
}

func runtimeDecodeShadowArg(raw uint64, targetType reflect.Type) (reflect.Value, bool) {
	ptr := unsafe.Pointer(uintptr(raw))
	if ptr == nil {
		return reflect.Value{}, false
	}

	handle, ok := handleFromShadow(ptr)
	if !ok {
		return reflect.Value{}, false
	}

	value := reflect.ValueOf(handle.Value())
	if !value.IsValid() {
		return reflect.Zero(targetType), true
	}
	if value.Type().AssignableTo(targetType) {
		return value, true
	}
	if value.Type().ConvertibleTo(targetType) {
		return value.Convert(targetType), true
	}
	if targetType.Kind() == reflect.Interface && value.Type().Implements(targetType) {
		return value, true
	}
	if targetType.Kind() == reflect.Ptr && value.Kind() != reflect.Ptr && value.CanAddr() && value.Addr().Type().AssignableTo(targetType) {
		return value.Addr(), true
	}
	if targetType.Kind() == reflect.Map {
		if converted, ok := convertMapValue(value, targetType); ok {
			return converted, true
		}
	}
	if targetType.Kind() == reflect.Slice {
		if converted, ok := convertSliceValue(value, targetType); ok {
			return converted, true
		}
	}
	return reflect.Value{}, false
}

// convertMapValue converts an AOT runtimeOrderedMap (or a Go map) into the
// target map type yaklib expects (e.g. map[string]interface{}), copying keys
// and values element-wise.
func convertMapValue(value reflect.Value, targetType reflect.Type) (reflect.Value, bool) {
	if !value.IsValid() || targetType == nil || targetType.Kind() != reflect.Map {
		return reflect.Value{}, false
	}
	var keys []string
	var values map[string]any
	if om, ok := value.Interface().(*runtimeOrderedMap); ok && om != nil {
		keys = om.keys
		values = om.values
	} else if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return reflect.Value{}, false
		}
		if om, ok := value.Interface().(*runtimeOrderedMap); ok && om != nil {
			keys = om.keys
			values = om.values
		} else {
			value = value.Elem()
			if !value.IsValid() || value.Kind() != reflect.Map {
				return reflect.Value{}, false
			}
			keys = make([]string, 0, value.Len())
			values = make(map[string]any, value.Len())
			iter := value.MapRange()
			for iter.Next() {
				k := fmt.Sprint(iter.Key().Interface())
				keys = append(keys, k)
				values[k] = iter.Value().Interface()
			}
		}
	} else if value.Kind() == reflect.Map {
		keys = make([]string, 0, value.Len())
		values = make(map[string]any, value.Len())
		iter := value.MapRange()
		for iter.Next() {
			k := fmt.Sprint(iter.Key().Interface())
			keys = append(keys, k)
			values[k] = iter.Value().Interface()
		}
	} else {
		return reflect.Value{}, false
	}

	keyType := targetType.Key()
	elemType := targetType.Elem()
	out := reflect.MakeMapWithSize(targetType, len(keys))
	for _, k := range keys {
		kv := reflect.ValueOf(k)
		if !kv.Type().AssignableTo(keyType) {
			if !kv.Type().ConvertibleTo(keyType) {
				return reflect.Value{}, false
			}
			kv = kv.Convert(keyType)
		}
		ev := runtimeBoundaryMapElement(values[k], elemType)
		if !ev.IsValid() {
			ev = reflect.Zero(elemType)
		}
		if !ev.Type().AssignableTo(elemType) {
			if !ev.Type().ConvertibleTo(elemType) {
				return reflect.Value{}, false
			}
			ev = ev.Convert(elemType)
		}
		out.SetMapIndex(kv, ev)
	}
	return out, true
}

// runtimeBoundaryFuncForClosure turns a yak closure into a Go func of
// elemType, or a variadic func(...any) any when the destination is an
// interface. The sandbox VM calls native funcs; it cannot call the runtime
// closure struct.
func runtimeBoundaryFuncForClosure(closure runtimeCallableClosure, elemType reflect.Type) reflect.Value {
	fnType := elemType
	if fnType == nil || fnType.Kind() != reflect.Func {
		fnType = reflect.TypeOf(func(...any) any { return nil })
	}
	return runtimeMakeCallableWrapper(closure.fn, closure.paramMemberCount, closure.freeValues, fnType)
}

// runtimeBoundaryMapElement prepares one map value for a Go map. Closures
// become callable funcs, and nested ordered maps become Go maps.
func runtimeBoundaryMapElement(raw any, elemType reflect.Type) reflect.Value {
	if closure, ok := runtimeCallableClosureValue(raw); ok {
		return runtimeBoundaryFuncForClosure(closure, elemType)
	}
	if om, ok := raw.(*runtimeOrderedMap); ok && om != nil && elemType != nil &&
		(elemType.Kind() == reflect.Interface || elemType.Kind() == reflect.Map) {
		nested := elemType
		if nested.Kind() == reflect.Interface {
			nested = reflect.TypeOf(map[string]any{})
		}
		if converted, ok := convertMapValue(reflect.ValueOf(om), nested); ok {
			return converted
		}
	}
	return reflect.ValueOf(raw)
}

// convertSliceValue converts a Go slice (usually []any) into the target slice
// type yaklib expects, converting elements one by one.
func convertSliceValue(value reflect.Value, targetType reflect.Type) (reflect.Value, bool) {
	if !value.IsValid() || targetType == nil || targetType.Kind() != reflect.Slice {
		return reflect.Value{}, false
	}
	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return reflect.Value{}, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Slice {
		return reflect.Value{}, false
	}
	if value.Type().AssignableTo(targetType) {
		return value, true
	}
	elemType := targetType.Elem()
	out := reflect.MakeSlice(targetType, value.Len(), value.Len())
	for i := 0; i < value.Len(); i++ {
		elem := value.Index(i)
		for elem.IsValid() && elem.Kind() == reflect.Interface {
			if elem.IsNil() {
				elem = reflect.Zero(elemType)
				break
			}
			elem = elem.Elem()
		}
		if !elem.IsValid() {
			out.Index(i).Set(reflect.Zero(elemType))
			continue
		}
		if elem.Type().AssignableTo(elemType) {
			out.Index(i).Set(elem)
			continue
		}
		if elem.Type().ConvertibleTo(elemType) {
			out.Index(i).Set(elem.Convert(elemType))
			continue
		}
		if elemType.Kind() == reflect.String {
			out.Index(i).SetString(fmt.Sprint(elem.Interface()))
			continue
		}
		return reflect.Value{}, false
	}
	return out, true
}

func convertSliceForVariadicCall(val reflect.Value, targetSliceType reflect.Type) (reflect.Value, bool) {
	if !val.IsValid() || val.Kind() != reflect.Slice || targetSliceType == nil || targetSliceType.Kind() != reflect.Slice {
		return reflect.Value{}, false
	}
	if val.Type().AssignableTo(targetSliceType) {
		return val, true
	}
	elemType := targetSliceType.Elem()
	out := reflect.MakeSlice(targetSliceType, val.Len(), val.Len())
	for i := 0; i < val.Len(); i++ {
		elem := val.Index(i)
		for elem.IsValid() && elem.Kind() == reflect.Interface {
			if elem.IsNil() {
				elem = reflect.Zero(elemType)
				break
			}
			elem = elem.Elem()
		}
		if !elem.IsValid() {
			out.Index(i).Set(reflect.Zero(elemType))
			continue
		}
		if elem.Type().AssignableTo(elemType) {
			out.Index(i).Set(elem)
			continue
		}
		if elem.Type().ConvertibleTo(elemType) {
			out.Index(i).Set(elem.Convert(elemType))
			continue
		}
		if elemType.Kind() == reflect.String {
			out.Index(i).SetString(fmt.Sprint(elem.Interface()))
			continue
		}
		return reflect.Value{}, false
	}
	return out, true
}

func runtimeDecodeCallArgs(target reflect.Value, rawArgs []uint64, ellipsis bool) ([]reflect.Value, error) {
	methodType := target.Type()
	if !methodType.IsVariadic() {
		if len(rawArgs) != methodType.NumIn() {
			return nil, fmt.Errorf("method expects %d args, got %d", methodType.NumIn(), len(rawArgs))
		}
		args := make([]reflect.Value, 0, len(rawArgs))
		for index, raw := range rawArgs {
			arg, err := runtimeDecodeArg(raw, methodType.In(index))
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
		}
		return args, nil
	}

	fixedCount := methodType.NumIn() - 1
	variadicSliceType := methodType.In(fixedCount)
	variadicElemType := variadicSliceType.Elem()

	decodeFixed := func() ([]reflect.Value, error) {
		out := make([]reflect.Value, 0, fixedCount)
		for i := 0; i < fixedCount; i++ {
			arg, err := runtimeDecodeArg(rawArgs[i], methodType.In(i))
			if err != nil {
				return nil, err
			}
			out = append(out, arg)
		}
		return out, nil
	}

	if len(rawArgs) < fixedCount {
		return nil, fmt.Errorf("method expects at least %d args, got %d", fixedCount, len(rawArgs))
	}

	fixed, err := decodeFixed()
	if err != nil {
		return nil, err
	}

	variadicArgs := rawArgs[fixedCount:]

	// `f(slice...)` unpacks the slice into variadic elements; `f(slice)`
	// passes the slice as a single element. The compiler marks ellipsis calls
	// with FlagEllipsis so the two are distinguishable at runtime.
	if ellipsis && len(variadicArgs) == 1 {
		if sliceVal, ok := runtimeDecodeVariadicSliceValue(variadicArgs[0]); ok {
			elems := make([]reflect.Value, 0, sliceVal.Len())
			for i := 0; i < sliceVal.Len(); i++ {
				e := sliceVal.Index(i)
				for e.IsValid() && e.Kind() == reflect.Interface {
					if e.IsNil() {
						e = reflect.Zero(variadicElemType)
						break
					}
					e = e.Elem()
				}
				if e.IsValid() && e.Kind() == reflect.Ptr && !e.IsNil() {
					ee := e.Elem()
					if ee.IsValid() && (ee.Kind() == reflect.Slice || ee.Kind() == reflect.Map) {
						e = ee
					}
				}
				e = runtimePrepareVariadicElement(e, variadicElemType)
				if !e.IsValid() || !e.Type().AssignableTo(variadicElemType) {
					return nil, fmt.Errorf("cannot use %s as %s", e.Type(), variadicElemType)
				}
				elems = append(elems, e)
			}
			slice := reflect.MakeSlice(variadicSliceType, len(elems), len(elems))
			for i, elem := range elems {
				slice.Index(i).Set(elem)
			}
			return append(fixed, slice), nil
		}
	}

	if len(variadicArgs) == 0 {
		return append(fixed, reflect.Zero(variadicSliceType)), nil
	}

	elems := make([]reflect.Value, 0, len(variadicArgs))
	for _, raw := range variadicArgs {
		arg, err := runtimeDecodeArg(raw, variadicElemType)
		if err != nil {
			return nil, err
		}
		elems = append(elems, arg)
	}
	slice := reflect.MakeSlice(variadicSliceType, len(elems), len(elems))
	for i, elem := range elems {
		slice.Index(i).Set(elem)
	}
	return append(fixed, slice), nil
}

// runtimeDecodeVariadicSliceValue resolves a single raw argument that is an
// AOT slice shadow (*[]T) into its underlying slice reflect.Value.
func runtimeDecodeVariadicSliceValue(raw uint64) (reflect.Value, bool) {
	ptr := unsafe.Pointer(uintptr(raw &^ yakTaggedPointerMask))
	if ptr == nil {
		return reflect.Value{}, false
	}
	handle, ok := handleFromShadow(ptr)
	if !ok {
		return reflect.Value{}, false
	}
	value := reflect.ValueOf(handle.Value())
	for value.IsValid() && value.Kind() == reflect.Interface {
		if value.IsNil() {
			return reflect.Value{}, false
		}
		value = value.Elem()
	}
	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return reflect.Value{}, false
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Slice {
		return reflect.Value{}, false
	}
	return value, true
}

func runtimeCallReturnValue(results []reflect.Value) int64 {
	if len(results) == 0 {
		return 0
	}

	if len(results) == 1 {
		return runtimeValueToInt64(results[0])
	}

	// Multi-return Yak calls unpack via tuple index ("0", "1", ...). Box every
	// result into a []any shadow so runtime get_field can read each slot.
	tuple := make([]any, len(results))
	for i, r := range results {
		if !r.IsValid() || (r.Kind() == reflect.Interface && r.IsNil()) {
			tuple[i] = nil
			continue
		}
		tuple[i] = r.Interface()
	}
	return int64(uintptr(newRuntimeShadow(tuple)))
}

func callRuntimeValue(target reflect.Value, rawArgs []uint64, ellipsis bool) (int64, error) {
	args, err := runtimeDecodeCallArgs(target, rawArgs, ellipsis)
	if err != nil {
		return 0, err
	}
	methodType := target.Type()
	if methodType.IsVariadic() && len(args) > 0 {
		last := args[len(args)-1]
		unwrapped := last
		for unwrapped.IsValid() && unwrapped.Kind() == reflect.Interface {
			if unwrapped.IsNil() {
				break
			}
			unwrapped = unwrapped.Elem()
		}
		if unwrapped.IsValid() && unwrapped.Kind() == reflect.Ptr && !unwrapped.IsNil() {
			unwrapped = unwrapped.Elem()
		}
		if unwrapped.IsValid() && unwrapped.Kind() == reflect.Slice {
			// For interface{} variadic targets, unwrap AOT slice/map shadow
			// elements so yaklib sees the container values, not pointers.
			if methodType.NumIn() > 0 && methodType.In(methodType.NumIn()-1).Kind() == reflect.Interface {
				for i := 0; i < unwrapped.Len(); i++ {
					e := unwrapped.Index(i)
					for e.IsValid() && e.Kind() == reflect.Interface {
						if e.IsNil() {
							break
						}
						e = e.Elem()
					}
					if e.IsValid() && e.Kind() == reflect.Ptr && !e.IsNil() {
						ee := e.Elem()
						if ee.IsValid() && (ee.Kind() == reflect.Slice || ee.Kind() == reflect.Map) {
							unwrapped.Index(i).Set(ee)
						}
					}
				}
			}
			args[len(args)-1] = unwrapped
			return runtimeCallReturnValue(target.CallSlice(args)), nil
		}
	}
	return runtimeCallReturnValue(target.Call(args)), nil
}

// runtimeDurationMethod resolves methods of a raw time.Duration word.
// time.Duration is an int64 nanosecond count, so the compiler emits a shadow
// method call on the integer itself. Positive durations below ~146 years leave
// the tag bit clear; every negative duration is sign-extended and therefore
// has the tag bit set, so both shapes are accepted here. A tagged pointer to a
// real object has bit 62 set and bit 63 clear and does not take this path.
func runtimeDurationMethod(d time.Duration, name string) (reflect.Value, bool) {
	switch name {
	case "Nanoseconds":
		return reflect.ValueOf(func() int64 { return d.Nanoseconds() }), true
	case "Microseconds":
		return reflect.ValueOf(func() int64 { return d.Microseconds() }), true
	case "Milliseconds":
		return reflect.ValueOf(func() int64 { return d.Milliseconds() }), true
	case "Seconds":
		return reflect.ValueOf(func() float64 { return d.Seconds() }), true
	case "Minutes":
		return reflect.ValueOf(func() float64 { return d.Minutes() }), true
	case "Hours":
		return reflect.ValueOf(func() float64 { return d.Hours() }), true
	case "Abs":
		return reflect.ValueOf(func() time.Duration { return d.Abs() }), true
	case "String":
		return reflect.ValueOf(func() string { return d.String() }), true
	default:
		return reflect.Value{}, false
	}
}

// callRuntimeShadowMethod forwards ellipsis. bruter.Start(targets...) is a
// method call whose one argument is the yak slice (*[]any). Dropping the flag
// decodes that slice as a single string ("cannot use *[]interface {} as string").
func callRuntimeShadowMethod(objPtr unsafe.Pointer, methodName string, rawArgs []uint64, ellipsis bool) (int64, error) {
	handle, ok := handleFromShadow(objPtr)
	if !ok {
		// String receivers are passed as C-string pointers (or string shadows)
		// rather than shadow handles; resolve the string method directly.
		if s, ok := tryResolveShadowString(objPtr); ok {
			if method, ok := runtimeResolveStringMethod(s, methodName); ok {
				return callRuntimeValue(method, rawArgs, ellipsis)
			}
			return 0, fmt.Errorf("method %q not found on string", methodName)
		}
		word := uint64(uintptr(objPtr))
		if int64(word) < 0 || word&yakTaggedPointerMask == 0 {
			if method, ok := runtimeDurationMethod(time.Duration(int64(word)), methodName); ok {
				return callRuntimeValue(method, rawArgs, ellipsis)
			}
		}
		raw := word &^ yakTaggedPointerMask
		if s, ok := tryCString(raw); ok {
			if method, ok := runtimeResolveStringMethod(s, methodName); ok {
				return callRuntimeValue(method, rawArgs, ellipsis)
			}
			return 0, fmt.Errorf("method %q not found on string", methodName)
		}
		return 0, fmt.Errorf("invalid shadow object for method %q", methodName)
	}

	method, err := runtimeResolveMethod(handle.Value(), methodName)
	if err != nil {
		return 0, err
	}

	return callRuntimeValue(method, rawArgs, ellipsis)
}

// yak_runtime_drop_error implements yak's `f()~`. A trailing nil error is
// removed. A non-nil error is stored on the call context and recovered here
// so the caller can branch into try/catch instead of continuing. Any other
// tail is left alone and the first value is returned, matching the previous
// single-word unwrap.
//
//export yak_runtime_drop_error
func yak_runtime_drop_error(ctx unsafe.Pointer, value unsafe.Pointer) (ret int64) {
	if ctx != nil {
		// The invoke already copied any callee panic back to the caller.
		// This word is only the signal for the `~` on this call.
		ctxStoreWord(ctx, abi.WordPanic, 0)
		ctxClearFlags(ctx, abi.FlagPanicTaggedPointer)
	}
	defer func() {
		if r := recover(); r != nil {
			// A caught `~` is ordinary control flow. Logging it as a runtime
			// panic makes the mustpass harness reject a script that handled
			// the error. Uncaught panics still surface through the function
			// context and the main wrapper.
			if ctx != nil {
				panicValue, flags := recoveredPanicValue(r)
				ctxSetPanic(ctx, panicValue, flags)
			}
			ret = 0
		}
	}()
	if value == nil {
		return 0
	}
	if h, ok := handleFromShadow(value); ok {
		if elems, abiWords, ok := runtimeReturnTupleValue(h.Value()); ok && len(elems) > 0 {
			if len(elems) > 1 {
				if err, isErr := runtimeTupleTailIsError(elems[len(elems)-1], abiWords); isErr {
					if err != nil {
						panic(err)
					}
					return runtimeDropErrorValues(elems[:len(elems)-1])
				}
			}
			return runtimeDropErrorValues([]any{elems[0]})
		}
	}
	return int64(uintptr(value))
}

// runtimePrepareVariadicElement turns an ABI word stored in a yak slice into
// the variadic element type. `blacklist.Split(",")...` and option slices carry
// those words; setting the raw int64 into a func or string parameter panics.
func runtimePrepareVariadicElement(e reflect.Value, elemType reflect.Type) reflect.Value {
	if !e.IsValid() || elemType == nil {
		return e
	}
	if e.Type().AssignableTo(elemType) {
		return e
	}
	var word uint64
	isWord := false
	switch e.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		word = uint64(e.Int())
		isWord = true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		word = e.Uint()
		isWord = true
	}
	if isWord {
		if decoded, err := runtimeDecodeArg(word, elemType); err == nil && decoded.IsValid() && decoded.Type().AssignableTo(elemType) {
			return decoded
		}
	}
	if e.Type().ConvertibleTo(elemType) {
		return e.Convert(elemType)
	}
	return e
}

func runtimeDropErrorValues(vals []any) int64 {
	switch len(vals) {
	case 0:
		return 0
	case 1:
		return runtimeValueToInt64(reflect.ValueOf(vals[0]))
	default:
		return int64(uintptr(newRuntimeShadow(vals)))
	}
}

//export yak_runtime_string_slice
func yak_runtime_string_slice(parent unsafe.Pointer, low, high int64) unsafe.Pointer {
	defer recoverRuntimePanic()
	s := runtimePtrToString(parent)
	runes := []rune(s)
	if low < 0 {
		low = 0
	}
	if high < 0 || high > int64(len(runes)) {
		high = int64(len(runes))
	}
	if low > high {
		low = high
	}
	return newStdlibShadow(string(runes[low:high]))
}

// yak_runtime_sprintf implements yak's string `%` operator: a format string
// and either one value or a slice of values (`"n=%d" % 3`, `"%s %d" % [a, b]`).
//
//export yak_runtime_sprintf
func yak_runtime_sprintf(formatRaw, argRaw int64) int64 {
	defer recoverRuntimePanic()
	formatted := runtimePercentFormat(runtimePercentFormatString(uint64(formatRaw)), decodeTaggedArg(uint64(argRaw)))
	return int64(uintptr(newStdlibShadow(formatted)))
}

func runtimePercentFormatString(raw uint64) string {
	switch value := decodeTaggedArg(raw).(type) {
	case string:
		return value
	case []byte:
		return string(value)
	default:
		if value == nil {
			return ""
		}
		return fmt.Sprint(value)
	}
}

func runtimePercentFormat(format string, arg any) string {
	if arg == nil {
		return fmt.Sprintf(format, nil)
	}
	if b, ok := arg.([]byte); ok {
		return fmt.Sprintf(format, string(b))
	}
	if slice, ok := runtimeSliceValue(arg); ok {
		vals := make([]any, slice.Len())
		for i := 0; i < slice.Len(); i++ {
			vals[i] = runtimePercentArg(slice.Index(i).Interface())
		}
		return fmt.Sprintf(format, vals...)
	}
	return fmt.Sprintf(format, runtimePercentArg(arg))
}

func runtimePercentArg(v any) any {
	switch value := v.(type) {
	case string, []byte, float64, float32, bool:
		return value
	case int64:
		decoded := decodeTaggedArg(uint64(value))
		switch decoded.(type) {
		case string, []byte, float64:
			return decoded
		default:
			return value
		}
	default:
		return v
	}
}

//export yak_runtime_concat
func yak_runtime_concat(a, b unsafe.Pointer) unsafe.Pointer {
	defer recoverRuntimePanic()
	as := runtimePtrToString(a)
	bs := runtimePtrToString(b)
	return newStdlibShadow(as + bs)
}

func runtimePtrToString(ptr unsafe.Pointer) string {
	if ptr == nil {
		return ""
	}
	if s, ok := tryResolveShadowString(ptr); ok {
		return s
	}
	raw := uint64(uintptr(ptr))
	raw &^= yakTaggedPointerMask
	if s, ok := tryCString(raw); ok {
		return s
	}
	// Not a readable C string: yak template interpolation converts the value
	// to its string form (e.g. an int port becomes "41925"). An unmapped word
	// that merely looks like a pointer must take the same path instead of
	// faulting inside C.GoString.
	return fmt.Sprintf("%d", int64(raw))
}

func runtimeCallMappedElement(fn any, elem any) any {
	if fn == nil {
		return elem
	}
	fv := reflect.ValueOf(fn)
	if !fv.IsValid() || fv.Kind() != reflect.Func {
		return elem
	}
	out := fv.Call([]reflect.Value{reflect.ValueOf(elem)})
	if len(out) == 0 {
		return nil
	}
	return out[0].Interface()
}
