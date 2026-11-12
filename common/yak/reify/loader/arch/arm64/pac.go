package arm64

import (
	"fmt"
	"strings"

	"golang.org/x/arch/arm64/arm64asm"
)

// golang.org/x/arch does not decode pointer-authentication instructions, and
// arm64e macOS binaries use them in every prologue and epilogue. This parser
// decodes the encodings directly, so a signing hint is not reported as a decode
// error and an authenticating return still ends a block.
//
// The field layout follows the ARM ARM:
//
//	PACIA  Xd, Xn  1101 0101 0000 0001 0000 0000 000 Rn Rd
//	PACIZA Xd      1101 0101 0000 0001 0010 0000 000 Rd
//	RETAA          1101 0110 0101 1111 0000 1011 1111 1111
//	BRAA   Xn, Xm  1101 0111 0001 1111 0000 1000 000 Rn Rm
const (
	pacKeyMask  = 0xfffffc00
	pacKeyData  = 0xdac10000
	pacKeyShift = 5
)

var pacForms = []struct {
	value uint32
	name  string
}{
	{0xdac10000, "pacia"},
	{0xdac10400, "pacib"},
	{0xdac10800, "autia"},
	{0xdac10c00, "autib"},
	{0xdac12000, "paciza"},
	{0xdac12400, "pacizb"},
	{0xdac12800, "autiza"},
	{0xdac12c00, "autizb"},
}

// pacInstruction returns the synthetic instruction for one
// pointer-authentication encoding, or false when the word belongs to another
// class. Both the decoder and the boundary scanner call it, so a function that
// ends in RETAA is not cut short.
func pacInstruction(raw uint32) (arm64asm.Inst, bool) {
	for _, form := range pacForms {
		if raw&pacKeyMask == form.value {
			return arm64asm.Inst{Op: arm64asm.NOP}, true
		}
	}
	switch raw & 0xfffff800 {
	case 0xd65f0800:
		return arm64asm.Inst{Op: arm64asm.RET, Args: [5]arm64asm.Arg{arm64asm.X30}}, true
	case 0xd71f0800, 0xd73f0800, 0xd61f0800, 0xd63f0800:
		branch := raw&0x300000 != 0
		index := (raw >> pacKeyShift) & 0x1f
		if branch {
			index = raw & 0x1f
		}
		kind := arm64asm.BR
		if branch {
			kind = arm64asm.BLR
		}
		return arm64asm.Inst{Op: kind, Args: [5]arm64asm.Arg{arm64Register(index)}}, true
	}
	return arm64asm.Inst{}, false
}

// decodePAC returns a synthetic instruction for one pointer-authentication
// encoding, or false when the word belongs to another class.
func decodePAC(raw uint32, address uint64) (instruction, bool) {
	for _, form := range pacForms {
		if raw&pacKeyMask != form.value {
			continue
		}
		destination := registerText(uint8(raw & 0x1f))
		text := form.name + " " + destination
		if raw&0x2000 == 0 {
			text += ", " + registerText(uint8((raw>>pacKeyShift)&0x1f))
		}
		// Signing and authenticating a pointer changes no memory or register
		// value the decompiler models; keep it as a no-op with the mnemonic.
		return instruction{address: address, raw: raw, inst: arm64asm.Inst{Op: arm64asm.NOP}, size: 4, text: text}, true
	}
	switch raw & 0xfffff800 {
	case 0xd65f0800:
		// RETAA (bit 10 clear) and RETAB (bit 10 set).
		text := "retaa"
		if raw&0x400 != 0 {
			text = "retab"
		}
		return instruction{address: address, raw: raw, inst: arm64asm.Inst{Op: arm64asm.RET, Args: [5]arm64asm.Arg{arm64asm.X30}}, size: 4, text: text}, true
	case 0xd71f0800, 0xd73f0800, 0xd61f0800, 0xd63f0800:
		// BRAA/BRAB and BLRAA/BLRAB branch through a keyed pointer. The
		// trailing Z forms authenticate with the implicit modifier only, so
		// the register number in Rn is the target.
		keyed := raw&0x200000 != 0
		branch := raw&0x300000 != 0
		index := (raw >> pacKeyShift) & 0x1f
		if branch {
			index = raw & 0x1f
		}
		name := "braa"
		kind := arm64asm.BR
		if branch {
			name = "blraa"
			kind = arm64asm.BLR
		}
		if raw&0x400 != 0 {
			name = strings.TrimSuffix(name, "a") + "ab"
		}
		text := name
		if !keyed {
			text += "z"
		}
		text += " " + registerText(uint8(index))
		return instruction{
			address: address,
			raw:     raw,
			inst:    arm64asm.Inst{Op: kind, Args: [5]arm64asm.Arg{arm64Register(index)}},
			size:    4,
			text:    text,
		}, true
	}
	return instruction{}, false
}

// registerText renders a five-bit register field as x0..x30 or xzr.
func registerText(index uint8) string {
	if index == 31 {
		return "xzr"
	}
	return fmt.Sprintf("x%d", int(index))
}

func arm64Register(index uint32) arm64asm.Reg {
	if index == 31 {
		return arm64asm.XZR
	}
	return arm64asm.X0 + arm64asm.Reg(index)
}
