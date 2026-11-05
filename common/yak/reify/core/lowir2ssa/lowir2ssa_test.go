package lowir2ssa

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
)

func testTarget() core.Target {
	return core.Target{OS: core.OSLinux, Architecture: core.ArchAMD64, Bits: 64, ByteOrder: core.LittleEndian}
}

func integer(bits uint16) core.ValueType {
	return core.ValueType{Kind: core.TypeInteger, Bits: bits}
}

func pointer() core.ValueType {
	return core.ValueType{Kind: core.TypePointer, Bits: 64}
}

func register(name string, bits uint16) core.Operand {
	return core.Operand{Kind: core.OperandRegister, Type: integer(bits), Name: name}
}

func value(id core.ValueID, typ core.ValueType) core.Operand {
	return core.Operand{Kind: core.OperandValue, Type: typ, Value: id}
}

func literal(data ...byte) core.Operand {
	return core.Operand{Kind: core.OperandConstant, Literal: data}
}

func testIR() *core.LowIR {
	return &core.LowIR{
		Target: testTarget(),
		Functions: []core.Function{{
			ID:      1,
			Address: 0x1000,
			Blocks: []core.Block{
				{
					ID:      1,
					Address: 0x1000,
					Instructions: []core.Instruction{
						{Kind: core.OpConstant, Results: []core.Value{{ID: 1, Type: integer(64)}}, Operands: []core.Operand{literal(0x2a)}, Address: 0x1000},
						{Kind: core.OpConstant, Results: []core.Value{{ID: 2, Type: integer(64)}}, Operands: []core.Operand{literal(0x03)}, Address: 0x1001},
						{Kind: core.OpAdd, Results: []core.Value{{ID: 3, Type: integer(64)}}, Operands: []core.Operand{value(1, integer(64)), value(2, integer(64))}, Address: 0x1002},
						{Kind: core.OpBranch, Operands: []core.Operand{value(3, integer(64))}, Targets: []core.BlockID{2, 3}, Address: 0x1003},
					},
				},
				{ID: 2, Address: 0x1004, Instructions: []core.Instruction{{Kind: core.OpReturn, Operands: []core.Operand{value(3, integer(64))}, Address: 0x1004}}},
				{ID: 3, Address: 0x1005, Instructions: []core.Instruction{{Kind: core.OpReturn, Operands: []core.Operand{literal(0)}, Address: 0x1005}}},
			},
		}},
	}
}

func TestConvertBuildsFunctionsAndBranches(t *testing.T) {
	result, err := Convert(context.Background(), testIR())
	if err != nil {
		t.Fatal(err)
	}
	if result.Program == nil || result.Program.Funcs.Len() != 1 {
		t.Fatalf("bad program: %+v", result)
	}
	text := Text(result.Program)
	for _, fragment := range []string{"sub_0x1000", "add", "If", "ret"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("SSA text lacks %q:\n%s", fragment, text)
		}
	}
}

