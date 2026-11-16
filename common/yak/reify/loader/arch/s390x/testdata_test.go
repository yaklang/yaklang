package s390x

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/loader/arch/internal/archtest"
	"testing"
)

// fmtSscanf parses one two-digit hex byte.
func fmtSscanf(text string, value *byte) (int, error) {
	return fmt.Sscanf(text, "%02x", value)
}

// targetS390X is the target of one s390x code region.
func targetS390X() core.Target {
	return core.Target{
		OS:           core.OSLinux,
		Architecture: core.ArchS390X,
		Bits:         64,
		ByteOrder:    core.BigEndian,
	}
}

// regionBytes turns a hex string into a byte slice.
func regionBytes(t *testing.T, hex string) []byte {
	t.Helper()
	result := make([]byte, 0, len(hex)/2)
	for index := 0; index+2 <= len(hex); index += 2 {
		var value byte
		if _, err := fmtSscanf(hex[index:index+2], &value); err != nil {
			t.Fatalf("bad hex %q: %v", hex, err)
		}
		result = append(result, value)
	}
	return result
}

// parseAt decodes and lifts one region.
func parseAt(t *testing.T, address uint64, code []byte) *core.ParseResult {
	t.Helper()
	reader, err := core.NewByteReader("test", bytes.NewReader(code), int64(len(code)))
	if err != nil {
		t.Fatal(err)
	}
	request := core.ParseRequest{Target: targetS390X(), Address: address, Reader: reader, WithAssembly: true}
	result, err := parse(context.Background(), request)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return result
}

// TestDecodeLengths checks the two, four and six byte encodings against the
// listing llvm-objdump prints for the same bytes.
func TestDecodeLengths(t *testing.T) {
	cases := []struct {
		hex    string
		length int
		name   string
	}{
		{"07fe", 2, "bcr"},
		{"0de1", 2, "basr"},
		{"a7fb0018", 4, "aghi"},
		{"a7280001", 4, "lhi"},
		{"92016018", 4, "mvi"},
		{"c0e500045122", 6, "brasl"},
		{"c020000b0e7c", 6, "larl"},
		{"e3a0d0100004", 6, "lg"},
		{"e3e0ffe8ff24", 6, "stg"},
		{"b90400b1", 4, "lgr"},
		{"eb020030000a", 6, "srag"},
		{"d20910001000", 6, "mvc"},
	}
	for _, testCase := range cases {
		t.Run(testCase.hex, func(t *testing.T) {
			code := regionBytes(t, testCase.hex)
			reader, err := core.NewByteReader("test", bytes.NewReader(code), int64(len(code)))
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := decodeInstructions(context.Background(),
				core.ParseRequest{Target: targetS390X(), Address: 0x1000, Reader: reader}, &diagnostics{})
			if err != nil {
				t.Fatal(err)
			}
			if len(decoded) == 0 || decoded[0].undefined {
				t.Fatalf("not decoded")
			}
			if decoded[0].size != testCase.length {
				t.Fatalf("size = %d, want %d", decoded[0].size, testCase.length)
			}
			if got := nameOf(decoded[0].inst.Op); got != testCase.name {
				t.Fatalf("op = %q, want %q", got, testCase.name)
			}
		})
	}
}

// TestBranchTargets checks the PC-relative branch arithmetic. The immediate is a
// halfword count from the instruction address.
func TestBranchTargets(t *testing.T) {
	cases := []struct {
		hex    string
		offset int64
	}{
		// brasl %r14, .+0
		{"c0e500000000", 0},
		// brasl %r14, .+0x8 : imm 4 -> 8 bytes
		{"c0e500000004", 8},
		// aghi: no branch target
	}
	for _, testCase := range cases {
		code := regionBytes(t, testCase.hex)
		reader, _ := core.NewByteReader("test", bytes.NewReader(code), int64(len(code)))
		decoded, err := decodeInstructions(context.Background(),
			core.ParseRequest{Target: targetS390X(), Address: 0x1000, Reader: reader}, &diagnostics{})
		if err != nil {
			t.Fatal(err)
		}
		target, ok := branchTarget(decoded[0].inst, 0x1000, decoded[0].size)
		if !ok {
			t.Fatalf("%s: no target", testCase.hex)
		}
		if target != 0x1000+uint64(testCase.offset) {
			t.Fatalf("%s: target = %#x, want %#x", testCase.hex, target, 0x1000+uint64(testCase.offset))
		}
	}
}

