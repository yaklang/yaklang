package lowir2ssa

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
	"github.com/yaklang/yaklang/common/yak/ssa"
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
	for _, forbidden := range []string{"read_register", "write_register", "phi"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("SSA text still contains %q:\n%s", forbidden, text)
		}
	}
	source, err := ssa2source.Render(context.Background(), result.Program)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(source, "read_register") || strings.Contains(source, "phi") {
		t.Fatalf("source still uses register calls or phis:\n%s", source)
	}
	if strings.Count(source, "(void) {") != 1 {
		t.Fatalf("pseudo functions leaked into the source:\n%s", source)
	}
}

func TestValuesCarryBitWidthTypes(t *testing.T) {
	converter := &converter{types: make(map[core.ValueType]ssa.Type)}
	for bits, expected := range map[uint16]string{8: "i8", 16: "i16", 32: "i32", 64: "i64"} {
		if got := converter.typeFor(integer(bits)).String(); got != expected {
			t.Fatalf("i%d type = %s", bits, got)
		}
	}
	if got := converter.typeFor(pointer()).String(); got != "ptr64" {
		t.Fatalf("pointer type = %s", got)
	}
	if got := converter.typeFor(core.ValueType{Kind: core.TypeFloat, Bits: 64}).String(); got != "f64" {
		t.Fatalf("float type = %s", got)
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
	if strings.Contains(text, "read_register") {
		t.Fatalf("partial write kept a register call:\n%s", text)
	}
}

func countCalls(program *ssa.Program) int {
	total := 0
	program.Funcs.ForEach(func(_ string, function *ssa.Function) bool {
		if function == nil || function.IsExtern() {
			return true
		}
		for _, blockID := range function.Blocks {
			block, ok := function.GetBasicBlockByID(blockID)
			if !ok || block == nil {
				continue
			}
			for _, instructionID := range block.Insts {
				instruction, ok := function.GetInstructionById(instructionID)
				if !ok || instruction == nil {
					continue
				}
				if _, ok := ssa.ToCall(instruction); ok {
					total++
				}
			}
		}
		return true
	})
	return total
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
	for _, fragment := range []string{"load(", "helper"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("SSA text lacks %q:\n%s", fragment, text)
		}
	}
	if strings.Contains(text, "reify.") {
		t.Fatalf("SSA text still uses reify pseudo-functions:\n%s", text)
	}
	if calls := countCalls(result.Program); calls != 1 {
		t.Fatalf("calls = %d, want only the real call", calls)
	}
}

func TestIntrinsicBecomesOpaqueMarker(t *testing.T) {
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
	if strings.Count(text, "entry_0:") != 1 {
		t.Fatalf("extern call targets leaked into the SSA text:\n%s", text)
	}
	if strings.Contains(text, "type-case") {
		t.Fatalf("type cast still uses a hyphen:\n%s", text)
	}
	if !strings.Contains(text, "x86.flags.add") {
		t.Fatalf("intrinsic marker is missing:\n%s", text)
	}
	if strings.Contains(text, "reify.") {
		t.Fatalf("intrinsic was preserved as a reify call:\n%s", text)
	}
	if calls := countCalls(result.Program); calls != 0 {
		t.Fatalf("intrinsic produced %d calls", calls)
	}
}

func TestOnlyCallProducesCall(t *testing.T) {
	ir := &core.LowIR{
		Target: testTarget(),
		Functions: []core.Function{{
			ID:      1,
			Address: 0x5500,
			Blocks: []core.Block{{
				ID:      1,
				Address: 0x5500,
				Instructions: []core.Instruction{
					{Kind: core.OpConstant, Results: []core.Value{{ID: 1, Type: pointer()}}, Operands: []core.Operand{literal(0x10)}, Address: 0x5500},
					{Kind: core.OpLoad, Results: []core.Value{{ID: 2, Type: integer(64)}}, Operands: []core.Operand{value(1, pointer())}, Address: 0x5501},
					{Kind: core.OpStore, Operands: []core.Operand{value(1, pointer()), value(2, integer(64))}, Address: 0x5502},
					{Kind: core.OpIntrinsic, Results: []core.Value{{ID: 3, Type: integer(64)}}, Operands: []core.Operand{{Kind: core.OperandSymbol, Name: "x86.flags.add"}}, Address: 0x5503},
					{Kind: core.OpSelect, Results: []core.Value{{ID: 4, Type: integer(64)}}, Operands: []core.Operand{value(2, integer(64)), value(3, integer(64)), value(2, integer(64))}, Address: 0x5504},
					{Kind: core.OpCall, Results: []core.Value{{ID: 5, Type: integer(64)}}, Operands: []core.Operand{value(1, pointer())}, Address: 0x5505},
					{Kind: core.OpReturn, Operands: []core.Operand{value(5, integer(64))}, Address: 0x5506},
				},
			}},
		}},
	}
	result, err := Convert(context.Background(), ir)
	if err != nil {
		t.Fatal(err)
	}
	text := Text(result.Program)
	if strings.Contains(text, "reify.") {
		t.Fatalf("opaque operations were preserved as reify calls:\n%s", text)
	}
	if calls := countCalls(result.Program); calls != 1 {
		t.Fatalf("calls = %d, want 1 (only the LowIR call)", calls)
	}
}

