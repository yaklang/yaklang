package pass_test

import (
	"context"
	. "github.com/yaklang/yaklang/common/yak/reify/core/ssa2source/pass"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestApplyFunctionDefine(t *testing.T) {
	program := parseSSA(t, `extern puts
caller
type: () -> void
entry_0:
    <ptr64> t1 = <ptr64> 0x402004
    <any> t2 = call <any> puts (<ptr64> t1) binding[] member[]
    ret`)
	call := findCall(t, program, "puts")
	if matched := ApplyFunctionDefine(program); matched != 1 {
		t.Fatalf("matched = %d", matched)
	}
	if call.GetType().String() != "int32" {
		t.Fatalf("call type = %s", call.GetType())
	}
}

func TestRedefineStringArgument(t *testing.T) {
	program := parseSSA(t, `extern puts
caller
type: () -> void
entry_0:
    <ptr64> t1 = <ptr64> 0x402004
    <ptr64> t2 = type_cast[ptr64] <ptr64> t1
    <any> t3 = call <any> puts (<ptr64> t2) binding[] member[]
    ret`)
	call := findCall(t, program, "puts")
	ApplyFunctionDefine(program)
	replaced := RedefineTypeValue(program, WithStringAt(func(addr uint64) (string, bool) {
		if addr == 0x402004 {
			return "hello, reify", true
		}
		return "", false
	}))
	if replaced != 1 {
		t.Fatalf("replaced = %d", replaced)
	}
	value, ok := call.GetValueById(call.Args[0])
	if !ok || value == nil {
		t.Fatal("argument is missing")
	}
	constant, ok := ssa.ToConstInst(value)
	if !ok || constant == nil || constant.GetRawValue() != "hello, reify" {
		t.Fatalf("argument = %v", value)
	}
	text, err := ssa2source.Render(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, `puts("hello, reify")`) {
		t.Fatalf("source lacks string argument:\n%s", text)
	}
}

func TestRedefineVariadicStringArgument(t *testing.T) {
	program := parseSSA(t, `extern printf
caller
type: () -> void
entry_0:
    <ptr64> t1 = <ptr64> 0x402004
    <i64> t2 = <i64> 42
    <any> t3 = call <any> printf (<ptr64> t1, <i64> t2) binding[] member[]
    ret`)
	call := findCall(t, program, "printf")
	ApplyFunctionDefine(program)
	replaced := RedefineTypeValue(program, WithStringAt(func(addr uint64) (string, bool) {
		if addr == 0x402004 {
			return "%d", true
		}
		return "", false
	}))
	if replaced != 1 {
		t.Fatalf("replaced = %d", replaced)
	}
	first, _ := call.GetValueById(call.Args[0])
	firstConst, ok := ssa.ToConstInst(first)
	if !ok || firstConst.GetRawValue() != "%d" {
		t.Fatalf("format argument = %v", first)
	}
	second, _ := call.GetValueById(call.Args[1])
	secondConst, ok := ssa.ToConstInst(second)
	if !ok || secondConst.GetRawValue() != int64(42) {
		t.Fatalf("variadic value changed: %v", second)
	}
}
