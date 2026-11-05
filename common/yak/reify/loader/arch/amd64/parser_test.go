package amd64

import (
	"bytes"
	"context"
	"debug/elf"
	"encoding/binary"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func amd64Target() core.Target {
	return core.Target{OS: core.OSLinux, Architecture: core.ArchAMD64, Bits: 64, ByteOrder: core.LittleEndian}
}

func amd64Reader(t *testing.T, code []byte) *core.ByteReader {
	t.Helper()
	reader, err := core.NewByteReader("test", bytes.NewReader(code), int64(len(code)))
	if err != nil {
		t.Fatal(err)
	}
	reader, err = reader.WithOrder(core.LittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

func amd64Request(t *testing.T, address uint64, code []byte) core.ParseRequest {
	t.Helper()
	return core.ParseRequest{Target: amd64Target(), Address: address, Reader: amd64Reader(t, code)}
}

func parseAMD64At(t *testing.T, address uint64, code []byte) *core.ParseResult {
	t.Helper()
	result, err := (Parser{}).Parse(context.Background(), amd64Request(t, address, code))
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

func valueDefinition(ir *core.LowIR, id core.ValueID) (core.Instruction, bool) {
	for _, instruction := range allInstructions(ir) {
		for _, result := range instruction.Results {
			if result.ID == id {
				return instruction, true
			}
		}
	}
	return core.Instruction{}, false
}

func literalUint64(t *testing.T, literal []byte) uint64 {
	t.Helper()
	var padded [8]byte
	copy(padded[:], literal)
	return binary.LittleEndian.Uint64(padded[:])
}

func resolveCopy(ir *core.LowIR, id core.ValueID) core.Instruction {
	definition, ok := valueDefinition(ir, id)
	for ok && definition.Kind == core.OpCopy && len(definition.Operands) == 1 && definition.Operands[0].Kind == core.OperandValue {
		id = definition.Operands[0].Value
		definition, ok = valueDefinition(ir, id)
	}
	return definition
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

func TestAMD64AvailableAndChecksTarget(t *testing.T) {
	if !(Parser{}).Available() {
		t.Fatal("amd64 decoder is not available")
	}
	if err := (Parser{}).Check(amd64Target()); err != nil {
		t.Fatalf("linux/amd64 rejected: %v", err)
	}
	if err := (Parser{}).Check(core.Target{OS: core.OSLinux, Architecture: core.ArchAMD64, Bits: 64, ByteOrder: core.BigEndian}); err != nil {
		t.Fatalf("parser interpreted target byte order: %v", err)
	}
	for _, target := range []core.Target{
		{OS: core.OSLinux, Architecture: core.ArchAMD64, Bits: 32, ByteOrder: core.LittleEndian},
		{OS: core.OSLinux, Architecture: core.Arch386, Bits: 32, ByteOrder: core.LittleEndian},
	} {
		if err := (Parser{}).Check(target); err == nil {
			t.Fatalf("invalid target accepted: %+v", target)
		}
	}
}

func TestAMD64Disassembles(t *testing.T) {
	request := amd64Request(t, 0x1000, []byte{0xf3, 0x0f, 0x1e, 0xfa, 0x48, 0x89, 0xe5, 0xc3})
	result, err := (Parser{}).Disassemble(context.Background(), request)
	if err != nil {
		t.Fatalf("disassemble: %v", err)
	}
	if result.Instructions != 3 || len(result.Diagnostics) != 0 {
		t.Fatalf("bad disassembly: %+v", result)
	}
	for _, fragment := range []string{"f3 0f 1e fa", "endbr64", "mov", "ret"} {
		if !strings.Contains(result.Text, fragment) {
			t.Fatalf("disassembly lacks %q: %s", fragment, result.Text)
		}
	}

	partial, err := (Parser{}).Disassemble(context.Background(), amd64Request(t, 0x2000, []byte{0x90, 0x0f}))
	if err != nil {
		t.Fatalf("partial disassemble: %v", err)
	}
	decodeErrors := 0
	for _, diagnostic := range partial.Diagnostics {
		if diagnostic.Code == "decode-error" {
			decodeErrors++
		}
	}
	if partial.Instructions != 1 || decodeErrors != 1 {
		t.Fatalf("bad partial disassembly: %+v", partial)
	}
}

func TestAMD64DisassembleRejectsEmptyAndCancellation(t *testing.T) {
	if _, err := (Parser{}).Disassemble(context.Background(), core.ParseRequest{Target: amd64Target()}); err == nil {
		t.Fatal("empty code accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Parser{}).Disassemble(ctx, amd64Request(t, 0x3000, []byte{0x90})); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestAMD64PrologueLiftsStackAndReturn(t *testing.T) {
	code := []byte{
		0xf3, 0x0f, 0x1e, 0xfa, // endbr64
		0x55,             // push rbp
		0x48, 0x89, 0xe5, // mov rbp, rsp
		0x48, 0x83, 0xec, 0x10, // sub rsp, 0x10
		0xc9, // leave
		0xc3, // ret
	}
	result := parseAMD64At(t, 0x1000, code)
	if result.IR == nil || result.IR.Target != amd64Target() {
		t.Fatalf("bad IR target: %+v", result.IR)
	}
	if len(result.IR.Functions) != 1 || result.IR.Functions[0].ID != 1 || result.IR.Functions[0].Address != 0x1000 {
		t.Fatalf("bad function: %+v", result.IR.Functions)
	}
	blocks := allBlocks(result.IR)
	if len(blocks) != 1 {
		t.Fatalf("wanted one fallthrough block, got %d", len(blocks))
	}
	block := blocks[0]
	if block.Address != 0x1000 || block.Instructions[len(block.Instructions)-1].Kind != core.OpReturn {
		t.Fatalf("block does not end in return: %+v", block)
	}
	if _, ok := findSymbol(result.IR, core.OpIntrinsic, "x86.endbr64"); !ok {
		t.Fatal("endbr64 was not preserved")
	}
	if _, ok := findInstruction(result.IR, core.OpStore, nil); !ok {
		t.Fatal("push did not store to the stack")
	}
	if _, ok := findInstruction(result.IR, core.OpLoad, nil); !ok {
		t.Fatal("leave did not load the saved frame pointer")
	}
	if _, ok := findInstruction(result.IR, core.OpWriteRegister, func(instruction core.Instruction) bool {
		return len(instruction.Operands) > 0 && instruction.Operands[0].Name == "rsp"
	}); !ok {
		t.Fatal("stack pointer was not written")
	}
	if got := diagnosticCount(result, "decode-error"); got != 0 {
		t.Fatalf("decoded prologue produced %d decode errors", got)
	}
	if got := diagnosticCount(result, "block-falls-off-region"); got != 0 {
		t.Fatalf("terminated block reported a fallthrough: %d", got)
	}
}

func TestAMD64ConditionalBranchBuildsTwoTargets(t *testing.T) {
	code := []byte{
		0x85, 0xc0, // test eax, eax
		0x74, 0x02, // je 0x1006
		0x90, // nop
		0x90, // nop
		0xc3, // ret
	}
	result := parseAMD64At(t, 0x1000, code)
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
	if !addresses[0x1006] || !addresses[0x1004] {
		t.Fatalf("branch targets are wrong: %+v", addresses)
	}
	if _, ok := findSymbol(result.IR, core.OpIntrinsic, "x86.flags.test"); !ok {
		t.Fatal("test did not update flags")
	}
	if _, ok := findSymbol(result.IR, core.OpIntrinsic, "x86.cond.e"); !ok {
		t.Fatal("branch did not read the equality condition")
	}
}

func TestAMD64CallAndJumpTargets(t *testing.T) {
	code := []byte{
		0xe8, 0x05, 0x00, 0x00, 0x00, // call 0x200a
		0x90,       // nop
		0xeb, 0x02, // jmp 0x200a
		0x90, // nop
		0x90, // nop
		0xc3, // ret
	}
	result := parseAMD64At(t, 0x2000, code)
	call, ok := findInstruction(result.IR, core.OpCall, nil)
	if !ok || len(call.Operands) != 1 || call.Operands[0].Kind != core.OperandConstant {
		t.Fatalf("call target is missing: %+v", call)
	}
	if target := literalUint64(t, call.Operands[0].Literal); target != 0x200a {
		t.Fatalf("call target = %#x", target)
	}
	jump, ok := findInstruction(result.IR, core.OpJump, func(instruction core.Instruction) bool {
		return len(instruction.Targets) == 1
	})
	if !ok {
		t.Fatal("direct jump has no block target")
	}
	targetBlock := core.BlockID(0)
	for _, block := range allBlocks(result.IR) {
		if block.ID == jump.Targets[0] {
			targetBlock = block.ID
			if block.Address != 0x200a {
				t.Fatalf("jump target block starts at %#x", block.Address)
			}
		}
	}
	if targetBlock == 0 {
		t.Fatal("jump target block is missing")
	}
}

func TestAMD64RIPRelativeLoadAndLEA(t *testing.T) {
	for _, tc := range []struct {
		name string
		code []byte
		kind core.Opcode
	}{
		{"load", []byte{0x48, 0x8b, 0x05, 0x10, 0x00, 0x00, 0x00, 0xc3}, core.OpLoad},
		{"lea", []byte{0x48, 0x8d, 0x05, 0x10, 0x00, 0x00, 0x00, 0xc3}, core.OpWriteRegister},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := parseAMD64At(t, 0x1000, tc.code)
			instruction, ok := findInstruction(result.IR, tc.kind, nil)
			if !ok {
				t.Fatalf("%s was not lifted", tc.kind)
			}
			if tc.kind == core.OpLoad {
				if len(instruction.Operands) == 0 || instruction.Operands[0].Kind != core.OperandValue {
					t.Fatalf("load has no address operand: %+v", instruction)
				}
				definition, ok := valueDefinition(result.IR, instruction.Operands[0].Value)
				if !ok || definition.Kind != core.OpConstant {
					t.Fatalf("load address is not a constant: %+v", definition)
				}
				if address := literalUint64(t, definition.Operands[0].Literal); address != 0x1017 {
					t.Fatalf("RIP-relative address = %#x", address)
				}
				return
			}
			if len(instruction.Operands) != 2 || instruction.Operands[1].Kind != core.OperandValue {
				t.Fatalf("lea write has no value: %+v", instruction)
			}
			definition, ok := valueDefinition(result.IR, instruction.Operands[1].Value)
			if !ok || definition.Kind != core.OpConstant {
				t.Fatalf("lea address is not a constant: %+v", definition)
			}
			if address := literalUint64(t, definition.Operands[0].Literal); address != 0x1017 {
				t.Fatalf("lea address = %#x", address)
			}
		})
	}
}

func TestAMD64IndexedMemoryStore(t *testing.T) {
	result := parseAMD64At(t, 0x3000, []byte{0x48, 0x89, 0x47, 0x08, 0xc3})
	store, ok := findInstruction(result.IR, core.OpStore, nil)
	if !ok || len(store.Operands) != 2 {
		t.Fatalf("store was not lifted: %+v", store)
	}
	address, ok := valueDefinition(result.IR, store.Operands[0].Value)
	if !ok || address.Kind != core.OpAdd {
		t.Fatalf("store address is not an add: %+v", address)
	}
	if len(address.Operands) != 2 {
		t.Fatalf("store address add has %d operands", len(address.Operands))
	}
	base := resolveCopy(result.IR, address.Operands[0].Value)
	if base.Kind != core.OpReadRegister || base.Operands[0].Name != "rdi" {
		t.Fatalf("store base is not rdi: %+v", base)
	}
	displacement, ok := valueDefinition(result.IR, address.Operands[1].Value)
	if !ok || displacement.Kind != core.OpConstant || literalUint64(t, displacement.Operands[0].Literal) != 8 {
		t.Fatalf("store displacement is not 8: %+v", displacement)
	}
}

func TestAMD64UnsupportedInstructionIsExplicit(t *testing.T) {
	result := parseAMD64At(t, 0x4000, []byte{0x0f, 0x58, 0xc1, 0xc3})
	if _, ok := findSymbol(result.IR, core.OpIntrinsic, "x86.addps"); !ok {
		t.Fatal("unsupported instruction was dropped")
	}
	if diagnosticCount(result, "unsupported-instruction") != 1 {
		t.Fatalf("unexpected diagnostics: %+v", result.Diagnostics)
	}
}

func TestAMD64NewlyLiftedInstructions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		code   []byte
		symbol string
	}{
		{"bt", []byte{0x48, 0x0f, 0xa3, 0xd8, 0xc3}, "x86.flags.bt"},
		{"mul", []byte{0x48, 0xf7, 0xe1, 0xc3}, "x86.mul.lo"},
		{"div", []byte{0x48, 0xf7, 0xf1, 0xc3}, "x86.div.quot"},
		{"idiv", []byte{0x48, 0xf7, 0xf9, 0xc3}, "x86.idiv.rem"},
		{"punpcklqdq", []byte{0x66, 0x0f, 0x6c, 0xc1, 0xc3}, "x86.punpcklqdq"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := parseAMD64At(t, 0xf000, tc.code)
			if diagnosticCount(result, "unsupported-instruction") != 0 {
				t.Fatalf("%s is still unsupported: %+v", tc.name, result.Diagnostics)
			}
			if _, ok := findSymbol(result.IR, core.OpIntrinsic, tc.symbol); !ok {
				t.Fatalf("%s did not emit %s", tc.name, tc.symbol)
			}
		})
	}
}

func TestAMD64DisassembleReportsLiftDiagnostics(t *testing.T) {
	code := []byte{0x0f, 0x58, 0xc1, 0xc3}
	parsed := parseAMD64At(t, 0x4000, code)
	disassembly, err := (Parser{}).Disassemble(context.Background(), amd64Request(t, 0x4000, code))
	if err != nil {
		t.Fatal(err)
	}
	parsedCount := diagnosticCount(parsed, "unsupported-instruction")
	disassembledCount := 0
	for _, diagnostic := range disassembly.Diagnostics {
		if diagnostic.Code == "unsupported-instruction" {
			disassembledCount++
		}
	}
	if disassembledCount != parsedCount || disassembledCount == 0 {
		t.Fatalf("asm and lowir diagnostics differ: asm=%d lowir=%d", disassembledCount, parsedCount)
	}
}

func TestAMD64DecodeErrors(t *testing.T) {
	if _, err := (Parser{}).Parse(context.Background(), amd64Request(t, 0x5000, []byte{0x0f})); err == nil {
		t.Fatal("undecodable region accepted")
	}
	result, err := (Parser{}).Parse(context.Background(), amd64Request(t, 0x5000, []byte{0x90, 0x0f}))
	if err != nil {
		t.Fatalf("partially decodable region rejected: %v", err)
	}
	if diagnosticCount(result, "decode-error") != 1 {
		t.Fatalf("decode error was not reported: %+v", result.Diagnostics)
	}
	if blocks := allBlocks(result.IR); len(blocks) == 0 || blocks[len(blocks)-1].Instructions[len(blocks[len(blocks)-1].Instructions)-1].Kind != core.OpTrap {
		t.Fatal("truncated region did not end in a trap")
	}
}

func TestAMD64BlockFallsOffRegion(t *testing.T) {
	result := parseAMD64At(t, 0x6000, []byte{0x90})
	if diagnosticCount(result, "block-falls-off-region") != 1 {
		t.Fatalf("missing fallthrough diagnostic: %+v", result.Diagnostics)
	}
	blocks := allBlocks(result.IR)
	last := blocks[0].Instructions[len(blocks[0].Instructions)-1]
	if last.Kind != core.OpTrap || len(last.Operands) == 0 || last.Operands[0].Name != "region-end" {
		t.Fatalf("block does not end at a region-end trap: %+v", last)
	}
}

func TestAMD64ResultOwnsBytes(t *testing.T) {
	code := []byte{0x48, 0xb8, 0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11, 0xc3}
	result := parseAMD64At(t, 0x7000, code)
	constant, ok := findInstruction(result.IR, core.OpConstant, func(instruction core.Instruction) bool {
		return len(instruction.Operands) == 1 && len(instruction.Operands[0].Literal) == 8
	})
	if !ok {
		t.Fatal("immediate constant was not lifted")
	}
	before := append([]byte(nil), constant.Operands[0].Literal...)
	code[2] = 0xff
	if !reflect.DeepEqual(before, constant.Operands[0].Literal) {
		t.Fatal("LowIR retained the caller's byte slice")
	}
}

func TestAMD64Deterministic(t *testing.T) {
	code := []byte{0x55, 0x48, 0x89, 0xe5, 0x48, 0x83, 0xec, 0x10, 0xc9, 0xc3}
	first := parseAMD64At(t, 0x8000, code)
	second := parseAMD64At(t, 0x8000, code)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("two parses of the same region differ")
	}
}

func TestAMD64ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (Parser{}).Parse(ctx, amd64Request(t, 0x9000, []byte{0x90}))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %v", err)
	}
}