func TestCallCapturesArgumentRegisters(t *testing.T) {
	ir := &core.LowIR{
		Target: testTarget(),
		Functions: []core.Function{{
			ID:      1,
			Address: 0x5800,
			Blocks: []core.Block{{
				ID:      1,
				Address: 0x5800,
				Instructions: []core.Instruction{
					{Kind: core.OpConstant, Results: []core.Value{{ID: 1, Type: integer(64)}}, Operands: []core.Operand{literal(0x10)}, Address: 0x5800},
					{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rdi", 64), value(1, integer(64))}, Address: 0x5801},
					{Kind: core.OpConstant, Results: []core.Value{{ID: 2, Type: integer(32)}}, Operands: []core.Operand{literal(0x20)}, Address: 0x5802},
					{Kind: core.OpWriteRegister, Operands: []core.Operand{register("esi", 32), value(2, integer(32))}, Address: 0x5803},
					{Kind: core.OpCall, Operands: []core.Operand{{Kind: core.OperandSymbol, Name: "helper"}}, Address: 0x5804},
					{Kind: core.OpCall, Operands: []core.Operand{{Kind: core.OperandSymbol, Name: "helper2"}}, Address: 0x5805},
					{Kind: core.OpReturn, Address: 0x5806},
				},
			}},
		}},
	}
	result, err := Convert(context.Background(), ir)
	if err != nil {
		t.Fatal(err)
	}
	var calls []*ssa.Call
	result.Program.Funcs.ForEach(func(_ string, function *ssa.Function) bool {
		if function == nil || function.IsExtern() {
			return true
		}
		for _, blockID := range function.Blocks {
			block, ok := function.GetBasicBlockByID(blockID)
			if !ok || block == nil {
				continue
			}
			for _, instructionID := range block.Insts {
				instruction, ok := function.GetInstructionById(instructionID)
				if !ok || instruction == nil {
					continue
				}
				if call, ok := ssa.ToCall(instruction); ok {
					calls = append(calls, call)
				}
			}
		}
		return true
	})
	if len(calls) != 2 {
		t.Fatalf("calls = %d", len(calls))
	}
	if len(calls[0].Args) != 2 {
		t.Fatalf("first call args = %d, want 2", len(calls[0].Args))
	}
	if len(calls[1].Args) != 0 {
		t.Fatalf("second call args = %d, want 0", len(calls[1].Args))
	}
}

func TestReturnUsesRAX(t *testing.T) {
	ir := &core.LowIR{
		Target: testTarget(),
		Functions: []core.Function{{
			ID:      1,
			Address: 0x5a00,
			Blocks: []core.Block{{
				ID:      1,
				Address: 0x5a00,
				Instructions: []core.Instruction{
					{Kind: core.OpConstant, Results: []core.Value{{ID: 1, Type: integer(64)}}, Operands: []core.Operand{literal(0x7)}, Address: 0x5a00},
					{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rax", 64), value(1, integer(64))}, Address: 0x5a01},
					{Kind: core.OpReturn, Address: 0x5a02},
				},
			}},
		}},
	}
	result, err := Convert(context.Background(), ir)
	if err != nil {
		t.Fatal(err)
	}
	source, err := ssa2source.Render(context.Background(), result.Program)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "return 0x7;") {
		t.Fatalf("return does not use rax:\n%s", source)
	}
}

func TestCallReturnValueUsesRAX(t *testing.T) {
	ir := &core.LowIR{
		Target: testTarget(),
		Functions: []core.Function{{
			ID:      1,
			Address: 0x5b00,
			Blocks: []core.Block{{
				ID:      1,
				Address: 0x5b00,
				Instructions: []core.Instruction{
					{Kind: core.OpCall, Operands: []core.Operand{{Kind: core.OperandSymbol, Name: "helper"}}, Address: 0x5b00},
					{Kind: core.OpReadRegister, Results: []core.Value{{ID: 1, Type: integer(64)}}, Operands: []core.Operand{register("rax", 64)}, Address: 0x5b01},
					{Kind: core.OpReturn, Operands: []core.Operand{value(1, integer(64))}, Address: 0x5b02},
				},
			}},
		}},
	}
	result, err := Convert(context.Background(), ir)
	if err != nil {
		t.Fatal(err)
	}
	source, err := ssa2source.Render(context.Background(), result.Program)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, " = helper(") || !strings.Contains(source, "return v") {
		t.Fatalf("call return value is not returned:\n%s", source)
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
