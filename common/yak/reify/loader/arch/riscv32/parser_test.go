package riscv32

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func target() core.Target {
	return core.Target{OS: core.OSLinux, Architecture: core.ArchRISCV32, Bits: 32, ByteOrder: core.LittleEndian}
}

func requestAt(t *testing.T, address uint64, code []byte) core.ParseRequest {
	t.Helper()
	reader, err := core.NewByteReader("test", bytes.NewReader(code), int64(len(code)))
	if err != nil {
		t.Fatal(err)
	}
	return core.ParseRequest{Target: target(), Address: address, Reader: reader}
}

func words(values ...uint32) []byte {
	code := make([]byte, 0, len(values)*4)
	for _, value := range values {
		code = binary.LittleEndian.AppendUint32(code, value)
	}
	return code
}

func parseAt(t *testing.T, address uint64, code []byte) *core.ParseResult {
	t.Helper()
	result, err := (Parser{}).Parse(context.Background(), requestAt(t, address, code))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return result
}

func allInstructions(ir *core.LowIR) []core.Instruction {
	var result []core.Instruction
	for _, function := range ir.Functions {
		for _, block := range function.Blocks {
			result = append(result, block.Instructions...)
		}
	}
	return result
}

func findInstruction(ir *core.LowIR, kind core.Opcode, predicate func(core.Instruction) bool) (core.Instruction, bool) {
	for _, instruction := range allInstructions(ir) {
		if instruction.Kind == kind && (predicate == nil || predicate(instruction)) {
			return instruction, true
		}
	}
	return core.Instruction{}, false
}

func TestRISCVAvailableAndChecksTarget(t *testing.T) {
	parser := Parser{}
	if !parser.Available() {
		t.Fatal("riscv64 parser reports no decoder")
	}
	if parser.Kind() != core.ArchRISCV32 || parser.Name() != "riscv32" {
		t.Fatalf("unexpected identity: %s %s", parser.Kind(), parser.Name())
	}
	if err := parser.Check(target()); err != nil {
		t.Fatalf("target rejected: %v", err)
	}
	for _, invalid := range []core.Target{
		{Architecture: core.ArchRISCV64, Bits: 64},
		{Architecture: core.ArchRISCV32, Bits: 64},
		{Architecture: core.ArchRISCV32, Bits: 32, Mode: "c"},
	} {
		if err := parser.Check(invalid); err == nil {
			t.Fatalf("%+v accepted", invalid)
		}
	}
}

func TestRISCVLiftsArithmeticAndReturn(t *testing.T) {
	// addi a0, a1, 1 ; ret
	result := parseAt(t, 0x10000, words(0x00158513, 0x00008067))
	if _, ok := findInstruction(result.IR, core.OpAdd, nil); !ok {
		t.Fatal("addi did not lift to add")
	}
	if _, ok := findInstruction(result.IR, core.OpReturn, nil); !ok {
		t.Fatal("ret did not lift to return")
	}
	names := map[string]bool{}
	for _, instruction := range allInstructions(result.IR) {
		if instruction.Kind == core.OpWriteRegister && len(instruction.Operands) > 0 {
			names[instruction.Operands[0].Name] = true
		}
	}
	if !names["x10"] {
		t.Fatalf("destination register was not written: %+v", names)
	}
}

func TestRISCVBranchesAndCalls(t *testing.T) {
	// beq a0, a1, +8 ; nop ; ret
	result := parseAt(t, 0x10100, words(0x00b50463, 0x00000013, 0x00008067))
	branch, ok := findInstruction(result.IR, core.OpBranch, nil)
	if !ok || len(branch.Targets) != 2 {
		t.Fatalf("conditional branch has no two targets: %+v", branch)
	}
	// jal ra, +8 ; nop ; ret
	result = parseAt(t, 0x10110, words(0x008000ef, 0x00000013, 0x00008067))
	call, ok := findInstruction(result.IR, core.OpCall, nil)
	if !ok {
		t.Fatal("jal did not lift to a call")
	}
	if call.Operands[0].Kind != core.OperandConstant {
		t.Fatalf("call target is not a constant: %+v", call)
	}
}

func TestRISCVLoadsAndStores(t *testing.T) {
	// lw a0, 8(a1) ; sd a0, 8(a1) ; ret
	result := parseAt(t, 0x10200, words(0x0085a503, 0x00a5b423, 0x00008067))
	if _, ok := findInstruction(result.IR, core.OpLoad, nil); !ok {
		t.Fatal("lw did not lift to a load")
	}
	if _, ok := findInstruction(result.IR, core.OpStore, nil); !ok {
		t.Fatal("sd did not lift to a store")
	}
}

func TestRISCVDisassembles(t *testing.T) {
	disassembly, err := (Parser{}).Disassemble(context.Background(), requestAt(t, 0x10300, words(0x00158513, 0x00008067)))
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"ADDI", "JALR", "0x10300"} {
		if !strings.Contains(disassembly.Text, fragment) {
			t.Fatalf("disassembly lacks %q:\n%s", fragment, disassembly.Text)
		}
	}
}

func TestRISCVContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (Parser{}).Parse(ctx, requestAt(t, 0x10400, words(0x00008067)))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}
