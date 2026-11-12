package arm

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func targetARM() core.Target {
	return core.Target{OS: core.OSLinux, Architecture: core.ArchARM, Bits: 32, ByteOrder: core.LittleEndian}
}

func requestAt(t *testing.T, address uint64, code []byte) core.ParseRequest {
	t.Helper()
	reader, err := core.NewByteReader("test", bytes.NewReader(code), int64(len(code)))
	if err != nil {
		t.Fatal(err)
	}
	return core.ParseRequest{Target: targetARM(), Address: address, Reader: reader}
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

func TestARMAvailableAndChecksTarget(t *testing.T) {
	parser := Parser{}
	if !parser.Available() {
		t.Fatal("arm parser reports no decoder")
	}
	if parser.Kind() != core.ArchARM || parser.Name() != "arm" {
		t.Fatalf("unexpected identity: %s %s", parser.Kind(), parser.Name())
	}
	if err := parser.Check(targetARM()); err != nil {
		t.Fatalf("arm target rejected: %v", err)
	}
	if err := parser.Check(core.Target{Architecture: core.ArchARM, Bits: 32, Mode: "arm"}); err != nil {
		t.Fatalf("arm mode rejected: %v", err)
	}
	for _, target := range []core.Target{
		{Architecture: core.ArchARM64, Bits: 64},
		{Architecture: core.ArchARM, Bits: 64},
		{Architecture: core.ArchARM, Bits: 32, Mode: "thumb"},
	} {
		if err := parser.Check(target); err == nil {
			t.Fatalf("%+v accepted", target)
		}
	}
}

func TestARMPrologueAndEpilogue(t *testing.T) {
	// push {r4, r11, lr}; mov r11, sp; sub sp, sp, #8; add sp, sp, #8; pop {r4, r11, lr}; bx lr
	code := words(0xe92d4810, 0xe1a0b00d, 0xe24dd008, 0xe28dd008, 0xe8bd4810, 0xe12fff1e)
	result := parseAt(t, 0x11000, code)
	if len(result.IR.Functions) != 1 || result.IR.Functions[0].Address != 0x11000 {
		t.Fatalf("bad function: %+v", result.IR.Functions)
	}
	blocks := allBlocks(result.IR)
	if len(blocks) != 1 || blocks[0].Instructions[len(blocks[0].Instructions)-1].Kind != core.OpReturn {
		t.Fatalf("block does not end in return: %+v", blocks)
	}
	if _, ok := findInstruction(result.IR, core.OpStore, nil); !ok {
		t.Fatal("push did not store to the stack")
	}
	if _, ok := findInstruction(result.IR, core.OpLoad, nil); !ok {
		t.Fatal("pop did not load from the stack")
	}
	names := map[string]bool{}
	for _, instruction := range allInstructions(result.IR) {
		if instruction.Kind == core.OpWriteRegister && len(instruction.Operands) > 0 {
			names[instruction.Operands[0].Name] = true
		}
	}
	for _, name := range []string{"sp", "r11", "r4", "lr"} {
		if !names[name] {
			t.Fatalf("%s was not written: %+v", name, names)
		}
	}
}

func TestARMConditionalBranchBuildsTwoTargets(t *testing.T) {
	// cmp r0, #1; bne +0; nop; bx lr
	code := words(0xe3500001, 0x1a000000, 0xe1a00000, 0xe12fff1e)
	result := parseAt(t, 0x12000, code)
	branch, ok := findInstruction(result.IR, core.OpBranch, nil)
	if !ok || len(branch.Targets) != 2 {
		t.Fatalf("conditional branch has no two targets: %+v", branch)
	}
	addresses := map[uint64]bool{}
	for _, id := range branch.Targets {
		for _, block := range allBlocks(result.IR) {
			if block.ID == id {
				addresses[block.Address] = true
			}
		}
	}
	if !addresses[0x1200c] || !addresses[0x12008] {
		t.Fatalf("branch targets are wrong: %+v", addresses)
	}
	if _, ok := findInstruction(result.IR, core.OpIntrinsic, func(instruction core.Instruction) bool {
		return len(instruction.Operands) > 0 && instruction.Operands[0].Name == "arm.cond.ne"
	}); !ok {
		t.Fatal("branch did not read the not-equal condition")
	}
}

func TestARMCallAndReturn(t *testing.T) {
	// bl +0; nop; bx lr
	code := words(0xeb000000, 0xe1a00000, 0xe12fff1e)
	result := parseAt(t, 0x13000, code)
	call, ok := findInstruction(result.IR, core.OpCall, nil)
	if !ok || len(call.Operands) != 1 || call.Operands[0].Kind != core.OperandConstant {
		t.Fatalf("call target is missing: %+v", call)
	}
	if binary.LittleEndian.Uint32(append(call.Operands[0].Literal, 0, 0, 0, 0)[:4]) != 0x13008 {
		t.Fatalf("call target = % x", call.Operands[0].Literal)
	}
	if _, ok := findInstruction(result.IR, core.OpReturn, nil); !ok {
		t.Fatal("bx lr did not lift to a return")
	}
}

func TestARMLoadStoreAddressingModes(t *testing.T) {
	// ldr r1, [r0, #8]; str r1, [r0], #4; ldrh r2, [r0, #-2]
	code := words(0xe5901008, 0xe4801004, 0xe1d020b2)
	result := parseAt(t, 0x14000, code)
	loads := 0
	stores := 0
	for _, instruction := range allInstructions(result.IR) {
		switch instruction.Kind {
		case core.OpLoad:
			loads++
		case core.OpStore:
			stores++
		}
	}
	if loads != 2 || stores != 1 {
		t.Fatalf("loads=%d stores=%d", loads, stores)
	}
	// The post-indexed store must write r0 back.
	updates := 0
	for _, instruction := range allInstructions(result.IR) {
		if instruction.Kind == core.OpWriteRegister && len(instruction.Operands) > 0 && instruction.Operands[0].Name == "r0" {
			updates++
		}
	}
	if updates < 1 {
		t.Fatalf("post-indexed base update missing: %d", updates)
	}
}

func TestARMBitfieldAndExtendAreExplicit(t *testing.T) {
	// uxtb r0, r1; ubfx r2, r3, #4, #8
	code := words(0xe6ef0071, 0xe7e32053)
	result := parseAt(t, 0x15000, code)
	if _, ok := findInstruction(result.IR, core.OpZeroExtend, nil); !ok {
		t.Fatal("uxtb did not lift to a zero extension")
	}
	if _, ok := findInstruction(result.IR, core.OpIntrinsic, func(instruction core.Instruction) bool {
		return len(instruction.Operands) > 0 && instruction.Operands[0].Name == "arm.ubfx"
	}); !ok {
		t.Fatal("ubfx did not lift to an intrinsic")
	}
}

func TestARMUndecodableWordIsKept(t *testing.T) {
	// UDF #0 is undecodable by armasm and must stay in the IR as a trap.
	code := words(0xe7f000f0, 0xe12fff1e)
	result := parseAt(t, 0x16000, code)
	trap, ok := findInstruction(result.IR, core.OpTrap, func(instruction core.Instruction) bool {
		return len(instruction.Operands) > 0 && instruction.Operands[0].Name == "arm.data"
	})
	if !ok {
		t.Fatalf("undecodable word did not lift to a trap: %+v", trap)
	}
	if len(result.Diagnostics) == 0 || result.Diagnostics[0].Code != "decode-error" {
		t.Fatalf("missing decode-error diagnostic: %+v", result.Diagnostics)
	}
}

func TestARMDisassembles(t *testing.T) {
	// mov r0, #42; bx lr
	disassembly, err := (Parser{}).Disassemble(context.Background(), requestAt(t, 0x17000, words(0xe3a0002a, 0xe12fff1e)))
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"mov r0, #42", "bx lr", "0x17000  2a 00 a0 e3"} {
		if !strings.Contains(disassembly.Text, fragment) {
			t.Fatalf("disassembly lacks %q:\n%s", fragment, disassembly.Text)
		}
	}
}

func TestARMContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (Parser{}).Parse(ctx, requestAt(t, 0x18000, words(0xe12fff1e)))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}