func diamondIR() *core.LowIR {
	write := func(id core.ValueID, valueByte byte, address uint64) []core.Instruction {
		return []core.Instruction{
			{Kind: core.OpConstant, Results: []core.Value{{ID: id, Type: integer(64)}}, Operands: []core.Operand{literal(valueByte)}, Address: address},
			{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rax", 64), value(id, integer(64))}, Address: address + 1},
		}
	}
	return &core.LowIR{
		Target: testTarget(),
		Functions: []core.Function{{
			ID:      1,
			Address: 0x2000,
			Blocks: []core.Block{
				{
					ID:      1,
					Address: 0x2000,
					Instructions: []core.Instruction{
						{Kind: core.OpConstant, Results: []core.Value{{ID: 1, Type: integer(32)}}, Operands: []core.Operand{literal(1)}, Address: 0x2000},
						{Kind: core.OpBranch, Operands: []core.Operand{value(1, integer(32))}, Targets: []core.BlockID{2, 3}, Address: 0x2001},
					},
				},
				{ID: 2, Address: 0x2002, Instructions: append(write(2, 0x22, 0x2002), core.Instruction{Kind: core.OpJump, Targets: []core.BlockID{4}, Address: 0x2004})},
				{ID: 3, Address: 0x2005, Instructions: append(write(3, 0x33, 0x2005), core.Instruction{Kind: core.OpJump, Targets: []core.BlockID{4}, Address: 0x2007})},
				{
					ID:      4,
					Address: 0x2008,
					Instructions: []core.Instruction{
						{Kind: core.OpReadRegister, Results: []core.Value{{ID: 4, Type: integer(64)}}, Operands: []core.Operand{register("rax", 64)}, Address: 0x2008},
						{Kind: core.OpReturn, Operands: []core.Operand{value(4, integer(64))}, Address: 0x2009},
					},
				},
			},
		}},
	}
}

func TestRegistersBecomeVariablesWithoutPhi(t *testing.T) {
	result, err := Convert(context.Background(), diamondIR())
	if err != nil {
		t.Fatal(err)
	}
	text := Text(result.Program)
	for _, forbidden := range []string{"read-register", "write-register", "phi"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("SSA text still contains %q:\n%s", forbidden, text)
		}
	}
	source, err := ssa2source.Render(context.Background(), result.Program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(source, "read-register") || strings.Contains(source, "phi") {
		t.Fatalf("source still uses register calls or phis:\n%s", source)
	}
	if strings.Count(source, "function ") != 1 {
		t.Fatalf("pseudo functions leaked into the source:\n%s", source)
	}
}

func TestValuesCarryBitWidthTypes(t *testing.T) {
	ir := &core.LowIR{
		Target: testTarget(),
		Functions: []core.Function{{
			ID:      1,
			Address: 0x2500,
			Blocks: []core.Block{{
				ID:      1,
				Address: 0x2500,
				Instructions: []core.Instruction{
					{Kind: core.OpConstant, Results: []core.Value{{ID: 1, Type: integer(8)}}, Operands: []core.Operand{literal(0x7f)}, Address: 0x2500},
					{Kind: core.OpConstant, Results: []core.Value{{ID: 2, Type: integer(16)}}, Operands: []core.Operand{literal(0xff, 0x7f)}, Address: 0x2501},
					{Kind: core.OpConstant, Results: []core.Value{{ID: 3, Type: integer(32)}}, Operands: []core.Operand{literal(0xff, 0xff, 0xff, 0x7f)}, Address: 0x2502},
					{Kind: core.OpConstant, Results: []core.Value{{ID: 4, Type: pointer()}}, Operands: []core.Operand{literal(0x10)}, Address: 0x2503},
					{Kind: core.OpWriteRegister, Operands: []core.Operand{register("al", 8), value(1, integer(8))}, Address: 0x2504},
					{Kind: core.OpWriteRegister, Operands: []core.Operand{register("ax", 16), value(2, integer(16))}, Address: 0x2505},
					{Kind: core.OpWriteRegister, Operands: []core.Operand{register("eax", 32), value(3, integer(32))}, Address: 0x2506},
					{Kind: core.OpReadRegister, Results: []core.Value{{ID: 5, Type: integer(8)}}, Operands: []core.Operand{register("al", 8)}, Address: 0x2507},
					{Kind: core.OpReadRegister, Results: []core.Value{{ID: 6, Type: integer(16)}}, Operands: []core.Operand{register("ax", 16)}, Address: 0x2508},
					{Kind: core.OpReadRegister, Results: []core.Value{{ID: 7, Type: integer(32)}}, Operands: []core.Operand{register("eax", 32)}, Address: 0x2509},
					{Kind: core.OpLoad, Results: []core.Value{{ID: 8, Type: integer(64)}}, Operands: []core.Operand{value(4, pointer())}, Address: 0x250a},
					{Kind: core.OpReturn, Address: 0x250b},
				},
			}},
		}},
	}
	result, err := Convert(context.Background(), ir)
	if err != nil {
		t.Fatal(err)
	}
	text := Text(result.Program)
	for _, fragment := range []string{"<i8>", "<i16>", "<i32>", "<i64>", "<ptr64>"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("typed SSA lacks %s:\n%s", fragment, text)
		}
	}
}

func TestPartialRegisterWritePreservesUpperBits(t *testing.T) {
	ir := &core.LowIR{
		Target: testTarget(),
		Functions: []core.Function{{
			ID:      1,
			Address: 0x3000,
			Blocks: []core.Block{{
				ID:      1,
				Address: 0x3000,
				Instructions: []core.Instruction{
					{Kind: core.OpConstant, Results: []core.Value{{ID: 1, Type: integer(64)}}, Operands: []core.Operand{literal(0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11)}, Address: 0x3000},
					{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rax", 64), value(1, integer(64))}, Address: 0x3001},
					{Kind: core.OpConstant, Results: []core.Value{{ID: 2, Type: integer(8)}}, Operands: []core.Operand{literal(0xaa)}, Address: 0x3002},
					{Kind: core.OpWriteRegister, Operands: []core.Operand{register("ah", 8), value(2, integer(8))}, Address: 0x3003},
					{Kind: core.OpReadRegister, Results: []core.Value{{ID: 3, Type: integer(64)}}, Operands: []core.Operand{register("rax", 64)}, Address: 0x3004},
					{Kind: core.OpReturn, Operands: []core.Operand{value(3, integer(64))}, Address: 0x3005},
				},
			}},
		}},
	}
	result, err := Convert(context.Background(), ir)
	if err != nil {
		t.Fatal(err)
	}
	text := Text(result.Program)
	for _, fragment := range []string{"and-not", " shl ", " or "} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("partial write lacks %q:\n%s", fragment, text)
		}
	}
	if strings.Contains(text, "read-register") {
		t.Fatalf("partial write kept a register call:\n%s", text)
	}
}

