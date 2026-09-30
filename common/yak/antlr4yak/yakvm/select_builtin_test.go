package yakvm

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestSelectBuiltinUsesCallerFrame(t *testing.T) {
	f := NewFrame(New())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f.ctx = ctx
	ch := make(chan int, 1)
	ch <- 7
	args := NewValue("__opcode_list__", []*Value{selectTestOperand(reflect.SelectRecv, ch, nil)}, "")
	builtin := NewValue("", SelectBuiltin(), SelectBuiltinName)
	got := builtin.NativeCall(f, false, args)
	if !reflect.DeepEqual(got, []any{-1, nil, false}) || len(ch) != 1 {
		t.Fatalf("select ignored caller context: %v, buffered=%d", got, len(ch))
	}
	got = builtin.NativeCall(f, false, NewValue("", []any{}, ""))
	if !reflect.DeepEqual(got, []any{-1, nil, false}) {
		t.Fatalf("empty select ignored caller context: %v", got)
	}
}

func TestSelectBuiltinRejectsInvalidCalls(t *testing.T) {
	for _, tc := range []struct {
		name    string
		async   bool
		args    []*Value
		message string
	}{
		{"missing argument", false, nil, "invalid select call"},
		{"nil argument", false, []*Value{nil}, "invalid select call"},
		{"extra argument", false, []*Value{NewIntValue(1), NewIntValue(2)}, "invalid select call"},
		{"wrong list", false, []*Value{NewIntValue(1)}, "invalid select operands"},
		{"nonempty generic list", false, []*Value{NewValue("", []any{1}, "")}, "invalid select operands"},
		{"async", true, []*Value{NewValue("", []*Value{}, "")}, "invalid select call"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := NewFrame(New())
			f.ctx = context.Background()
			defer func() {
				if p := recover(); p == nil || !strings.Contains(fmt.Sprint(p), tc.message) {
					t.Fatalf("wanted %q, got %v", tc.message, p)
				}
			}()
			NewValue("", SelectBuiltin(), SelectBuiltinName).nativeCall(tc.async, false, f, tc.args...)
		})
	}
}

func TestSelectBuiltinDoesNotInterceptOrdinaryNativeFunctions(t *testing.T) {
	f := NewFrame(New())
	ordinary := func(frame *Frame, values []*Value) []any {
		if frame != f || len(values) != 1 || values[0].Int() != 7 {
			t.Fatal("ordinary FFI arguments changed")
		}
		return []any{42}
	}
	got := NewValue("", ordinary, "ordinary").NativeCall(f, false,
		NewValue("", f, ""), NewValue("", []*Value{NewIntValue(7)}, ""))
	if !reflect.DeepEqual(got, []any{42}) {
		t.Fatalf("ordinary function result changed: %v", got)
	}
}

func TestSelectBuiltinResolvesWithoutEngineRegistration(t *testing.T) {
	f := NewFrame(New())
	f.ctx = context.Background()
	f._execCode(&Code{Opcode: OpPushId, Op1: NewIdentifierValue(SelectBuiltinName)}, false)
	builtin := f.pop()
	args := NewValue("__opcode_list__", []*Value{selectTestOperand(reflect.SelectDefault, nil, nil)}, "")
	got := builtin.NativeCall(f, false, args)
	if !reflect.DeepEqual(got, []any{0, nil, false}) {
		t.Fatalf("unregistered select result: %v", got)
	}
	// Ordinary unresolved names still use the external-variable resolver.
	f.vm.GetExternalVar = func(name string) (any, bool) {
		if name != "legacyExternal" {
			t.Fatalf("unexpected external lookup: %s", name)
		}
		return 42, true
	}
	f._execCode(&Code{Opcode: OpPushId, Op1: NewIdentifierValue("legacyExternal")}, false)
	if got := f.pop().Int(); got != 42 {
		t.Fatalf("external resolution changed: %v", got)
	}
	f._execCode(&Code{Opcode: OpPushId, Op1: NewIdentifierValue(SelectBuiltinName)}, false)
	if _, ok := f.pop().Value.(selectBuiltinFunc); !ok {
		t.Fatal("internal builtin reached external resolver")
	}
}
