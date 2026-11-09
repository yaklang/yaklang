package lowir2ssa

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
)

func stackAllocationIR() *core.LowIR {
	return &core.LowIR{
		Target: testTarget(),
		Functions: []core.Function{{
			ID:      1,
			Address: 0x9000,
			Blocks: []core.Block{{
				ID:      1,
				Address: 0x9000,
				Instructions: []core.Instruction{
					{Kind: core.OpReadRegister, Results: []core.Value{{ID: 1, Type: integer(64)}}, Operands: []core.Operand{register("rsp", 64)}, Address: 0x9000},
					{Kind: core.OpConstant, Results: []core.Value{{ID: 2, Type: integer(64)}}, Operands: []core.Operand{literal(0x8)}, Address: 0x9000},
					{Kind: core.OpSub, Results: []core.Value{{ID: 3, Type: integer(64)}}, Operands: []core.Operand{value(1, integer(64)), value(2, integer(64))}, Address: 0x9000},
					{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rsp", 64), value(3, integer(64))}, Address: 0x9000},
					{Kind: core.OpConstant, Results: []core.Value{{ID: 4, Type: integer(64)}}, Operands: []core.Operand{literal(0x10)}, Address: 0x9001},
					{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rdi", 64), value(4, integer(64))}, Address: 0x9001},
					{Kind: core.OpReadRegister, Results: []core.Value{{ID: 5, Type: integer(64)}}, Operands: []core.Operand{register("rsp", 64)}, Address: 0x9002},
					{Kind: core.OpConstant, Results: []core.Value{{ID: 6, Type: integer(64)}}, Operands: []core.Operand{literal(0x8)}, Address: 0x9002},
					{Kind: core.OpAdd, Results: []core.Value{{ID: 7, Type: integer(64)}}, Operands: []core.Operand{value(5, integer(64)), value(6, integer(64))}, Address: 0x9002},
					{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rsp", 64), value(7, integer(64))}, Address: 0x9002},
					{Kind: core.OpReturn, Address: 0x9003},
				},
			}},
		}},
	}
}

func TestStackFrameRemovedByDefault(t *testing.T) {
	function := stackAllocationIR().Functions[0]
	if removed := RemoveStackFrame(&function); removed == 0 {
		t.Fatal("stack frame pass removed nothing")
	}
	result, err := Convert(context.Background(), stackAllocationIR())
	if err != nil {
		t.Fatal(err)
	}
	text := Text(result.Program)
	if strings.Contains(text, "rsp") {
		t.Fatalf("frame remains in SSA:\\n%s", text)
	}
	source, err := ssa2source.Render(context.Background(), result.Program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(source, "rsp") {
		t.Fatalf("frame remains in source:\\n%s", source)
	}
}

func TestStackFrameKeptWhenRequested(t *testing.T) {
	result, err := Convert(context.Background(), stackAllocationIR(), WithKeepStackFrame())
	if err != nil {
		t.Fatal(err)
	}
	source, err := ssa2source.Render(context.Background(), result.Program)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "entry_rsp") {
		t.Fatalf("frame base was removed despite the option:\n%s", source)
	}
	if strings.Contains(source, "undefined") {
		t.Fatalf("entry stack value leaked as undefined:\n%s", source)
	}
	if !strings.Contains(source, "entry_rsp") {
		t.Fatalf("entry stack value is not explicit:\n%s", source)
	}
}

func TestStackFrameWithFramePointer(t *testing.T) {
	ir := stackAllocationIR()
	ir.Functions[0].Blocks[0].Instructions = []core.Instruction{
		{Kind: core.OpReadRegister, Results: []core.Value{{ID: 1, Type: integer(64)}}, Operands: []core.Operand{register("rbp", 64)}, Address: 0x9100},
		{Kind: core.OpReadRegister, Results: []core.Value{{ID: 2, Type: integer(64)}}, Operands: []core.Operand{register("rsp", 64)}, Address: 0x9100},
		{Kind: core.OpConstant, Results: []core.Value{{ID: 3, Type: integer(64)}}, Operands: []core.Operand{literal(0x8)}, Address: 0x9100},
		{Kind: core.OpSub, Results: []core.Value{{ID: 4, Type: integer(64)}}, Operands: []core.Operand{value(2, integer(64)), value(3, integer(64))}, Address: 0x9100},
		{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rsp", 64), value(4, integer(64))}, Address: 0x9100},
		{Kind: core.OpCopy, Results: []core.Value{{ID: 5, Type: pointer()}}, Operands: []core.Operand{value(4, integer(64))}, Address: 0x9100},
		{Kind: core.OpStore, Operands: []core.Operand{value(5, pointer()), value(1, integer(64))}, Address: 0x9100},
		{Kind: core.OpReadRegister, Results: []core.Value{{ID: 6, Type: integer(64)}}, Operands: []core.Operand{register("rsp", 64)}, Address: 0x9101},
		{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rbp", 64), value(6, integer(64))}, Address: 0x9101},
		{Kind: core.OpReadRegister, Results: []core.Value{{ID: 7, Type: integer(64)}}, Operands: []core.Operand{register("rsp", 64)}, Address: 0x9102},
		{Kind: core.OpConstant, Results: []core.Value{{ID: 8, Type: integer(64)}}, Operands: []core.Operand{literal(0x20)}, Address: 0x9102},
		{Kind: core.OpSub, Results: []core.Value{{ID: 9, Type: integer(64)}}, Operands: []core.Operand{value(7, integer(64)), value(8, integer(64))}, Address: 0x9102},
		{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rsp", 64), value(9, integer(64))}, Address: 0x9102},
		{Kind: core.OpConstant, Results: []core.Value{{ID: 10, Type: integer(32)}}, Operands: []core.Operand{literal(0)}, Address: 0x9103},
		{Kind: core.OpReturn, Operands: []core.Operand{value(10, integer(32))}, Address: 0x9104},
		{Kind: core.OpReadRegister, Results: []core.Value{{ID: 11, Type: integer(64)}}, Operands: []core.Operand{register("rbp", 64)}, Address: 0x9105},
		{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rsp", 64), value(11, integer(64))}, Address: 0x9105},
		{Kind: core.OpReadRegister, Results: []core.Value{{ID: 12, Type: integer(64)}}, Operands: []core.Operand{register("rsp", 64)}, Address: 0x9105},
		{Kind: core.OpCopy, Results: []core.Value{{ID: 13, Type: pointer()}}, Operands: []core.Operand{value(12, integer(64))}, Address: 0x9105},
		{Kind: core.OpLoad, Results: []core.Value{{ID: 14, Type: integer(64)}}, Operands: []core.Operand{value(13, pointer())}, Address: 0x9105},
		{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rbp", 64), value(14, integer(64))}, Address: 0x9105},
		{Kind: core.OpConstant, Results: []core.Value{{ID: 15, Type: integer(64)}}, Operands: []core.Operand{literal(0x8)}, Address: 0x9105},
		{Kind: core.OpAdd, Results: []core.Value{{ID: 16, Type: integer(64)}}, Operands: []core.Operand{value(12, integer(64)), value(15, integer(64))}, Address: 0x9105},
		{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rsp", 64), value(16, integer(64))}, Address: 0x9105},
	}
	function := ir.Functions[0]
	if removed := RemoveStackFrame(&function); removed < 6 {
		t.Fatalf("removed %d frame instructions", removed)
	}
	result, err := Convert(context.Background(), ir)
	if err != nil {
		t.Fatal(err)
	}
	text := Text(result.Program)
	for _, forbidden := range []string{"rsp", "rbp"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("frame register %s remains:\\n%s", forbidden, text)
		}
	}
}
