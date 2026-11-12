package arm

import (
	"golang.org/x/arch/arm/armasm"
)

// decodeThumb16 decodes one 16-bit Thumb instruction. The encoding is grouped by
// the top six bits, which the ARM ARM calls the opcode group. cond is the
// condition the enclosing IT block imposes.
func decodeThumb16(hw uint16, cond int) (thumbInstruction, bool) {
	// The group is the top six bits; the ARM ARM tables A5-1 and A5-2 number
	// them from 0b000000. The three 32-bit prefixes sit above and never reach
	// this function.
	switch hw >> 10 {
	case 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07:
		// 000xxx: shift by immediate, then ADD and SUB
		return thumb16ShiftOrAddSub(hw, cond)
	case 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f:
		// 001xxx: MOV, CMP, ADD and SUB with an eight-bit immediate
		return thumb16Immediate(hw, cond)
	case 0x10:
		// 010000: data processing on two low registers
		return thumb16DataProcessing(hw, cond)
	case 0x11:
		// 010001: high-register operations and branch exchange
		return thumb16HighRegister(hw, cond)
	case 0x12, 0x13:
		// 01001x: LDR with a PC-relative literal
		return thumb16LoadLiteral(hw, cond)
	case 0x14, 0x15, 0x16, 0x17:
		// 0101xx: load and store with a register offset
		return thumb16LoadStoreRegister(hw, cond)
	case 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f:
		// 011xxx: load and store word or byte with a scaled immediate
		return thumb16LoadStoreWordByte(hw, cond)
	case 0x20, 0x21, 0x22, 0x23:
		// 1000xx: load and store halfword with a scaled immediate
		return thumb16LoadStoreHalfword(hw, cond)
	case 0x24, 0x25, 0x26, 0x27:
		// 1001xx: load and store relative to the stack pointer
		return thumb16LoadStoreStack(hw, cond)
	case 0x28, 0x29, 0x2a, 0x2b:
		// 1010xx: ADR, and ADD with an immediate to the stack pointer
		return thumb16Address(hw, cond)
	case 0x2c, 0x2d, 0x2e, 0x2f:
		// 1011xx: stack adjustment, compare and branch, extend, push and pop,
		// hints and the branch-exchange space
		return thumb16Misc(hw, cond)
	case 0x30, 0x31, 0x32, 0x33:
		// 1100xx: load and store multiple
		return thumb16LoadStoreMultiple(hw, cond)
	case 0x34, 0x35, 0x36, 0x37:
		// 1101xx: conditional branch, supervisor call, permanent undefined
		return thumb16ConditionalBranch(hw, cond)
	case 0x38, 0x39:
		// 11100x: unconditional branch
		return thumb16Branch(hw)
	}
	return thumbInstruction{}, false
}

// thumb16ShiftOrAddSub decodes the shift-by-immediate group and the ADD/SUB
// forms that share it. bit 10 selects subtraction, bit 9 an immediate operand.
func thumb16ShiftOrAddSub(hw uint16, cond int) (thumbInstruction, bool) {
	if hw&0x1800 == 0x1800 {
		op := armasm.ADD
		if hw&0x0200 != 0 {
			op = armasm.SUB
		}
		destination := reg(uint32(hw & 0x07))
		source := reg(uint32(hw >> 3 & 0x07))
		if hw&0x0400 != 0 {
			return set(cond, op, destination, source, imm(uint32(hw>>6&0x07))), true
		}
		return set(cond, op, destination, source, reg(uint32(hw>>6&0x07))), true
	}
	var op armasm.Op
	switch hw >> 11 {
	case 0x00:
		op = armasm.LSL
	case 0x01:
		op = armasm.LSR
	default:
		op = armasm.ASR
	}
	destination := reg(uint32(hw & 0x07))
	source := reg(uint32(hw >> 3 & 0x07))
	count := uint32(hw >> 6 & 0x1f)
	if count == 0 {
		if op == armasm.LSL {
			// A zero left shift is the canonical register move.
			return set(cond, armasm.MOV, destination, source), true
		}
		count = 32
	}
	return set(cond, op, destination, source, imm(count)), true
}

