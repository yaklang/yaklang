package x86_16

import (
	"bytes"
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// target8086 is the target of one real-mode code region.
func target8086() core.Target {
	return core.Target{
		OS:           core.OSDOS,
		Architecture: core.Arch8086,
		Bits:         16,
		ByteOrder:    core.LittleEndian,
	}
}

// TestDecodeRealModeForms checks the sixteen-bit encodings that real-mode code
// uses: the accumulator moves, the interrupt call and the arithmetic forms.
func TestDecodeRealModeForms(t *testing.T) {
	// mov ax, 0x4c00 ; int 0x21 ; add ax, bx ; ret
	code := []byte{0xb8, 0x00, 0x4c, 0xcd, 0x21, 0x01, 0xd8, 0xc3}
	reader, err := core.NewByteReader("test", bytes.NewReader(code), int64(len(code)))
	if err != nil {
		t.Fatal(err)
	}
	request := core.ParseRequest{Target: target8086(), Address: 0x100, Reader: reader}
	result, err := (Parser{}).Parse(context.Background(), request)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.IR.Functions) != 1 {
		t.Fatalf("functions = %d", len(result.IR.Functions))
	}
	var moves, traps int
	for _, block := range result.IR.Functions[0].Blocks {
		for _, instruction := range block.Instructions {
			switch instruction.Kind {
			case core.OpWriteRegister:
				moves++
			case core.OpTrap:
				traps++
			}
		}
	}
	if moves == 0 {
		t.Fatal("no register write")
	}
	if traps != 1 {
		t.Fatalf("traps = %d, want 1", traps)
	}
}

// TestSixteenBitRegisters checks the register file: the profile maps the
// accumulator and the stack pointer to their sixteen-bit names.
func TestSixteenBitRegisters(t *testing.T) {
	profile := profile()
	if profile.Bits != 16 || profile.Mode != 16 {
		t.Fatalf("profile = %+v", profile)
	}
	if profile.StackPointer != "sp" || profile.StackFrame != "bp" {
		t.Fatalf("stack registers = %q %q", profile.StackPointer, profile.StackFrame)
	}
	if profile.Accumulator != "ax" || profile.CountRegister != "cx" {
		t.Fatalf("accumulator = %q count = %q", profile.Accumulator, profile.CountRegister)
	}
}

// TestCheckRejectsWrongTarget checks the target validation.
func TestCheckRejectsWrongTarget(t *testing.T) {
	parser := Parser{}
	if err := parser.Check(target8086()); err != nil {
		t.Fatalf("valid target rejected: %v", err)
	}
	wrong := target8086()
	wrong.Bits = 32
	if err := parser.Check(wrong); err == nil {
		t.Fatal("32-bit target accepted")
	}
	wrong = target8086()
	wrong.Architecture = core.Arch386
	if err := parser.Check(wrong); err == nil {
		t.Fatal("386 target accepted")
	}
}