func TestAMD64ParsesSystemBinary(t *testing.T) {
	path := "/bin/true"
	file, err := elf.Open(path)
	if err != nil {
		t.Skipf("%s unavailable: %v", path, err)
	}
	defer file.Close()
	if file.Machine != elf.EM_X86_64 {
		t.Skipf("%s is not x86-64", path)
	}
	section := file.Section(".text")
	if section == nil {
		t.Skipf("%s has no .text", path)
	}
	code, err := section.Data()
	if err != nil {
		t.Fatal(err)
	}
	result := parseAMD64At(t, section.Addr, code)
	if blocks := allBlocks(result.IR); len(blocks) < 10 {
		t.Fatalf("only %d blocks recovered", len(blocks))
	}
	if len(allInstructions(result.IR)) < 100 {
		t.Fatalf("only %d instructions recovered", len(allInstructions(result.IR)))
	}
	if diagnosticCount(result, "decode-error") != 0 {
		t.Fatalf("real binary produced decode errors: %+v", result.Diagnostics)
	}
	validateAMD64IR(t, result.IR)
}

func TestAMD64VectorMovesAndBitwiseOps(t *testing.T) {
	code := []byte{
		0x0f, 0x28, 0xc1, // movaps xmm0, xmm1
		0x66, 0x0f, 0xef, 0xc1, // pxor xmm0, xmm1
		0xc3,
	}
	result := parseAMD64At(t, 0xa000, code)
	if diagnosticCount(result, "unsupported-instruction") != 0 {
		t.Fatalf("vector instructions were not lowered: %+v", result.Diagnostics)
	}
	write, ok := findInstruction(result.IR, core.OpWriteRegister, func(instruction core.Instruction) bool {
		if len(instruction.Operands) < 2 || instruction.Operands[0].Name != "xmm0" || instruction.Operands[1].Kind != core.OperandValue {
			return false
		}
		definition, ok := valueDefinition(result.IR, instruction.Operands[1].Value)
		return ok && definition.Kind == core.OpXor
	})
	if !ok {
		t.Fatal("xmm0 xor was not lowered")
	}
	definition, _ := valueDefinition(result.IR, write.Operands[1].Value)
	if definition.Results[0].Type.Bits != 128 {
		t.Fatalf("vector xor has %d bits", definition.Results[0].Type.Bits)
	}
}

