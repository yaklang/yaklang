package arm64

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowirtext"
)

func arm64Target() core.Target {
	return core.Target{OS: core.OSDarwin, Architecture: core.ArchARM64, Bits: 64, ByteOrder: core.LittleEndian}
}

func arm64Reader(t *testing.T, code []byte) *core.ByteReader {
	t.Helper()
	reader, err := core.NewByteReader("test", bytes.NewReader(code), int64(len(code)))
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

func arm64Request(t *testing.T, address uint64, code []byte) core.ParseRequest {
	t.Helper()
	return core.ParseRequest{Target: arm64Target(), Address: address, Reader: arm64Reader(t, code)}
}

// words assembles a list of 32-bit instruction encodings into little-endian
// code bytes.
func words(values ...uint32) []byte {
	code := make([]byte, 0, len(values)*4)
	for _, value := range values {
		code = binary.LittleEndian.AppendUint32(code, value)
	}
	return code
}

func parseARM64At(t *testing.T, address uint64, code []byte) *core.ParseResult {
	t.Helper()
	result, err := (Parser{}).Parse(context.Background(), arm64Request(t, address, code))
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

func findSymbol(ir *core.LowIR, kind core.Opcode, name string) (core.Instruction, bool) {
	return findInstruction(ir, kind, func(instruction core.Instruction) bool {
		return len(instruction.Operands) > 0 && instruction.Operands[0].Kind == core.OperandSymbol && instruction.Operands[0].Name == name
	})
}

func writeRegisterNames(ir *core.LowIR) []string {
	var names []string
	for _, instruction := range allInstructions(ir) {
		if instruction.Kind != core.OpWriteRegister || len(instruction.Operands) == 0 {
			continue
		}
		names = append(names, instruction.Operands[0].Name)
	}
	return names
}

func diagnosticCount(result *core.ParseResult, code string) int {
	count := 0
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == code {
			count++
		}
	}
	return count
}

func literalUint64(t *testing.T, literal []byte) uint64 {
	t.Helper()
	var padded [8]byte
	copy(padded[:], literal)
	return binary.LittleEndian.Uint64(padded[:])
}

func TestARM64AvailableAndChecksTarget(t *testing.T) {
	parser := Parser{}
	if !parser.Available() {
		t.Fatal("arm64 parser reports no decoder")
	}
	if parser.Kind() != core.ArchARM64 || parser.Name() != "arm64" {
		t.Fatalf("unexpected identity: %s %s", parser.Kind(), parser.Name())
	}
	valid := []core.Target{
		{Architecture: core.ArchARM64, Bits: 64, ByteOrder: core.BigEndian},
		{Architecture: core.ArchARM64, Bits: 64, Mode: "arm64"},
	}
	for _, target := range valid {
		if err := parser.Check(target); err != nil {
			t.Fatalf("%+v rejected: %v", target, err)
		}
	}
	invalid := []core.Target{
		{Architecture: core.ArchAMD64, Bits: 64},
		{Architecture: core.ArchARM64, Bits: 32},
		{Architecture: core.ArchARM64, Bits: 64, Mode: "thumb"},
	}
	for _, target := range invalid {
		if err := parser.Check(target); err == nil {
			t.Fatalf("%+v accepted", target)
		}
	}
}

func TestARM64Disassembles(t *testing.T) {
	code := words(
		0xd10083ff, // sub sp, sp, #0x20
		0xa9017bfd, // stp x29, x30, [sp, #16]
		0xd65f03c0, // ret
	)
	disassembler := Parser{}
	disassembly, err := disassembler.Disassemble(context.Background(), arm64Request(t, 0x1000, code))
	if err != nil {
		t.Fatal(err)
	}
	if disassembly.Instructions != 3 {
		t.Fatalf("instructions = %d", disassembly.Instructions)
	}
	for _, fragment := range []string{"sub sp, sp, #0x20", "stp x29, x30, [sp,#16]", "ret"} {
		if !strings.Contains(disassembly.Text, fragment) {
			t.Fatalf("disassembly lacks %q:\n%s", fragment, disassembly.Text)
		}
	}
	if !strings.Contains(disassembly.Text, "0x1000  ff 83 00 d1") {
		t.Fatalf("raw bytes are missing:\n%s", disassembly.Text)
	}
}

func TestARM64PrologueLiftsStackAndReturn(t *testing.T) {
	code := words(
		0xd10083ff, // sub sp, sp, #0x20
		0xa9017bfd, // stp x29, x30, [sp, #16]
		0x910043fd, // add x29, sp, #16
		0xa8c17bfd, // ldp x29, x30, [sp], #16
		0x910083ff, // add sp, sp, #0x20
		0xd65f03c0, // ret
	)
	result := parseARM64At(t, 0x1000, code)
	if result.IR == nil || result.IR.Target != arm64Target() {
		t.Fatalf("bad IR target: %+v", result.IR)
	}
	if len(result.IR.Functions) != 1 || result.IR.Functions[0].Address != 0x1000 {
		t.Fatalf("bad function: %+v", result.IR.Functions)
	}
	blocks := allBlocks(result.IR)
	if len(blocks) != 1 || blocks[0].Instructions[len(blocks[0].Instructions)-1].Kind != core.OpReturn {
		t.Fatalf("block does not end in return: %+v", blocks)
	}
	if _, ok := findInstruction(result.IR, core.OpStore, nil); !ok {
		t.Fatal("stp did not store the frame record")
	}
	if _, ok := findInstruction(result.IR, core.OpLoad, nil); !ok {
		t.Fatal("ldp did not load the frame record")
	}
	names := strings.Join(writeRegisterNames(result.IR), ",")
	for _, register := range []string{"sp", "x29", "x30"} {
		if !strings.Contains(names, register) {
			t.Fatalf("%s was not written: %s", register, names)
		}
	}
	if got := diagnosticCount(result, "decode-error"); got != 0 {
		t.Fatalf("decoded prologue produced %d decode errors", got)
	}
	if got := diagnosticCount(result, "block-falls-off-region"); got != 0 {
		t.Fatalf("terminated block reported a fallthrough: %d", got)
	}
}

func TestARM64ConditionalBranchBuildsTwoTargets(t *testing.T) {
	code := words(
		0x7100405f, // cmp w2, #16
		0x5400006d, // b.le 0x1010
		0xd503201f, // nop
		0xd503201f, // nop
		0xd65f03c0, // ret
	)
	result := parseARM64At(t, 0x1000, code)
	if blocks := allBlocks(result.IR); len(blocks) != 3 {
		t.Fatalf("wanted three blocks, got %d", len(blocks))
	}
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
	if !addresses[0x1010] || !addresses[0x1008] {
		t.Fatalf("branch targets are wrong: %+v", addresses)
	}
	if _, ok := findSymbol(result.IR, core.OpIntrinsic, "flags.cmp"); !ok {
		t.Fatal("cmp did not update the flags")
	}
	if _, ok := findSymbol(result.IR, core.OpIntrinsic, "cond.le"); !ok {
		t.Fatal("branch did not read the less-or-equal condition")
	}
}

func TestARM64CallAndJumpTargets(t *testing.T) {
	code := words(
		0x94000005, // bl 0x2014
		0xd503201f, // nop
		0x14000003, // b 0x2014
		0xd503201f, // nop
		0xd503201f, // nop
		0xd65f03c0, // ret
	)
	result := parseARM64At(t, 0x2000, code)
	call, ok := findInstruction(result.IR, core.OpCall, nil)
	if !ok || len(call.Operands) != 1 || call.Operands[0].Kind != core.OperandConstant {
		t.Fatalf("call target is missing: %+v", call)
	}
	if target := literalUint64(t, call.Operands[0].Literal); target != 0x2014 {
		t.Fatalf("call target = %#x", target)
	}
	jump, ok := findInstruction(result.IR, core.OpJump, func(instruction core.Instruction) bool {
		return len(instruction.Targets) == 1 && instruction.Address == 0x2008
	})
	if !ok {
		t.Fatal("direct jump has no block target")
	}
	for _, block := range allBlocks(result.IR) {
		if block.ID == jump.Targets[0] && block.Address != 0x2014 {
			t.Fatalf("jump target block starts at %#x", block.Address)
		}
	}
}

func TestARM64LoadStoreAddressingModes(t *testing.T) {
	code := words(
		0xf9400420, // ldr x0, [x1, #8]
		0xf9000820, // str x0, [x1, #16]
		0xf8408422, // ldr x2, [x1], #8
		0xf8004423, // str x3, [x1], #4
	)
	result := parseARM64At(t, 0x3000, code)
	loadCount := 0
	storeCount := 0
	for _, instruction := range allInstructions(result.IR) {
		switch instruction.Kind {
		case core.OpLoad:
			loadCount++
		case core.OpStore:
			storeCount++
		}
	}
	if loadCount != 2 || storeCount != 2 {
		t.Fatalf("loads=%d stores=%d", loadCount, storeCount)
	}
	// Post-indexed instructions must write the base register back.
	updates := 0
	for _, instruction := range allInstructions(result.IR) {
		if instruction.Kind == core.OpWriteRegister && len(instruction.Operands) == 2 && instruction.Operands[0].Name == "x1" {
			updates++
		}
	}
	if updates < 2 {
		t.Fatalf("post-indexed base updates = %d", updates)
	}
}

func TestARM64DecodesPointerAuthentication(t *testing.T) {
	code := words(
		0xdac123f0, // paciza x16
		0xdac10020, // pacia x0, x1
		0xd65f0bff, // retaa
	)
	result := parseARM64At(t, 0x4000, code)
	if got := diagnosticCount(result, "decode-error"); got != 0 {
		t.Fatalf("PAC instructions produced %d decode errors", got)
	}
	if _, ok := findInstruction(result.IR, core.OpReturn, nil); !ok {
		t.Fatal("retaa did not lift to a return")
	}
	disassembler := Parser{}
	disassembly, err := disassembler.Disassemble(context.Background(), arm64Request(t, 0x4000, code))
	if err != nil {
		t.Fatal(err)
	}
	for _, mnemonic := range []string{"paciza x16", "pacia x0, x1", "retaa"} {
		if !strings.Contains(disassembly.Text, mnemonic) {
			t.Fatalf("disassembly lacks %q:\n%s", mnemonic, disassembly.Text)
		}
	}
}

func TestARM64UndefinedWordsBecomeTraps(t *testing.T) {
	code := words(0x000004b8, 0x00000000, 0xd65f03c0)
	result := parseARM64At(t, 0x5000, code)
	trap, ok := findInstruction(result.IR, core.OpTrap, nil)
	if !ok || !strings.Contains(trap.Operands[0].Name, "arm64.udf") {
		t.Fatalf("undefined word is not a trap: %+v", trap)
	}
}

func TestARM64VectorInstructionIsOpaque(t *testing.T) {
	code := words(
		0x4e080d00, // dup v0.2d, x8
		0x4f00e400, // movi v0.16b, #0
		0xd65f03c0, // ret
	)
	result := parseARM64At(t, 0x6000, code)
	if got := diagnosticCount(result, "unsupported-instruction"); got != 0 {
		t.Fatalf("vector instructions reported %d unsupported diagnostics", got)
	}
	intrinsic, ok := findInstruction(result.IR, core.OpIntrinsic, func(instruction core.Instruction) bool {
		return len(instruction.Operands) > 0 && instruction.Operands[0].Name == "arm64.dup"
	})
	if !ok || len(intrinsic.Results) != 1 {
		t.Fatalf("dup is not an opaque vector operation: %+v", intrinsic)
	}
}

func TestARM64DecodeErrors(t *testing.T) {
	code := words(0xd4000000, 0x14000000)
	result := parseARM64At(t, 0x7000, code)
	if got := diagnosticCount(result, "decode-error"); got == 0 {
		t.Fatal("undecodable word produced no diagnostic")
	}
}

func TestARM64ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (Parser{}).Parse(ctx, arm64Request(t, 0x8000, words(0xd65f03c0)))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestARM64Deterministic(t *testing.T) {
	code := words(0xd10083ff, 0xa9017bfd, 0xd65f03c0)
	first := parseARM64At(t, 0x9000, code)
	second := parseARM64At(t, 0x9000, code)
	if len(first.IR.Functions) != len(second.IR.Functions) {
		t.Fatal("function count is not deterministic")
	}
	firstText := lowirtext.Format(first.IR)
	secondText := lowirtext.Format(second.IR)
	if firstText != secondText {
		t.Fatal("LowIR text is not deterministic")
	}
	if len(allInstructions(first.IR)) != len(allInstructions(second.IR)) {
		t.Fatal("instruction count is not deterministic")
	}
}