// thumb16Immediate decodes MOV, CMP, ADD and SUB with an eight-bit immediate.
func thumb16Immediate(hw uint16, cond int) (thumbInstruction, bool) {
	var op armasm.Op
	switch hw >> 11 & 0x03 {
	case 0x00:
		op = armasm.MOV
	case 0x01:
		op = armasm.CMP
	case 0x02:
		op = armasm.ADD
	default:
		op = armasm.SUB
	}
	return set(cond, op, reg(uint32(hw>>8&0x07)), imm(uint32(hw&0xff))), true
}

// thumb16DataProcessing decodes the sixteen two-operand ALU forms.
func thumb16DataProcessing(hw uint16, cond int) (thumbInstruction, bool) {
	destination := reg(uint32(hw & 0x07))
	source := reg(uint32(hw >> 3 & 0x07))
	switch hw >> 6 & 0x0f {
	case 0x0:
		return set(cond, armasm.AND, destination, source), true
	case 0x1:
		return set(cond, armasm.EOR, destination, source), true
	case 0x2:
		return set(cond, armasm.LSL, destination, source), true
	case 0x3:
		return set(cond, armasm.LSR, destination, source), true
	case 0x4:
		return set(cond, armasm.ASR, destination, source), true
	case 0x5:
		return set(cond, armasm.ADC, destination, source), true
	case 0x6:
		return set(cond, armasm.SBC, destination, source), true
	case 0x7:
		return set(cond, armasm.ROR, destination, source), true
	case 0x8:
		return set(cond, armasm.TST, destination, source), true
	case 0x9:
		// RSB with a zero minuend is the canonical negate.
		return set(cond, armasm.RSB, destination, source, imm(0)), true
	case 0xa:
		return set(cond, armasm.CMP, destination, source), true
	case 0xb:
		return set(cond, armasm.CMN, destination, source), true
	case 0xc:
		return set(cond, armasm.ORR, destination, source), true
	case 0xd:
		return set(cond, armasm.MUL, destination, source, destination), true
	case 0xe:
		return set(cond, armasm.BIC, destination, source), true
	default:
		return set(cond, armasm.MVN, destination, source), true
	}
}

// thumb16HighRegister decodes the high-register operations and the
// branch-exchange forms. bit 7 is the high bit of the destination and bit 6 the
// high bit of the source.
func thumb16HighRegister(hw uint16, cond int) (thumbInstruction, bool) {
	destination := uint32(hw&0x07) | uint32(hw>>4&0x08)
	source := uint32(hw>>3&0x0f) | uint32(hw&0x80)>>4
	switch hw >> 8 & 0x03 {
	case 0x0:
		return set(cond, armasm.ADD, reg(destination), reg(destination), reg(source)), true
	case 0x1:
		return set(cond, armasm.CMP, reg(destination), reg(source)), true
	case 0x2:
		return set(cond, armasm.MOV, reg(destination), reg(source)), true
	default:
		if hw&0x0080 != 0 {
			return set(cond, armasm.BLX, reg(source)), true
		}
		return set(cond, armasm.BX, reg(source)), true
	}
}

// thumb16LoadLiteral decodes the PC-relative load. The offset is a word count
// from the aligned program counter, which the armasm PCRel convention expresses
// relative to the instruction address plus eight.
func thumb16LoadLiteral(hw uint16, cond int) (thumbInstruction, bool) {
	offset := int32(hw&0xff) * 4
	return set(cond, armasm.LDR, reg(uint32(hw>>8&0x07)), pcrel(offset)), true
}

// thumbLoadStoreOp maps the three-bit register-offset selector to an opcode.
func thumbLoadStoreOp(field uint16) (armasm.Op, bool) {
	switch field {
	case 0x0:
		return armasm.STR, true
	case 0x1:
		return armasm.STRH, true
	case 0x2:
		return armasm.STRB, true
	case 0x3:
		return armasm.LDRSB, true
	case 0x4:
		return armasm.LDR, true
	case 0x5:
		return armasm.LDRH, true
	case 0x6:
		return armasm.LDRB, true
	case 0x7:
		return armasm.LDRSH, true
	}
	return 0, false
}

