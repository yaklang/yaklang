package yakvm

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sync"
	"testing"
)

func incrementalAssignmentFrame(value *Value, variant int) *Frame {
	f := NewFrame(New())
	f.ctx = context.Background()
	f.codes = []*Code{{Opcode: OpList, Unary: 1}, {Opcode: OpPushLeftRef, Unary: 1}, {Opcode: OpList, Unary: 1}, {Opcode: OpAssign}}
	switch variant {
	case 1, 2, 3:
		f.codes = f.codes[:variant]
	case 4:
		f.codes[1].Unary = 0
	case 5:
		f.codes[1].Unary = -1
	case 6:
		f.codes[1] = &Code{Opcode: OpPush, Op1: NewIntValue(3)}
	case 7:
		f.codes[2].Unary = 2
	case 8:
		f.codes[2].Unary = 0
	case 9:
		f.codes[3].Opcode = OpPop
	case 10:
		f.codes[0].Unary = 2
	case 11:
		f.codes[1] = nil
	case 12:
		f.codes[2] = nil
	case 13:
		f.codes[3] = nil
	}
	f.push(value)
	return f
}

func incrementalRunAssignment(f *Frame, unfused bool) (p any) {
	return corePanic(func() {
		for f.codePointer < len(f.codes) {
			// The dispatcher's debug argument disables scalar fusion. Run the
			// original instructions without enabling debugger output/callbacks.
			f.execCode(f.codes[f.codePointer], unfused)
			f.nextCode()
		}
	})
}

func incrementalCheckAssignment(t *testing.T, value *Value, variant int) {
	t.Helper()
	fast, ordinary := incrementalAssignmentFrame(value, variant), incrementalAssignmentFrame(value, variant)
	var original Value
	if value != nil {
		original = *value
	}
	p := incrementalRunAssignment(fast, false)
	q := incrementalRunAssignment(ordinary, true)
	x, xok := fast.scope.GetValueByID(1)
	y, yok := ordinary.scope.GetValueByID(1)
	if fmt.Sprint(p) != fmt.Sprint(q) || fast.codePointer != ordinary.codePointer || xok != yok ||
		!reflect.DeepEqual(x, y) || !reflect.DeepEqual(fast.stack.values, ordinary.stack.values) {
		t.Fatalf("variant=%d RHS=%#v\nfast: panic=%v pc=%d binding=%#v stack=%v\nordinary: panic=%v pc=%d binding=%#v stack=%v", variant, value, p, fast.codePointer, x, fast.stack.values, q, ordinary.codePointer, y, ordinary.stack.values)
	}
	if value != nil && !reflect.DeepEqual(*value, original) {
		t.Fatal("assignment changed its RHS template")
	}
	if xok && (x.CallerRef == nil || x.CallerRef.SymbolId != 1 || x.CallerRef == value) {
		t.Fatalf("assignment lost fresh reference metadata: %#v", x)
	}
	if xok && (x == y || x.CallerRef == y.CallerRef) {
		t.Fatal("independent assignments share mutable binding metadata")
	}
}

func TestCoreIncrementalAssignmentDifferential(t *testing.T) {
	for _, raw := range []any{nil, true, -123, int64(math.MaxInt64), 1.25, "你好🙂", []byte{0, 255}, []any{1, nil, "x"}, map[string]int{"x": 7}, (map[string]int)(nil)} {
		for variant := 0; variant < 14; variant++ {
			incrementalCheckAssignment(t, NewAutoValue(raw), variant)
		}
	}
	for _, value := range []*Value{nil, undefined, NewValue("__channel__opcode_list__", []any{7, true}, ""), NewValue("__channel__opcode_list__", []any{}, "")} {
		for variant := 0; variant < 14; variant++ {
			incrementalCheckAssignment(t, value, variant)
		}
	}
}

func FuzzCoreIncrementalAssignment(f *testing.F) {
	for variant := byte(0); variant < 14; variant++ {
		f.Add([]byte{variant, 0, 7})
	}
	for kind := byte(1); kind < 7; kind++ {
		f.Add([]byte{0, kind, 255, 0, 128})
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 2 || len(data) > 512 {
			return
		}
		var value *Value
		switch data[1] % 7 {
		case 0:
			value = NewAutoValue(len(data))
		case 1:
			value = NewAutoValue(string(data[2:]))
		case 2:
			value = NewAutoValue(data[2:])
		case 3:
			value = undefined
		case 4:
			value = NewValue("__channel__opcode_list__", []any{len(data), true}, "")
		case 5:
			value = NewAutoValue(map[string]any{"payload": string(data[2:])})
		case 6:
			value = nil
		}
		incrementalCheckAssignment(t, value, int(data[0]%14))
	})
}

