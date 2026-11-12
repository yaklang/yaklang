package arm64

import (
	"fmt"

	"golang.org/x/arch/arm64/arm64asm"
)

// golang.org/x/arch decodes some MSR-immediate forms but not the newer
// PSTATE fields (DIT, UAO, PAN, SSBS, TCO). The Go runtime uses DIT on
// arm64 machines with pointer authentication, so a missing decode would cost a
// whole function.
//
// Encoding (ARM ARM C6.2, MSR <pstatefield>, #<imm>):
//
//	1101 0101 0000 00 op1 0100 CRm op2 11111
//
// op1:op2:CRm select the field; the immediate occupies CRm bits 11:8.
// pstateFields maps op1:op2 to the field name. op2 is bits 7:5 and op1 bits
// 18:16; the values below were taken from llvm-mc encodings.
var pstateFields = map[[2]uint32]string{
	{0, 5}: "spsel",
	{0, 6}: "pan",
	{3, 2}: "dit",
	{3, 3}: "tco",
	{3, 6}: "ssbs",
}

// pstateInstruction decodes MSR <pstatefield>, #<imm> for the fields the
// library does not know, or false when the word is another instruction.
func pstateInstruction(raw uint32, address uint64) (instruction, bool) {
	if raw&0xfff8f01f != 0xd500401f {
		return instruction{}, false
	}
	op1 := (raw >> 16) & 0x7
	op2 := (raw >> 5) & 0x7
	name, ok := pstateFields[[2]uint32{op1, op2}]
	if !ok {
		return instruction{}, false
	}
	immediate := (raw >> 8) & 0xf
	text := fmt.Sprintf("msr %s, #%#x", name, immediate)
	return instruction{
		address: address,
		raw:     raw,
		inst:    arm64asm.Inst{Op: arm64asm.NOP},
		size:    4,
		text:    text,
		pstate:  true,
	}, true
}