// thumb16LoadStoreRegister decodes the register-offset load and store forms.
// The selector sits in the same bits in both places, so one mapper serves the
// wide and the narrow halfword.
func thumb16LoadStoreRegister(hw uint16, cond int) (thumbInstruction, bool) {
	op, ok := thumbLoadStoreOp(hw >> 9 & 0x07)
	if !ok {
		return thumbInstruction{}, false
	}
	memory := armasm.Mem{Base: reg(uint32(hw >> 3 & 0x07)), Mode: armasm.AddrOffset}
	if hw&0x0200 != 0 {
		memory.Index = reg(uint32(hw >> 6 & 0x07))
		memory.Sign = 1
	} else {
		memory.Base = reg(uint32(hw >> 6 & 0x07))
		memory.Index = reg(uint32(hw >> 3 & 0x07))
		memory.Sign = 1
	}
	return set(cond, op, reg(uint32(hw&0x07)), memory), true
}

// thumb16LoadStoreWordByte decodes the word and byte forms with a scaled
// immediate offset.
func thumb16LoadStoreWordByte(hw uint16, cond int) (thumbInstruction, bool) {
	byteForm := hw&0x1000 != 0
	load := hw&0x0800 != 0
	var op armasm.Op
	switch {
	case byteForm && load:
		op = armasm.LDRB
	case byteForm:
		op = armasm.STRB
	case load:
		op = armasm.LDR
	default:
		op = armasm.STR
	}
	offset := int32(hw >> 6 & 0x1f)
	if !byteForm {
		offset *= 4
	}
	memory := armasm.Mem{Base: reg(uint32(hw >> 3 & 0x07)), Mode: armasm.AddrOffset, Offset: int16(offset)}
	return set(cond, op, reg(uint32(hw&0x07)), memory), true
}

// thumb16LoadStoreHalfword decodes the halfword forms with a scaled immediate.
func thumb16LoadStoreHalfword(hw uint16, cond int) (thumbInstruction, bool) {
	op := armasm.STRH
	if hw&0x0800 != 0 {
		op = armasm.LDRH
	}
	memory := armasm.Mem{
		Base:   reg(uint32(hw >> 3 & 0x07)),
		Mode:   armasm.AddrOffset,
		Offset: int16(int32(hw>>6&0x1f) * 2),
	}
	return set(cond, op, reg(uint32(hw&0x07)), memory), true
}

// thumb16LoadStoreStack decodes the stack-pointer relative forms.
func thumb16LoadStoreStack(hw uint16, cond int) (thumbInstruction, bool) {
	op := armasm.STR
	if hw&0x0800 != 0 {
		op = armasm.LDR
	}
	memory := armasm.Mem{
		Base:   armasm.SP,
		Mode:   armasm.AddrOffset,
		Offset: int16(int32(hw&0xff) * 4),
	}
	return set(cond, op, reg(uint32(hw>>8&0x07)), memory), true
}

// thumb16Address decodes ADR and ADD with an immediate to the stack pointer.
func thumb16Address(hw uint16, cond int) (thumbInstruction, bool) {
	destination := reg(uint32(hw >> 8 & 0x07))
	if hw&0x0800 != 0 {
		return set(cond, armasm.ADD, destination, reg(uint32(armasm.SP)), imm(uint32(hw&0xff)*4)), true
	}
	// ADR adds an aligned PC value to the instruction address plus four.
	return set(cond, armasm.ADD, destination, pcrel(int32(hw&0xff)*4)), true
}

