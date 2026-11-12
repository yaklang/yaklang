package arm64

import (
	"fmt"
	"strings"

	"golang.org/x/arch/arm64/arm64asm"
)

// golang.org/x/arch does not decode the ARMv8.1 LSE atomics, and the Go runtime
// emits them for sync/atomic on every modern core. This file decodes the
// load/store class the runtime uses, so one word does not cost a whole function.
//
// Encoding (ARM ARM C6.2, LDADD/LDCLR/LDEOR/LDSET/LDSMAX/LDSMIN/LDUMAX/LDUMIN/
// SWP): size 111000 A R 0 Rs opc 00 Rn Rt
//
//	size  = operand width: 00 byte, 01 halfword, 10 word, 11 doubleword
//	A     = acquire, R = release
//	opc   = 0000 LDADD, 0001 LDCLR, 0010 LDEOR, 0011 LDSET,
//	        0100 LDSMAX, 0101 LDSMIN, 0110 LDUMAX, 0111 LDUMIN, 1000 SWP
//
// The CAS family occupies a different opcode row:
//
//	size 001000 A R 1 Rs o0 11111 Rn Rt
//
// where o0 selects CAS (o0 = 0) from CASP (o0 = 1). CAS compares Rt with
// memory and swaps in Rs on equality, so Rd and Rs share bits 20:16.
// The load/store class is bits 29:24 = 111000 with bit 21 fixed to one and
// bits 11:10 fixed to zero; without those two bits a plain STR such as
// 0xf8004423 would match.
//
//	CAS uses bits 29:24 = 001000 with bits 15:10 = 111111.
const (
	lseClassMask  = 0x3f200c00
	lseClassValue = 0x38200000

	casClassMask  = 0x3f200c00
	casClassValue = 0x08200c00
)

var lseOperations = [...]string{
	0: "ldadd",
	1: "ldclr",
	2: "ldeor",
	3: "ldset",
	4: "ldsmax",
	5: "ldsmin",
	6: "ldumax",
	7: "ldumin",
	8: "swp",
}

// lseInstruction returns a synthetic instruction for one LSE load/store atomic,
// or false when the word belongs to another class.
func lseInstruction(raw uint32, address uint64) (instruction, bool) {
	if raw&casClassMask == casClassValue {
		return casInstruction(raw, address)
	}
	if raw&lseClassMask != lseClassValue {
		return instruction{}, false
	}
	opcode := (raw >> 12) & 0xf
	if int(opcode) >= len(lseOperations) {
		return instruction{}, false
	}
	name := lseOperations[opcode]
	acquire := raw&0x00800000 != 0
	release := raw&0x00400000 != 0
	switch {
	case acquire && release:
		name += "al"
	case acquire:
		name += "a"
	case release:
		name += "l"
	}
	switch (raw >> 30) & 0x3 {
	case 0:
		name += "b"
	case 1:
		name += "h"
	}
	// LDADD <Ws>, <Wt>, [<Xn|SP>]: Rs (20:16) is the value folded into memory,
	// Rt (4:0) receives the previous memory value.
	sizeBits := (raw >> 30) & 0x3
	source := sizedRegister(uint8((raw>>16)&0x1f), sizeBits)
	result := sizedRegister(uint8(raw&0x1f), sizeBits)
	base := registerText(uint8((raw >> 5) & 0x1f))
	text := fmt.Sprintf("%s %s, %s, [%s]", name, source, result, base)
	return instruction{
		address: address,
		raw:     raw,
		inst:    arm64asm.Inst{Op: arm64asm.NOP},
		size:    4,
		text:    text,
		lse:     true,
	}, true
}

// casInstruction decodes CAS, CASA, CASL, CASAL and their byte and halfword
// forms. CASP needs a register pair and is left as a decode error.
func casInstruction(raw uint32, address uint64) (instruction, bool) {
	// Bits 15:10 are 111111 in every CAS form; the CASP pair form uses the
	// same row with bit 15 set and is not decoded here.
	if raw&0x0000fc00 != 0x0000fc00 {
		return instruction{}, false
	}
	name := "cas"
	acquire := raw&0x00800000 != 0
	release := raw&0x00400000 != 0
	switch {
	case acquire && release:
		name += "al"
	case acquire:
		name += "a"
	case release:
		name += "l"
	}
	switch (raw >> 30) & 0x3 {
	case 0:
		name += "b"
	case 1:
		name += "h"
	}
	// CAS <Ws>, <Wt>, [<Xn|SP>]: Rs (20:16) holds the compare value and Rt
	// (4:0) the value written on equality.
	size := (raw >> 30) & 0x3
	expected := sizedRegister(uint8((raw>>16)&0x1f), size)
	desired := sizedRegister(uint8(raw&0x1f), size)
	base := registerText(uint8((raw >> 5) & 0x1f))
	text := fmt.Sprintf("%s %s, %s, [%s]", name, expected, desired, base)
	return instruction{
		address: address,
		raw:     raw,
		inst:    arm64asm.Inst{Op: arm64asm.NOP},
		size:    4,
		text:    text,
		lse:     true,
	}, true
}

// sizedRegister renders a register in the width the instruction operates on:
// byte and halfword forms keep the X name, word forms use W, doubleword X.
func sizedRegister(index uint8, size uint32) string {
	switch size {
	case 2:
		if index == 31 {
			return "wzr"
		}
		return fmt.Sprintf("w%d", int(index))
	case 3:
		return registerText(index)
	default:
		return registerText(index)
	}
}

// lseSymbol is the intrinsic name of one LSE atomic. The acquire/release
// suffix and the width suffix are kept so the effect stays visible.
func lseSymbol(text string) string {
	name := text
	if index := strings.IndexByte(name, ' '); index > 0 {
		name = name[:index]
	}
	return "arm64." + name
}
