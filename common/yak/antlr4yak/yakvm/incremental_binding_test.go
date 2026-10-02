package yakvm

import (
	"fmt"
	"reflect"
	"runtime"
	"testing"
)

// Keep the pre-optimization binder (5cb87aca0a) as a test-only compatibility
// oracle. In particular, named arguments count toward arity before filtering,
// duplicate symbols overwrite, and nil positional slots consume an argument.
// Do not update this oracle when changing the optimized implementation.
func incrementalLegacyBinding(f *Function, vs []*Value, check bool) map[int]*Value {
	name := f.GetActualName()
	fixed := len(f.paramSymbols)
	variadicID := 0
	if f.IsVariableParameter() {
		if fixed == 0 {
			panic(fmt.Sprintf("function %s has no variadic parameter", name))
		}
		fixed--
		variadicID = f.paramSymbols[fixed]
	} else if check && fixed != len(vs) {
		panic(fmt.Sprintf("function %v params number not match, expect %v, got %v", name, fixed, len(vs)))
	}
	params := make(map[int]*Value)
	if check && fixed > len(vs) {
		panic(fmt.Sprintf("runtime error: function %s need at least %d params, got %d params", name, fixed, len(vs)))
	}
	positional := make([]*Value, 0)
	for _, value := range vs {
		if value != nil && value.SymbolId != 0 {
			params[value.SymbolId] = value
		} else {
			positional = append(positional, value)
		}
	}
	i := 0
	for parameter, symbol := range f.paramSymbols[:fixed] {
		if _, named := params[symbol]; named {
			continue
		}
		value := undefined
		if i < len(positional) {
			value = positional[i]
			i++
		} else if check {
			panic(fmt.Sprintf("runtime error: function %s missing parameter %d", name, parameter+1))
		}
		params[symbol] = value
	}
	if f.IsVariableParameter() {
		tail := make([]interface{}, len(positional)-i)
		for j, value := range positional[i:] {
			if value != nil {
				tail[j] = value.Value
			}
		}
		params[variadicID] = NewValue("[]any", tail, "")
	}
	return params
}

func incrementalCheckBinding(t *testing.T, symbols []int, variadic, check bool, args []*Value) {
	t.Helper()
	f := NewFunction(nil, NewSymbolTable())
	f.SetName("compatibility")
	f.SetParamSymbols(symbols)
	f.SetIsVariableParameter(variadic)
	input := append([]*Value(nil), args...)
	values := make([]Value, len(args))
	for i, value := range args {
		if value != nil {
			values[i] = *value
		}
	}
	var want, got map[int]*Value
	wantPanic := corePanic(func() { want = incrementalLegacyBinding(f, args, check) })
	gotPanic := corePanic(func() { got = YakVMValuesToFunctionMap(f, args, check) })
	if fmt.Sprint(gotPanic) != fmt.Sprint(wantPanic) || !reflect.DeepEqual(got, want) {
		t.Fatalf("symbols=%v variadic=%v check=%v args=%v\ngot=%v panic=%v\nwant=%v panic=%v", symbols, variadic, check, args, got, gotPanic, want, wantPanic)
	}
	for i, value := range args {
		if value != input[i] || (value != nil && !reflect.DeepEqual(*value, values[i])) {
			t.Fatalf("caller argument %d was modified", i)
		}
	}
}

func incrementalBindingArg(kind, i int) *Value {
	if kind == 0 {
		return nil
	}
	v := NewIntValue(100 + i)
	switch kind {
	case 2:
		v.SymbolId = 1
	case 3:
		v.SymbolId = 99
	case 4:
		v.SymbolId = -1
	}
	return v
}

func TestCoreIncrementalBindingDifferential(t *testing.T) {
	for n := 0; n <= 5; n++ {
		symbols := make([]int, n)
		for i := range symbols {
			symbols[i] = i + 1
		}
		for count, limit := 0, 1; count <= 4; count, limit = count+1, limit*5 {
			for pattern := 0; pattern < limit; pattern++ {
				args := make([]*Value, count)
				for i, p := 0, pattern; i < count; i, p = i+1, p/5 {
					args[i] = incrementalBindingArg(p%5, i)
				}
				for _, variadic := range []bool{false, true} {
					for _, check := range []bool{false, true} {
						incrementalCheckBinding(t, symbols, variadic, check, args)
					}
				}
			}
		}
	}
	// Public Function metadata can contain duplicate, zero, or negative IDs.
	for _, symbols := range [][]int{{1, 1}, {0, -1, 1}, {99, 1, 99}} {
		for _, variadic := range []bool{false, true} {
			for _, check := range []bool{false, true} {
				incrementalCheckBinding(t, symbols, variadic, check, []*Value{incrementalBindingArg(2, 0), nil, incrementalBindingArg(1, 2)})
			}
		}
	}
}