func validateAMD64IR(t *testing.T, ir *core.LowIR) {
	t.Helper()
	if len(ir.Functions) == 0 {
		t.Fatal("LowIR has no functions")
	}
	functions := map[core.FunctionID]bool{}
	for _, function := range ir.Functions {
		if function.ID == 0 || functions[function.ID] {
			t.Fatalf("duplicate or zero function id %d", function.ID)
		}
		functions[function.ID] = true
		blocks := map[core.BlockID]bool{}
		for _, block := range function.Blocks {
			if block.ID == 0 || blocks[block.ID] {
				t.Fatalf("duplicate or zero block id %d", block.ID)
			}
			blocks[block.ID] = true
			if len(block.Instructions) == 0 {
				t.Fatalf("empty block %d", block.ID)
			}
			if !amd64ControlTransfer(block.Instructions[len(block.Instructions)-1].Kind) {
				t.Fatalf("block %d does not end in a control transfer", block.ID)
			}
		}
		values := map[core.ValueID]core.ValueType{}
		for _, block := range function.Blocks {
			for _, instruction := range block.Instructions {
				for _, operand := range instruction.Operands {
					if operand.Kind != core.OperandValue {
						continue
					}
					typ, ok := values[operand.Value]
					if !ok {
						t.Fatalf("operand %d is used before definition at %#x", operand.Value, instruction.Address)
					}
					if typ != operand.Type {
						t.Fatalf("operand %d type = %+v, defined as %+v", operand.Value, operand.Type, typ)
					}
				}
				for _, result := range instruction.Results {
					if result.ID == 0 {
						t.Fatalf("zero value id at %#x", instruction.Address)
					}
					if _, ok := values[result.ID]; ok {
						t.Fatalf("value %d is defined twice", result.ID)
					}
					values[result.ID] = result.Type
				}
				for _, target := range instruction.Targets {
					if !blocks[target] {
						t.Fatalf("instruction at %#x targets missing block %d", instruction.Address, target)
					}
				}
			}
		}
	}
}

