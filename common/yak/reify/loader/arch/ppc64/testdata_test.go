package ppc64

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
	return core.Target{OS: core.OSLinux, Architecture: core.ArchPPC64, Bits: 64, ByteOrder: core.BigEndian}
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

func TestPPCAvailableAndChecksTarget(t *testing.T) {
	parser := Parser{}
	if !parser.Available() {
		t.Fatal("ppc64 parser reports no decoder")
	}
	if parser.Kind() != core.ArchPPC64 || parser.Name() != "ppc64" {
		t.Fatalf("unexpected identity: %s %s", parser.Kind(), parser.Name())
	}
	if err := parser.Check(target()); err != nil {
		t.Fatalf("target rejected: %v", err)
	}
	for _, invalid := range []core.Target{
		{Architecture: core.ArchPPC, Bits: 32},
		{Architecture: core.ArchPPC64, Bits: 32},
		{Architecture: core.ArchPPC64, Bits: 64, Mode: "le"},
	} {
		if err := parser.Check(invalid); err == nil {
			t.Fatalf("%+v accepted", invalid)
		}
	}
}

func TestPPCLiftsArithmeticAndReturn(t *testing.T) {
	// addi r3, r4, 1 ; blr
	result := parseAt(t, 0x10000, words(0x38640001, 0x4e800020))
	if _, ok := findInstruction(result.IR, core.OpAdd, nil); !ok {
		t.Fatal("addi did not lift to add")
	}
	if _, ok := findInstruction(result.IR, core.OpReturn, nil); !ok {
		t.Fatal("blr did not lift to return")
	}
	names := map[string]bool{}
	for _, instruction := range allInstructions(result.IR) {
		if instruction.Kind == core.OpWriteRegister && len(instruction.Operands) > 0 {
			names[instruction.Operands[0].Name] = true
		}
	}
	if !names["r3"] {
		t.Fatalf("destination register was not written: %+v", names)
	}
}

func TestPPCBranchesAndCalls(t *testing.T) {
	// cmpwi r3, 0 ; beq +8 ; nop ; blr
	result := parseAt(t, 0x10100, words(0x2c030000, 0x41820008, 0x60000000, 0x4e800020))
	branch, ok := findInstruction(result.IR, core.OpBranch, nil)
	if !ok || len(branch.Targets) != 2 {
		t.Fatalf("conditional branch has no two targets: %+v", branch)
	}
	// bl +8 ; nop ; blr
	result = parseAt(t, 0x10110, words(0x48000009, 0x60000000, 0x4e800020))
	if _, ok := findInstruction(result.IR, core.OpCall, nil); !ok {
		t.Fatal("bl did not lift to a call")
	}
}

func TestPPCLoadsAndStores(t *testing.T) {
	// ld r3, 8(r4) ; std r3, 8(r4) ; blr
	result := parseAt(t, 0x10200, words(0xe8640008, 0xf8640008, 0x4e800020))
	if _, ok := findInstruction(result.IR, core.OpLoad, nil); !ok {
		t.Fatal("ld did not lift to a load")
	}
	if _, ok := findInstruction(result.IR, core.OpStore, nil); !ok {
		t.Fatal("std did not lift to a store")
	}
}

func TestPPCDisassembles(t *testing.T) {
	disassembly, err := (Parser{}).Disassemble(context.Background(), requestAt(t, 0x10300, words(0x38640001, 0x4e800020)))
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"addi", "blr", "0x10300"} {
		if !strings.Contains(disassembly.Text, fragment) {
			t.Fatalf("disassembly lacks %q:\n%s", fragment, disassembly.Text)
		}
	}
}

func TestPPCContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (Parser{}).Parse(ctx, requestAt(t, 0x10400, words(0x4e800020)))
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
		Target: core.Target{OS: core.OSLinux, Architecture: core.ArchPPC64, Bits: 64, ByteOrder: core.BigEndian},
	})
}