func TestCoreIncrementalAssignmentGuards(t *testing.T) {
	for _, mode := range []YVMMode{YAK, LUA, NASL} {
		for _, debug := range []bool{false, true} {
			f := incrementalAssignmentFrame(NewIntValue(7), 0)
			f.vm.config.SetYVMMode(mode)
			f._execCode(f.codes[0], debug)
			if got, want := f.codePointer == 3, mode == YAK && !debug; got != want {
				t.Fatalf("mode=%s debug=%v: fused=%v want=%v", mode, debug, got, want)
			}
		}
	}
	for _, value := range []*Value{nil, NewAutoValue(NewFunction(nil, NewSymbolTable()))} {
		f := incrementalAssignmentFrame(value, 0)
		if f.tryAssignSingle() {
			t.Fatal("nil/function RHS must use ordinary binding")
		}
	}
	f := incrementalAssignmentFrame(NewIntValue(7), 0)
	f.vm.debugMode, f.indebuggerEval = true, true
	f._execCode(f.codes[0], false)
	if f.codePointer != 0 {
		t.Fatal("VM debugging enabled scalar fusion")
	}
	f = incrementalAssignmentFrame(NewIntValue(7), 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f.ctx = ctx
	if !f.tryAssignSingle() || f.codePointer != len(f.codes) {
		t.Fatal("fusion missed cancellation at the assignment boundary")
	}
	if _, ok := f.scope.GetValueByID(1); ok {
		t.Fatal("cancelled assignment modified scope")
	}
}

func TestCoreIncrementalAssignmentAllocations(t *testing.T) {
	measure := func(unfused bool) float64 {
		f := incrementalAssignmentFrame(NewIntValue(7), 0)
		value := f.pop()
		return testing.AllocsPerRun(100, func() {
			f.codePointer = 0
			f.push(value)
			if p := incrementalRunAssignment(f, unfused); p != nil {
				t.Fatalf("assignment failed: %v", p)
			}
		})
	}
	fast, ordinary := measure(false), measure(true)
	if ordinary-fast < 4 {
		t.Fatalf("scalar wrapper savings regressed: fused=%g ordinary=%g allocs/op", fast, ordinary)
	}
	t.Logf("fused=%g ordinary=%g allocs/op", fast, ordinary)
}

func TestCoreIncrementalFrameIsolation(t *testing.T) {
	vm := New()
	a, b := NewFrame(vm), NewFrame(vm)
	// Operator implementations are installed by the embedding engine. Inject
	// a test operator so this remains meaningful in the standalone VM package.
	key := OpcodeFlag(-123)
	a.BinaryOperatorTable[key] = func(left, right *Value) *Value { return left }
	if b.BinaryOperatorTable[key] != nil || NewFrame(vm).BinaryOperatorTable[key] != nil {
		t.Fatal("root frames share a mutable binary operator table")
	}
	a.UnaryOperatorTable[key] = func(value *Value) *Value { return value }
	if b.UnaryOperatorTable[key] != nil || NewFrame(vm).UnaryOperatorTable[key] != nil {
		t.Fatal("root frames share a mutable unary operator table")
	}
	child := NewSubFrame(a)
	if child.BinaryOperatorTable[key] == nil || child.UnaryOperatorTable[key] == nil {
		t.Fatal("child lost its parent's operator overrides")
	}
	if a.GlobalVariables != vm.runtimeGlobalVar || b.GlobalVariables != vm.runtimeGlobalVar {
		t.Fatal("root frame lost the linked runtime globals")
	}
	for _, id := range []int{math.MinInt, -4096, -1, 0, 1, 99, 4096, math.MaxInt} {
		if got := NewValueRef(id).Literal; got != fmt.Sprintf("__symbol_%d__", id) {
			t.Fatalf("reference literal id=%d: %q", id, got)
		}
	}
}

func TestCoreIncrementalHandlerSnapshots(t *testing.T) {
	vm := New()
	register := func(n int) { vm.RegisterMapMemberCallHandler("a:b", "c", func(any) any { return n }) }
	register(1)
	vm.RegisterMapMemberCallHandler("a", "b:c", func(any) any { return 99 })
	old := NewFrame(vm)
	register(2)
	child, fresh := NewSubFrame(old), NewFrame(vm)
	for _, f := range []*Frame{old, child} {
		if f.execHijackMapMemberCallHandler("a:b", "c", nil) != 1 {
			t.Fatal("registration changed an existing root/child snapshot")
		}
	}
	if fresh.execHijackMapMemberCallHandler("a:b", "c", nil) != 2 || fresh.execHijackMapMemberCallHandler("a", "b:c", nil) != 99 {
		t.Fatal("fresh snapshot or tuple key isolation failed")
	}
	if got := fresh.execHijackMapMemberCallHandler("missing", "c", 7); got != 7 {
		t.Fatal("unregistered handler changed origin")
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for n := 0; n < 1000; n++ {
			register(n)
		}
	}()
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 100; n++ {
				f := NewFrame(vm)
				first := f.execHijackMapMemberCallHandler("a:b", "c", nil)
				if first != f.execHijackMapMemberCallHandler("a:b", "c", nil) || old.execHijackMapMemberCallHandler("a:b", "c", nil) != 1 {
					t.Error("snapshot changed during concurrent registration")
				}
			}
		}()
	}
	wg.Wait()
	if got := testing.AllocsPerRun(100, func() { vm.hijackMapMemberCallHandlers.loadSnapshot() }); got != 0 {
		t.Fatalf("warm snapshot lookup allocated: %g", got)
	}
}

var incrementalFrameSink *Frame

func TestCoreIncrementalFrameAllocationBound(t *testing.T) {
	vm := New()
	empty := testing.AllocsPerRun(100, func() { incrementalFrameSink = NewFrame(vm) })
	for n := 0; n < 1024; n++ {
		vm.RegisterMapMemberCallHandler("host", fmt.Sprint(n), func(origin any) any { return origin })
	}
	// AllocsPerRun warms the lazily published snapshot before measuring roots.
	populated := testing.AllocsPerRun(100, func() { incrementalFrameSink = NewFrame(vm) })
	if populated > empty+1 {
		t.Fatalf("warm roots copy the handler registry: empty=%g populated=%g allocs/op", empty, populated)
	}
}

func BenchmarkCoreIncrementalAssignment(b *testing.B) {
	for _, unfused := range []bool{false, true} {
		b.Run(fmt.Sprintf("unfused%v", unfused), func(b *testing.B) {
			f := incrementalAssignmentFrame(NewIntValue(7), 0)
			value := f.pop()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				f.codePointer = 0
				f.push(value)
				if p := incrementalRunAssignment(f, unfused); p != nil {
					b.Fatal(p)
				}
			}
		})
	}
}