func TestAMD64LowIRInvariants(t *testing.T) {
	code := []byte{
		0xf3, 0x0f, 0x1e, 0xfa,
		0x55,
		0x48, 0x89, 0xe5,
		0x48, 0x83, 0xec, 0x20,
		0x48, 0x8b, 0x05, 0x10, 0x00, 0x00, 0x00,
		0x48, 0x89, 0x45, 0xf8,
		0x85, 0xc0,
		0x74, 0x06,
		0x48, 0x83, 0xc4, 0x20,
		0x5d,
		0xc3,
		0x48, 0x83, 0xc4, 0x20,
		0x5d,
		0xc3,
	}
	result := parseAMD64At(t, 0xb000, code)
	validateAMD64IR(t, result.IR)
}

func TestAMD64ThreeOperandIMUL(t *testing.T) {
	result := parseAMD64At(t, 0xc000, []byte{0x48, 0x6b, 0xc0, 0x03, 0xc3})
	multiply, ok := findInstruction(result.IR, core.OpMul, nil)
	if !ok {
		t.Fatal("imul was not lifted")
	}
	if len(multiply.Operands) != 2 {
		t.Fatalf("imul has %d operands", len(multiply.Operands))
	}
	left, ok := valueDefinition(result.IR, multiply.Operands[0].Value)
	if !ok || left.Kind != core.OpReadRegister || left.Operands[0].Name != "rax" {
		t.Fatalf("imul left operand is not rax: %+v", left)
	}
	right, ok := valueDefinition(result.IR, multiply.Operands[1].Value)
	if !ok || right.Kind != core.OpConstant || literalUint64(t, right.Operands[0].Literal) != 3 {
		t.Fatalf("imul multiplier is not 3: %+v", right)
	}
}

