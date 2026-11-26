package mips64

import (
	"bytes"
	"context"
	"embed"
	"encoding/binary"
	"errors"
	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/loader/arch/internal/archtest"
	"strings"
	"testing"
)

func target() core.Target {
	return core.Target{OS: core.OSLinux, Architecture: core.ArchMIPS64, Bits: 64, ByteOrder: core.BigEndian}
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
		code = binary.BigEndian.AppendUint32(code, value)
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

func TestMIPSAvailableAndChecksTarget(t *testing.T) {
	parser := Parser{}
	if !parser.Available() {
		t.Fatal("mips parser reports no decoder")
	}
	if parser.Kind() != core.ArchMIPS64 || parser.Name() != "mips64" {
		t.Fatalf("unexpected identity: %s %s", parser.Kind(), parser.Name())
	}
	if err := parser.Check(target()); err != nil {
		t.Fatalf("target rejected: %v", err)
	}
	for _, invalid := range []core.Target{
		{Architecture: core.ArchMIPS, Bits: 32},
		{Architecture: core.ArchMIPS64, Bits: 32},
		{Architecture: core.ArchMIPS64, Bits: 64, Mode: "micromips"},
	} {
		if err := parser.Check(invalid); err == nil {
			t.Fatalf("%+v accepted", invalid)
		}
	}
}

func TestMIPSLiftsArithmeticAndReturn(t *testing.T) {
	// addiu v0, a0, 1 ; jr ra
	result := parseAt(t, 0x10000, words(0x24820001, 0x03e00008))
	if _, ok := findInstruction(result.IR, core.OpAdd, nil); !ok {
		t.Fatal("addiu did not lift to add")
	}
	if _, ok := findInstruction(result.IR, core.OpReturn, nil); !ok {
		t.Fatal("jr ra did not lift to return")
	}
	names := map[string]bool{}
	for _, instruction := range allInstructions(result.IR) {
		if instruction.Kind == core.OpWriteRegister && len(instruction.Operands) > 0 {
			names[instruction.Operands[0].Name] = true
		}
	}
	if !names["v0"] {
		t.Fatalf("destination register was not written: %+v", names)
	}
}

func TestMIPSBranchesAndCalls(t *testing.T) {
	// beq a0, a1, +8 ; nop ; addiu a0, a0, 1 ; jr ra ; nop
	// The delay slot executes before the branch, and the taken target is the
	// instruction after the delay slot.
	result := parseAt(t, 0x10100, words(0x10850002, 0x00000000, 0x24840001, 0x03e00008, 0x00000000))
	branch, ok := findInstruction(result.IR, core.OpBranch, nil)
	if !ok || len(branch.Targets) != 2 {
		t.Fatalf("conditional branch has no two targets: %+v", branch)
	}
	addresses := map[uint64]bool{}
	for _, function := range result.IR.Functions {
		for _, block := range function.Blocks {
			addresses[block.Address] = true
		}
	}
	if !addresses[0x10108] || !addresses[0x1010c] {
		t.Fatalf("branch targets are wrong: %+v", addresses)
	}
	// jal 0x10118 ; nop ; jr ra ; nop
	result = parseAt(t, 0x10110, words(0x0c004006, 0x00000000, 0x03e00008, 0x00000000))
	if _, ok := findInstruction(result.IR, core.OpCall, nil); !ok {
		t.Fatal("jal did not lift to a call")
	}
}

func TestMIPSLoadsAndStores(t *testing.T) {
	// lw v0, 8(a0) ; sw v0, 8(a0) ; jr ra
	result := parseAt(t, 0x10200, words(0x8c820008, 0xac820008, 0x03e00008))
	if _, ok := findInstruction(result.IR, core.OpLoad, nil); !ok {
		t.Fatal("lw did not lift to a load")
	}
	if _, ok := findInstruction(result.IR, core.OpStore, nil); !ok {
		t.Fatal("sw did not lift to a store")
	}
}

func TestMIPSLuiAndDisassembly(t *testing.T) {
	// lui a0, 0x1234 ; jr ra
	result := parseAt(t, 0x10300, words(0x3c041234, 0x03e00008))
	if _, ok := findInstruction(result.IR, core.OpConstant, func(instruction core.Instruction) bool {
		return len(instruction.Operands) > 0 && len(instruction.Operands[0].Literal) >= 4 &&
			binary.LittleEndian.Uint32(instruction.Operands[0].Literal) == 0x12340000
	}); !ok {
		t.Fatal("lui did not produce the shifted constant")
	}
	disassembly, err := (Parser{}).Disassemble(context.Background(), requestAt(t, 0x10300, words(0x3c041234, 0x03e00008)))
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"lui", "jr", "0x10300"} {
		if !strings.Contains(disassembly.Text, fragment) {
			t.Fatalf("disassembly lacks %q:\n%s", fragment, disassembly.Text)
		}
	}
}

func TestMIPSContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (Parser{}).Parse(ctx, requestAt(t, 0x10400, words(0x03e00008)))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

//go:embed testdata
var testdata embed.FS

// TestFixtures runs every case in testdata. The cases carry the expectations in
// JSON: the decoded bytes, the LowIR text, the diagnostics, and the counts the
// frame, canary and ABI checks must produce.
func TestFixtures(t *testing.T) {
	archtest.Run(t, archtest.Config{
		FS:     testdata,
		Dir:    "testdata",
		Parser: Parser{},
		Target: core.Target{OS: core.OSLinux, Architecture: core.ArchMIPS64, Bits: 64, ByteOrder: core.BigEndian},
	})
}