func FuzzCoreIncrementalBinding(f *testing.F) {
	for _, seed := range [][]byte{{}, {0, 2, 0, 1, 2}, {3, 3, 1, 1, 1, 2, 0, 3, 4}, {1, 1, 255, 0, 0, 2, 2}, {2, 0, 0, 1, 1, 1}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		// Both metadata and argument counts are bounded; fuzz never executes code.
		if len(data) < 2 || len(data) > 256 {
			return
		}
		n := int(data[1] % 33)
		if len(data) < 2+n {
			return
		}
		symbols := make([]int, n)
		for i := range symbols {
			symbols[i] = int(int8(data[2+i]))
		}
		args := make([]*Value, len(data)-2-n)
		for i, kind := range data[2+n:] {
			args[i] = incrementalBindingArg(int(kind%5), i)
		}
		incrementalCheckBinding(t, symbols, data[0]&1 != 0, data[0]&2 != 0, args)
	})
}

var incrementalBindingSink map[int]*Value

func TestCoreIncrementalBindingAllocationBound(t *testing.T) {
	t.Run("positionalFiltering", func(t *testing.T) {
		f := NewFunction(nil, NewSymbolTable())
		f.SetParamSymbols([]int{1, 2})
		args := []*Value{NewIntValue(1), NewIntValue(2)}
		fast := testing.AllocsPerRun(100, func() { incrementalBindingSink = YakVMValuesToFunctionMap(f, args, true) })
		old := testing.AllocsPerRun(100, func() { incrementalBindingSink = incrementalLegacyBinding(f, args, true) })
		if old-fast < 1 {
			t.Fatalf("positional filtering savings regressed: optimized=%g legacy=%g allocs/op", fast, old)
		}
	})
	// These have constant-size output despite very large inputs. Capacity hints
	// based on argument/signature length previously wasted 0.5–2.5 MiB per call.
	for _, name := range []string{"allNamed", "duplicateSymbols", "missingVariadic"} {
		t.Run(name, func(t *testing.T) {
			f := NewFunction(nil, NewSymbolTable())
			var args []*Value
			symbols := make([]int, 65536)
			for i := range symbols {
				symbols[i] = 1
			}
			check := false
			switch name {
			case "allNamed":
				f.SetParamSymbols([]int{1})
				v := NewIntValue(7)
				v.SymbolId = 1
				args = make([]*Value, len(symbols))
				for i := range args {
					args[i] = v
				}
			case "duplicateSymbols":
				f.SetParamSymbols(symbols)
				args = []*Value{NewIntValue(7)}
			case "missingVariadic":
				f.SetParamSymbols(symbols)
				f.SetIsVariableParameter(true)
				check = true
			}
			run := func() { corePanic(func() { incrementalBindingSink = YakVMValuesToFunctionMap(f, args, check) }) }
			run() // Warm panic formatting and runtime state before sampling.
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			const repeats = 100
			for i := 0; i < repeats; i++ {
				run()
			}
			runtime.ReadMemStats(&after)
			bytes := (after.TotalAlloc - before.TotalAlloc) / repeats
			// Deliberate slack for platform/runtime bookkeeping, never a time gate.
			if bytes > 8192 {
				t.Fatalf("constant-size binding allocated %d B/op; input-sized reservation returned", bytes)
			}
			t.Logf("65536-entry input: %d B/op", bytes)
		})
	}
}

func BenchmarkCoreIncrementalBinding(b *testing.B) {
	for _, named := range []bool{false, true} {
		for _, legacy := range []bool{false, true} {
			b.Run(fmt.Sprintf("named%v/legacy%v", named, legacy), func(b *testing.B) {
				f := NewFunction(nil, NewSymbolTable())
				f.SetParamSymbols([]int{1, 2})
				args := []*Value{NewIntValue(1), NewIntValue(2)}
				if named {
					args[0].SymbolId = 1
				}
				bind := YakVMValuesToFunctionMap
				if legacy {
					bind = incrementalLegacyBinding
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					incrementalBindingSink = bind(f, args, true)
				}
			})
		}
	}
}