func TestAMD64SETccAndCMOVcc(t *testing.T) {
	code := []byte{
		0x0f, 0x95, 0xc0, // setne al
		0x48, 0x0f, 0x44, 0xc3, // cmove rax, rbx
		0xc3,
	}
	result := parseAMD64At(t, 0xd000, code)
	if _, ok := findSymbol(result.IR, core.OpIntrinsic, "x86.cond.ne"); !ok {
		t.Fatal("setne did not read the ne condition")
	}
	if _, ok := findInstruction(result.IR, core.OpSelect, nil); !ok {
		t.Fatal("cmove was not lowered to select")
	}
}

func TestAMD64WidthExtensions(t *testing.T) {
	for _, tc := range []struct {
		name string
		code []byte
		kind core.Opcode
		bits uint16
	}{
		{"movzx", []byte{0x0f, 0xb6, 0xc0, 0xc3}, core.OpZeroExtend, 32},
		{"movsxd", []byte{0x48, 0x63, 0xc0, 0xc3}, core.OpSignExtend, 64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := parseAMD64At(t, 0xe000, tc.code)
			extension, ok := findInstruction(result.IR, tc.kind, nil)
			if !ok || extension.Results[0].Type.Bits != tc.bits {
				t.Fatalf("%s not lowered to %s: %+v", tc.name, tc.kind, extension)
			}
		})
	}
}