// thumb16Misc decodes the 1011 group: stack adjustment, compare and branch on
// zero, the extend forms, push and pop, and the hint and branch-exchange space.
func thumb16Misc(hw uint16, cond int) (thumbInstruction, bool) {
	switch hw >> 8 & 0x0f {
	case 0x0:
		op := armasm.ADD
		if hw&0x0080 != 0 {
			op = armasm.SUB
		}
		return set(cond, op, reg(uint32(armasm.SP)), reg(uint32(armasm.SP)), imm(uint32(hw&0x7f)*4)), true
	case 0x1, 0x3, 0x9, 0xb:
		// CBZ and CBNZ. Bit 11 selects the zero test direction, bit 9 is the
		// high half of the halfword offset.
		offset := int64(hw>>3&0x1f) << 1
		if hw&0x0200 != 0 {
			offset |= 1 << 6
		}
		mnemonic := "cbz"
		if hw&0x0800 != 0 {
			mnemonic = "cbnz"
		}
		return thumbInstruction{
			text:    mnemonic + " " + registerText(uint32(hw&0x07)) + ", .+" + itoa(offset),
			compare: &compareBranch{mnemonic: mnemonic, register: uint32(hw & 0x07), target: offset},
		}, true
	case 0x2:
		destination := reg(uint32(hw & 0x07))
		source := reg(uint32(hw >> 3 & 0x07))
		switch hw >> 6 & 0x03 {
		case 0x0:
			return set(cond, armasm.SXTH, destination, source), true
		case 0x1:
			return set(cond, armasm.SXTB, destination, source), true
		case 0x2:
			return set(cond, armasm.UXTH, destination, source), true
		default:
			return set(cond, armasm.UXTB, destination, source), true
		}
	case 0x4:
		mask := uint32(hw & 0xff)
		if hw&0x0100 != 0 {
			mask |= 1 << 14
		}
		return set(cond, armasm.PUSH, list(mask)), true
	case 0x5:
		mask := uint32(hw&0xff) | 1<<14
		return set(cond, armasm.PUSH, list(mask)), true
	case 0xc:
		mask := uint32(hw & 0xff)
		if hw&0x0100 != 0 {
			mask |= 1 << 15
		}
		return set(cond, armasm.POP, list(mask)), true
	case 0xd:
		mask := uint32(hw&0xff) | 1<<15
		return set(cond, armasm.POP, list(mask)), true
	case 0xa:
		// REV, REV16 and REVSH.
		destination := reg(uint32(hw & 0x07))
		source := reg(uint32(hw >> 3 & 0x07))
		switch hw >> 6 & 0x03 {
		case 0x0:
			return set(cond, armasm.REV, destination, source), true
		case 0x1:
			return set(cond, armasm.REV16, destination, source), true
		case 0x3:
			return set(cond, armasm.REVSH, destination, source), true
		}
		return thumbInstruction{}, false
	case 0xe:
		// BKPT. The operand is the eight-bit immediate in the low byte.
		return set(14, armasm.BKPT, imm(uint32(hw&0xff))), true
	case 0x6, 0x7:
		// CPS and SETEND change processor state; the effect stays opaque.
		return set(cond, armasm.NOP), true
	case 0xf:
		return thumb16Hint(hw, cond)
	}
	return thumbInstruction{}, false
}

// thumb16Hint decodes the hint space. A hint cannot be conditional and no IT
// block may contain one, so the condition is always AL.
func thumb16Hint(hw uint16, cond int) (thumbInstruction, bool) {
	switch hw >> 4 & 0x0f {
	case 0x0:
		return set(14, armasm.NOP), true
	case 0x1:
		return set(14, armasm.YIELD), true
	case 0x2:
		return set(14, armasm.WFE), true
	case 0x3:
		return set(14, armasm.WFI), true
	case 0x4:
		return set(14, armasm.SEV), true
	default:
		return set(14, armasm.NOP), true
	}
}

// thumb16LoadStoreMultiple decodes the STM and LDM forms. The writeback is
// implicit in Thumb.
func thumb16LoadStoreMultiple(hw uint16, cond int) (thumbInstruction, bool) {
	op := armasm.STM
	if hw&0x0800 != 0 {
		op = armasm.LDM
	}
	memory := armasm.Mem{Base: reg(uint32(hw >> 8 & 0x07)), Mode: armasm.AddrLDM_WB}
	return set(cond, op, memory, list(uint32(hw&0xff))), true
}

// thumb16ConditionalBranch decodes the conditional branch, the supervisor call
// and the permanent undefined form. The offset is a signed halfword count
// relative to the instruction address plus four.
func thumb16ConditionalBranch(hw uint16, cond int) (thumbInstruction, bool) {
	code := int(hw >> 8 & 0x0f)
	// Neither the supervisor call nor the permanent undefined form carries a
	// condition, and no IT block may contain them.
	switch code {
	case 0x0e:
		return set(14, armasm.UNDEF, imm(uint32(hw&0xff))), true
	case 0x0f:
		return set(14, armasm.SVC, imm(uint32(hw&0xff))), true
	}
	offset := int32(int8(hw&0xff)) * 2
	return set(code, armasm.B, pcrel(offset)), true
}

// thumb16Branch decodes the unconditional branch.
func thumb16Branch(hw uint16) (thumbInstruction, bool) {
	offset := int32(int16(hw<<5) >> 4)
	return set(14, armasm.B, pcrel(offset)), true
}