func TestMemoryAndCallsStayExplicit(t *testing.T) {
	ir := testIR()
	ir.Functions[0].Blocks = []core.Block{{
		ID:      1,
		Address: 0x4000,
		Instructions: []core.Instruction{
			{Kind: core.OpConstant, Results: []core.Value{{ID: 1, Type: pointer()}}, Operands: []core.Operand{literal(0x10)}, Address: 0x4000},
			{Kind: core.OpLoad, Results: []core.Value{{ID: 2, Type: integer(64)}}, Operands: []core.Operand{value(1, pointer())}, Address: 0x4001},
			{Kind: core.OpStore, Operands: []core.Operand{value(1, pointer()), value(2, integer(64))}, Address: 0x4002},
			{Kind: core.OpCall, Results: []core.Value{{ID: 3, Type: integer(64)}}, Operands: []core.Operand{{Kind: core.OperandSymbol, Name: "helper"}}, Address: 0x4003},
			{Kind: core.OpReturn, Operands: []core.Operand{value(3, integer(64))}, Address: 0x4004},
		},
	}}
	result, err := Convert(context.Background(), ir)
	if err != nil {
		t.Fatal(err)
	}
	text := Text(result.Program)
	for _, fragment := range []string{"reify.load", "reify.store", "reify.call_helper"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("SSA text lacks %q:\n%s", fragment, text)
		}
	}
}

func TestIntrinsicsUseReservedPrefix(t *testing.T) {
	ir := testIR()
	ir.Functions[0].Blocks = []core.Block{{
		ID:      1,
		Address: 0x5000,
		Instructions: []core.Instruction{
			{Kind: core.OpIntrinsic, Results: []core.Value{{ID: 1, Type: integer(64)}}, Operands: []core.Operand{{Kind: core.OperandSymbol, Name: "x86.flags.add"}}, Address: 0x5000},
			{Kind: core.OpReturn, Address: 0x5001},
		},
	}}
	result, err := Convert(context.Background(), ir)
	if err != nil {
		t.Fatal(err)
	}
	text := Text(result.Program)
	if strings.Count(text, "entry-0:") != 1 {
		t.Fatalf("pseudo functions leaked into the SSA text:\n%s", text)
	}
	if !strings.Contains(text, "reify.intrinsic.x86.flags.add") {
		t.Fatalf("intrinsic target was renamed unexpectedly:\n%s", text)
	}
}

func TestConvertRejectsEmptyAndCancelled(t *testing.T) {
	if _, err := Convert(context.Background(), nil); err == nil {
		t.Fatal("nil LowIR accepted")
	}
	if _, err := Convert(context.Background(), &core.LowIR{}); err == nil {
		t.Fatal("empty LowIR accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Convert(ctx, testIR()); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestRegisterViewNames(t *testing.T) {
	for _, tc := range []struct {
		name      string
		bits      uint16
		canonical string
		width     uint16
		shift     uint16
	}{
		{"al", 8, "rax", 8, 0},
		{"ah", 8, "rax", 8, 8},
		{"eax", 32, "rax", 32, 0},
		{"rax", 64, "rax", 64, 0},
		{"r8b", 8, "r8", 8, 0},
		{"r8w", 16, "r8", 16, 0},
		{"r8d", 32, "r8", 32, 0},
		{"r8l", 32, "r8", 32, 0},
		{"r8", 64, "r8", 64, 0},
		{"xmm3", 128, "xmm3", 128, 0},
		{"rflags", 64, "rflags", 64, 0},
	} {
		view := registerViewFor(tc.name, tc.bits)
		if view.canonical != tc.canonical || view.width != tc.width || view.shift != tc.shift {
			t.Fatalf("%s: %+v", tc.name, view)
		}
	}
}

func TestBuilderSatisfiesContract(t *testing.T) {
	var _ core.SSAIRBuilder = Builder{}
}