func TestAMD64BranchTargetDiagnostics(t *testing.T) {
	outside := parseAMD64At(t, 0xf000, []byte{0xeb, 0x10, 0xc3})
	if diagnosticCount(outside, "branch-target-outside-region") != 1 {
		t.Fatalf("outside target not reported: %+v", outside.Diagnostics)
	}
	misaligned := parseAMD64At(t, 0xf100, []byte{0xeb, 0x01, 0x48, 0x89, 0xe5, 0xc3})
	if diagnosticCount(misaligned, "branch-target-misaligned") != 1 {
		t.Fatalf("misaligned target not reported: %+v", misaligned.Diagnostics)
	}
}

func TestAMD64LockPrefixIsReported(t *testing.T) {
	result := parseAMD64At(t, 0x10000, []byte{0xf0, 0x48, 0x01, 0x07, 0xc3})
	if diagnosticCount(result, "atomic-prefix-unmodeled") != 1 {
		t.Fatalf("lock prefix not reported: %+v", result.Diagnostics)
	}
	if _, ok := findInstruction(result.IR, core.OpAdd, nil); !ok {
		t.Fatal("locked add was dropped")
	}
}

func TestAMD64UnaryAndNop(t *testing.T) {
	code := []byte{
		0x48, 0xf7, 0xd8, // neg rax
		0x48, 0xf7, 0xd0, // not rax
		0x90, // nop
		0xc3,
	}
	result := parseAMD64At(t, 0x11000, code)
	for _, kind := range []core.Opcode{core.OpNeg, core.OpNot, core.OpNop} {
		if _, ok := findInstruction(result.IR, kind, nil); !ok {
			t.Fatalf("%s was not lifted", kind)
		}
	}
}
