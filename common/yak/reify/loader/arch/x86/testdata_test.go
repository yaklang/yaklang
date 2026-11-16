package x86

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

func target386() core.Target {
	return core.Target{OS: core.OSLinux, Architecture: core.Arch386, Bits: 32, ByteOrder: core.LittleEndian}
}

func requestAt(t *testing.T, address uint64, code []byte) core.ParseRequest {
	t.Helper()
	reader, err := core.NewByteReader("test", bytes.NewReader(code), int64(len(code)))
	if err != nil {
		t.Fatal(err)
	}
	return core.ParseRequest{Target: target386(), Address: address, Reader: reader}
}

func parseAt(t *testing.T, address uint64, code []byte) *core.ParseResult {
	t.Helper()
	result, err := (Parser{}).Parse(context.Background(), requestAt(t, address, code))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return result
}

func allBlocks(ir *core.LowIR) []core.Block {
	var result []core.Block
	for _, function := range ir.Functions {
		result = append(result, function.Blocks...)
	}
	return result
}

func allInstructions(ir *core.LowIR) []core.Instruction {
	var result []core.Instruction
	for _, block := range allBlocks(ir) {
		result = append(result, block.Instructions...)
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

func Test386AvailableAndChecksTarget(t *testing.T) {
	parser := Parser{}
	if !parser.Available() {
		t.Fatal("386 parser reports no decoder")
	}
	if parser.Kind() != core.Arch386 || parser.Name() != "386" {
		t.Fatalf("unexpected identity: %s %s", parser.Kind(), parser.Name())
	}
	if err := parser.Check(target386()); err != nil {
		t.Fatalf("386 target rejected: %v", err)
	}
	for _, target := range []core.Target{
		{Architecture: core.ArchAMD64, Bits: 64},
		{Architecture: core.Arch386, Bits: 64},
		{Architecture: core.Arch386, Bits: 32, Mode: "protected"},
	} {
		if err := parser.Check(target); err == nil {
			t.Fatalf("%+v accepted", target)
		}
	}
}

func Test386UsesThirtyTwoBitRegistersAndStack(t *testing.T) {
	// push ebp; mov ebp, esp; sub esp, 8; leave; ret
	code := []byte{0x55, 0x89, 0xe5, 0x83, 0xec, 0x08, 0xc9, 0xc3}
	result := parseAt(t, 0x401000, code)
	names := map[string]bool{}
	for _, instruction := range allInstructions(result.IR) {
		if instruction.Kind != core.OpWriteRegister || len(instruction.Operands) == 0 {
			continue
		}
		names[instruction.Operands[0].Name] = true
		if instruction.Operands[0].Type.Bits != 32 {
			t.Fatalf("write width = %d, want 32", instruction.Operands[0].Type.Bits)
		}
	}
	for _, name := range []string{"esp", "ebp"} {
		if !names[name] {
			t.Fatalf("%s was not written: %+v", name, names)
		}
	}
	if names["rsp"] || names["rbp"] {
		t.Fatalf("64-bit register names leaked into 386 output: %+v", names)
	}
	// push ebp subtracts four bytes, not eight.
	slot, ok := findInstruction(result.IR, core.OpConstant, func(instruction core.Instruction) bool {
		if len(instruction.Operands) == 0 || instruction.Operands[0].Kind != core.OperandConstant {
			return false
		}
		return binary.LittleEndian.Uint32(append(instruction.Operands[0].Literal, 0, 0, 0, 0)[:4]) == 4
	})
	if !ok {
		t.Fatalf("push did not use a four-byte slot: %+v", slot)
	}
}

func Test386ConditionalBranchUsesFlags(t *testing.T) {
	// test eax, eax; je +2; nop; nop; ret
	code := []byte{0x85, 0xc0, 0x74, 0x02, 0x90, 0x90, 0xc3}
	result := parseAt(t, 0x402000, code)
	if blocks := allBlocks(result.IR); len(blocks) != 3 {
		t.Fatalf("wanted three blocks, got %d", len(blocks))
	}
	branch, ok := findInstruction(result.IR, core.OpBranch, nil)
	if !ok || len(branch.Targets) != 2 {
		t.Fatalf("conditional branch has no two targets: %+v", branch)
	}
	flags := false
	for _, instruction := range allInstructions(result.IR) {
		if instruction.Kind != core.OpWriteRegister || len(instruction.Operands) == 0 {
			continue
		}
		if instruction.Operands[0].Name == "eflags" {
			flags = true
		}
	}
	if !flags {
		t.Fatal("flags were not written to eflags")
	}
}

func Test386Disassembles(t *testing.T) {
	disassembly, err := (Parser{}).Disassemble(context.Background(), requestAt(t, 0x403000, []byte{0x55, 0xc3}))
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"push ebp", "ret", "0x403000  55"} {
		if !strings.Contains(disassembly.Text, fragment) {
			t.Fatalf("disassembly lacks %q:\n%s", fragment, disassembly.Text)
		}
	}
}

func Test386ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (Parser{}).Parse(ctx, requestAt(t, 0x404000, []byte{0xc3}))
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
		Target: core.Target{OS: core.OSLinux, Architecture: core.Arch386, Bits: 32, ByteOrder: core.LittleEndian},
	})
}
