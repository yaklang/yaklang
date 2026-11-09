package ssapass

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func newProgram(t *testing.T, name string) (*ssa.Program, *ssa.Function, *ssa.FunctionBuilder) {
	t.Helper()
	program := ssa.NewProgram(nil, ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	function := program.NewFunction(name)
	return program, function, function.GetOrCreateBuilder()
}

func extern(program *ssa.Program, name string) *ssa.Function {
	function := program.NewFunction(name)
	function.SetExtern(true)
	return function
}

func ptrType() ssa.Type {
	return ssa.NewBasicType(ssa.NumberTypeKind, "ptr64")
}

func TestApplyFunctionDefine(t *testing.T) {
	program, function, builder := newProgram(t, "caller")
	target := extern(program, "puts")
	address := builder.EmitConstInst(int64(0x402004))
	address.SetType(ptrType())
	call := builder.EmitCall(builder.NewCall(target, []ssa.Value{address}))
	builder.EmitReturn(nil)
	function.Finish()

	if matched := ApplyFunctionDefine(program); matched != 1 {
		t.Fatalf("matched = %d", matched)
	}
	if call.GetType().String() != "int32" {
		t.Fatalf("call type = %s", call.GetType())
	}
}

func TestRedefineStringArgument(t *testing.T) {
	program, function, builder := newProgram(t, "caller")
	target := extern(program, "puts")
	address := builder.EmitConstInst(int64(0x402004))
	address.SetType(ptrType())
	cast := builder.EmitTypeCast(address, ptrType())
	call := builder.EmitCall(builder.NewCall(target, []ssa.Value{cast}))
	builder.EmitReturn(nil)
	function.Finish()

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
	program, function, builder := newProgram(t, "caller")
	target := extern(program, "printf")
	format := builder.EmitConstInst(int64(0x402004))
	format.SetType(ptrType())
	value := builder.EmitConstInst(int64(42))
	call := builder.EmitCall(builder.NewCall(target, []ssa.Value{format, value}))
	builder.EmitReturn(nil)
	function.Finish()

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