// TestLiftImmediateForms checks that the register-and-immediate forms write their
// destination and that the memory forms read or write memory.
func TestLiftImmediateForms(t *testing.T) {
	// aghi %r2, 0x18; lghi %r3, 1; br %r14
	code := regionBytes(t, "a7fb0018"+"a7380001"+"07fe")
	result := parseAt(t, 0x1000, code)
	functions := result.IR.Functions
	if len(functions) != 1 {
		t.Fatalf("functions = %d", len(functions))
	}
	var adds, writes, returns int
	for _, block := range functions[0].Blocks {
		for _, instruction := range block.Instructions {
			switch instruction.Kind {
			case core.OpAdd:
				adds++
			case core.OpWriteRegister:
				writes++
			case core.OpReturn:
				returns++
			}
		}
	}
	if adds == 0 {
		t.Fatal("no add operation")
	}
	if writes == 0 {
		t.Fatal("no register write")
	}
	if returns != 1 {
		t.Fatalf("returns = %d, want 1", returns)
	}
}

// TestLiftMemoryForms checks that a store writes memory and a load reads it.
func TestLiftMemoryForms(t *testing.T) {
	// lg %r10, 16(%r13); stg %r14, -24(%r15); br %r14
	code := regionBytes(t, "e3a0d0100004"+"e3e0ffe8ff24"+"07fe")
	result := parseAt(t, 0x1000, code)
	var loads, stores int
	for _, block := range result.IR.Functions[0].Blocks {
		for _, instruction := range block.Instructions {
			switch instruction.Kind {
			case core.OpLoad:
				loads++
			case core.OpStore:
				stores++
			}
		}
	}
	if loads != 1 {
		t.Fatalf("loads = %d, want 1", loads)
	}
	if stores != 1 {
		t.Fatalf("stores = %d, want 1", stores)
	}
}

// TestNoUnsupportedOnRealEncodings checks the lift covers the forms the sample
// binary uses, so no instruction falls back to the generic intrinsic.
func TestNoUnsupportedOnRealEncodings(t *testing.T) {
	// lgr; aghi; nilf; mvghi; mvc; brasl; brc; br
	code := regionBytes(t, "b90400b1"+"a7fb0018"+"c00b00000001"+"e5481000000a"+"d20910001000"+"c0e500045122"+"a7440002"+"07fe")
	reader, err := core.NewByteReader("test", bytes.NewReader(code), int64(len(code)))
	if err != nil {
		t.Fatal(err)
	}
	request := core.ParseRequest{Target: targetS390X(), Address: 0x1000, Reader: reader}
	result, err := parse(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "unsupported-instruction" {
			t.Fatalf("unsupported: %s", diagnostic.Message)
		}
	}
}

// TestUnknownWordBecomesData checks the Op(0) rendering: a word the decoder
// cannot name is data inside the code region and becomes a trap.
func TestUnknownWordBecomesData(t *testing.T) {
	// 0x0000 is not a valid s390x instruction.
	code := regionBytes(t, "0000"+"07fe")
	result := parseAt(t, 0x1000, code)
	var traps int
	for _, block := range result.IR.Functions[0].Blocks {
		for _, instruction := range block.Instructions {
			if instruction.Kind == core.OpTrap {
				traps++
			}
		}
	}
	if traps == 0 {
		t.Fatal("no trap for the unmapped word")
	}
}

// TestCheckRejectsWrongByteOrder checks the target validation.
func TestCheckRejectsWrongByteOrder(t *testing.T) {

	if err := checkTarget(targetS390X()); err != nil {
		t.Fatalf("valid target rejected: %v", err)
	}
	wrong := targetS390X()
	wrong.ByteOrder = core.LittleEndian
	if err := checkTarget(wrong); err == nil {
		t.Fatal("little-endian target accepted")
	}
	wrong = targetS390X()
	wrong.Bits = 32
	if err := checkTarget(wrong); err == nil {
		t.Fatal("32-bit target accepted")
	}
	wrong = targetS390X()
	wrong.Mode = "thumb"
	if err := checkTarget(wrong); err == nil {
		t.Fatal("execution mode accepted")
	}
}

//go:embed testdata
var testdata embed.FS

// TestFixtures runs every case in testdata.
func TestFixtures(t *testing.T) {
	archtest.Run(t, archtest.Config{
		FS:     testdata,
		Dir:    "testdata",
		Parser: Parser{},
		Target: core.Target{OS: core.OSLinux, Architecture: core.ArchS390X, Bits: 64, ByteOrder: core.BigEndian},
	})
}
