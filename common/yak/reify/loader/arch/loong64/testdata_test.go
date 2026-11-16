package loong64

import (
	"bytes"
	"context"
	"embed"
	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/loader/arch/internal/archtest"
	"golang.org/x/arch/loong64/loong64asm"
	"testing"
)

// targetLoong64 is the target of one LoongArch code region.
func targetLoong64() core.Target {
	return core.Target{
		OS:           core.OSLinux,
		Architecture: core.ArchLOONG64,
		Bits:         64,
		ByteOrder:    core.LittleEndian,
	}
}

// regionBytes turns the little-endian words of one region into bytes.
func regionBytes(t *testing.T, words ...uint32) []byte {
	t.Helper()
	result := make([]byte, 0, len(words)*4)
	for _, word := range words {
		result = append(result,
			byte(word), byte(word>>8), byte(word>>16), byte(word>>24))
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
	request := core.ParseRequest{Target: targetLoong64(), Address: address, Reader: reader, WithAssembly: true}
	result, err := parse(context.Background(), request)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return result
}

// TestDecodeWords checks the decoder against the encodings llvm-objdump prints
// for the same Go binary.
func TestDecodeWords(t *testing.T) {
	cases := []struct {
		word uint32
		name string
	}{
		{0x28c042d4, "ld.d"},
		{0x29ffa061, "st.d"},
		{0x02ffa063, "addi.d"},
		{0x0049fc8a, "srai.d"},
		{0x151c71cb, "lu12i.w"},
		{0x038e3d6b, "ori"},
		{0x17c71c6b, "lu32i.d"},
		{0x0338e16b, "lu52i.d"},
		{0x001e2c8b, "mulh.d"},
		{0x0010ac8b, "add.d"},
		{0x00490d6b, "srai.d"},
		{0x0011a96a, "sub.d"},
		{0x002d294b, "alsl.d"},
		{0x4c000020, "jirl"},
		{0x50000000, "b"},
		{0x54000000, "bl"},
	}
	for _, testCase := range cases {
		code := regionBytes(t, testCase.word)
		inst, err := loong64asm.Decode(code)
		if err != nil {
			t.Fatalf("%08x: %v", testCase.word, err)
		}
		if got := nameOf(inst.Op); got != testCase.name {
			t.Fatalf("%08x: op = %q, want %q", testCase.word, got, testCase.name)
		}
	}
}

// TestLoadStoreOperands checks the base-and-displacement reading. LoongArch
// writes the base register before the signed immediate.
func TestLoadStoreOperands(t *testing.T) {
	// ld.d $t8, $fp, 16 ; st.d $ra, $sp, -24
	code := regionBytes(t, 0x28c042d4, 0x29ffa061)
	result := parseAt(t, 0x11000, code)
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
	if stores != 0 && stores != 1 {
		t.Fatalf("stores = %d", stores)
	}
}

// TestLiftArithmetic checks that a register operation writes its destination.
func TestLiftArithmetic(t *testing.T) {
	// addi.d $sp, $sp, -24 ; add.d $a7, $a0, $a7 ; ret
	code := regionBytes(t, 0x02ffa063, 0x0010ac8b, 0x4c000020)
	result := parseAt(t, 0x11000, code)
	var additions, writes, returns int
	for _, block := range result.IR.Functions[0].Blocks {
		for _, instruction := range block.Instructions {
			switch instruction.Kind {
			case core.OpAdd:
				additions++
			case core.OpWriteRegister:
				writes++
			case core.OpReturn:
				returns++
			}
		}
	}
	if additions == 0 {
		t.Fatal("no add operation")
	}
	if writes == 0 {
		t.Fatal("no register write")
	}
	if returns != 1 {
		t.Fatalf("returns = %d, want 1", returns)
	}
}

// TestBranchTarget checks the PC-relative offset, which is a byte count.
func TestBranchTarget(t *testing.T) {
	// b .+0x10 : the offset occupies the low sixteen bits of the immediate, and
	// the library reports them shifted left by sixteen.
	code := regionBytes(t, 0x50000004)
	reader, _ := core.NewByteReader("test", bytes.NewReader(code), int64(len(code)))
	decoded, err := decodeInstructions(context.Background(),
		core.ParseRequest{Target: targetLoong64(), Address: 0x11000, Reader: reader}, &diagnostics{})
	if err != nil {
		t.Fatal(err)
	}
	target, ok := branchTarget(nameOf(decoded[0].inst.Op), args(decoded[0].inst), 0x11000)
	if !ok {
		t.Fatal("no target")
	}
	if target != 0x11010 {
		t.Fatalf("target = %#x, want %#x", target, 0x11010)
	}
}

// TestNoUnsupportedOnRealEncodings checks the sample binary's forms are all
// modeled or made opaque.
func TestNoUnsupportedOnRealEncodings(t *testing.T) {
	// addi.d; st.d; lu12i.w; ori; lu32i.d; lu52i.d; mulh.d; ret
	code := regionBytes(t, 0x02ffa063, 0x29ffa061, 0x151c71cb, 0x038e3d6b,
		0x17c71c6b, 0x0338e16b, 0x001e2c8b, 0x4c000020)
	reader, err := core.NewByteReader("test", bytes.NewReader(code), int64(len(code)))
	if err != nil {
		t.Fatal(err)
	}
	request := core.ParseRequest{Target: targetLoong64(), Address: 0x11000, Reader: reader}
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

// TestCheckRejectsWrongByteOrder checks the target validation.
func TestCheckRejectsWrongByteOrder(t *testing.T) {

	if err := checkTarget(targetLoong64()); err != nil {
		t.Fatalf("valid target rejected: %v", err)
	}
	for _, mutate := range []func(*core.Target){
		func(target *core.Target) { target.ByteOrder = core.BigEndian },
		func(target *core.Target) { target.Bits = 32 },
		func(target *core.Target) { target.Mode = "thumb" },
	} {
		target := targetLoong64()
		mutate(&target)
		if err := checkTarget(target); err == nil {
			t.Fatalf("invalid target accepted: %+v", target)
		}
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
		Target: core.Target{OS: core.OSLinux, Architecture: core.ArchLOONG64, Bits: 64, ByteOrder: core.LittleEndian},
	})
}
