package arm

import (
	"golang.org/x/arch/arm/armasm"
)

// decodeThumb32 decodes one 32-bit Thumb-2 instruction. The encoding is split by
// the top five bits of the first halfword into three groups; each group is
// decoded by the sub-opcode field that follows.
func decodeThumb32(first, second uint16) (thumbInstruction, bool) {
	switch first >> 11 {
	case 0x1d:
		return thumb32LoadStore(first, second)
	case 0x1e:
		return thumb32DataAndBranch(first, second)
	default:
		return thumb32RegisterData(first, second)
	}
}

// thumb2Reg returns the register encoded in a four-bit field.
func thumb2Reg(value uint16) armasm.Reg { return armasm.Reg(value & 0x0f) }

// thumb32LoadStore decodes the 11101 group. Bits 15..9 separate the families:
// 0b1110100 is the multiple and dual family, 0b1110101 the shifted-register
// data processing family, and the rest is coprocessor space.
func thumb32LoadStore(first, second uint16) (thumbInstruction, bool) {
	switch first >> 9 & 0x7f {
	case 0x74:
		return thumb32LoadStoreMultiple(first, second)
	case 0x75:
		return thumb32ShiftedRegister(first, second)
	}
	// The rest of the group is the coprocessor and Advanced SIMD space:
	// vpush, vpop, vldr, vstr and the vector loads. The vector register file has
	// no data-flow model, so the form stays opaque instead of undecodable.
	return thumb32Opaque("vfp", 0, 0, second), true
}

// thumb32LoadStoreMultiple decodes LDM, STM, PUSH, POP, LDRD and STRD.
func thumb32LoadStoreMultiple(first, second uint16) (thumbInstruction, bool) {
	base := uint32(first & 0x0f)
	list := uint32(second)
	writeback := first&0x20 != 0
	load := first&0x10 != 0
	switch {
	case first&0xffc0 == 0xe880, first&0xffc0 == 0xe900:
		// Bits 8 and 7 select the addressing mode: increment after or
		// decrement before. An LDM that writes the stack pointer back and
		// loads the program counter is the epilogue.
		op := armasm.STM
		if load {
			op = armasm.LDM
		}
		if first&0xffc0 == 0xe900 {
			if load {
				op = armasm.LDMDB
			} else {
				op = armasm.STMDB
			}
		}
		memory := armasm.Mem{Base: thumb2Reg(uint16(base)), Mode: armasm.AddrOffset}
		if writeback {
			memory.Mode = armasm.AddrLDM_WB
		}
		return thumb32Set(14, op, memory, armasm.RegList(list)), true
	case first&0x0040 != 0:
		// LDRD and STRD: bit 8 is P, bit 7 is U, bit 5 is W and bit 4 is L.
		// Bit 6 is set in this family and clear in the LDM and STM family.
		var op armasm.Op
		if load {
			op = armasm.LDRD
		} else {
			op = armasm.STRD
		}
		// The immediate is a word offset.
		offset := int32(second&0xff) * 4
		memory := armasm.Mem{
			Base:   thumb2Reg(uint16(base)),
			Mode:   armasm.AddrOffset,
			Offset: int16(offset),
			Sign:   1,
		}
		if first&0x0080 == 0 {
			// U clear means the base decreases.
			memory.Sign = -1
		}
		if first&0x0020 != 0 {
			memory.Mode = armasm.AddrPreIndex
		}
		firstRegister := thumb2Reg(second >> 12 & 0x0f)
		secondRegister := thumb2Reg(second >> 8 & 0x0f)
		return thumb32Set(14, op, firstRegister, secondRegister, memory), true
	}
	return thumbInstruction{}, false
}

// thumb32ShiftedRegister decodes the data processing forms that take a shifted
// register operand.
func thumb32ShiftedRegister(first, second uint16) (thumbInstruction, bool) {
	opcode := first >> 5 & 0x0f
	setFlags := first&0x10 != 0
	rn := uint32(first & 0x0f)
	rd := uint32(second >> 8 & 0x0f)
	rm := uint32(second & 0x0f)
	shiftType := armasm.Shift(second >> 4 & 0x03)
	shiftCount := uint8((second>>12&0x07)<<2 | second>>6&0x03)
	var operand armasm.Arg = thumb2Reg(uint16(rm))
	if shiftCount != 0 || shiftType != armasm.ShiftLeft {
		operand = armasm.RegShift{Reg: armasm.Reg(rm), Shift: shiftType, Count: shiftCount}
	}
	var destination armasm.Arg = thumb2Reg(uint16(rd))
	// A destination of pc marks a compare-like form: the result is discarded
	// and the flags are the output.
	test := rd == 0x0f
	if test {
		destination = armasm.Reg(armasm.R15)
	}
	var base armasm.Arg = thumb2Reg(uint16(rn))
	// Rn of pc (r15) turns ORR, ORN and similar into a two-operand move.
	move := rn == 0x0f
	if move {
		base = nil
	}
	var op armasm.Op
	switch opcode {
	case 0x0:
		op = armasm.AND
	case 0x1:
		op = armasm.BIC
	case 0x2:
		op = armasm.ORR
	case 0x3:
		// armasm has no ORN, so the form stays opaque.
		return thumb32Opaque("orn", rd, rn, second), true
	case 0x4:
		op = armasm.EOR
	case 0x6:
		// PKHBT and PKHTB pack one halfword of Rn with one of Rm. Bit 5 of the
		// second halfword selects the TB form; the shift amount is bits 14..10.
		shift := uint8(second >> 10 & 0x1f)
		if second&0x0020 != 0 {
			return thumb32Set(14, armasm.PKHTB, destination, thumb2Reg(uint16(rn)),
				armasm.RegShift{Reg: armasm.Reg(rm), Shift: armasm.ShiftRightSigned, Count: shift}), true
		}
		return thumb32Set(14, armasm.PKHBT, destination, thumb2Reg(uint16(rn)),
			armasm.RegShift{Reg: armasm.Reg(rm), Shift: armasm.ShiftLeft, Count: shift}), true
	case 0x8:
		op = armasm.ADD
	case 0xa:
		op = armasm.ADC
	case 0xb:
		op = armasm.SBC
	case 0xd:
		op = armasm.SUB
	case 0xe:
		op = armasm.RSB
	default:
		return thumbInstruction{}, false
	}
	if test {
		switch opcode {
		case 0x0:
			op = armasm.TST
		case 0x4:
			op = armasm.TEQ
		case 0x8:
			op = armasm.CMN
		case 0xd:
			op = armasm.CMP
		}
	}
	if move {
		switch opcode {
		case 0x2:
			op = armasm.MOV
		case 0x3:
			op = armasm.MVN
		}
		return thumb32Set(14, op, destination, operand), true
	}
	if test {
		return thumb32Set(14, op, base, operand), true
	}
	if setFlags {
		// The flag-setting variants share the armasm numbering.
		op += 16
	}
	return thumb32Set(14, op, destination, base, operand), true
}

// thumb32DataAndBranch decodes the 11110 group. Bit 15 of the second halfword
// separates the branch and system space from the two immediate data-processing
// families; within the immediate families bit 9 of the first halfword selects
// the modified or the plain binary form.
func thumb32DataAndBranch(first, second uint16) (thumbInstruction, bool) {
	if second&0x8000 != 0 {
		return thumb32BranchOrSystem(first, second)
	}
	if first&0x0200 != 0 {
		return thumb32PlainImmediate(first, second)
	}
	return thumb32ModifiedImmediate(first, second)
}

// thumb32BranchOrSystem decodes the branch and system-control space.
func thumb32BranchOrSystem(first, second uint16) (thumbInstruction, bool) {
	switch second >> 14 & 0x03 {
	case 0x2:
		if second&0x1000 != 0 {
			return thumb32Branch(first, second)
		}
		// A condition field of fifteen marks the system-control forms: the
		// MRS and MSR transfers and the barriers. They have no data-flow
		// model, so they stay opaque.
		if first>>6&0x0f == 0x0f {
			switch first >> 4 & 0x03 {
			case 0x0:
				return thumb32Opaque("msr", 0, 0, second), true
			case 0x1:
				return thumb32Opaque("mrs", 0, 0, second), true
			}
			return set(14, armasm.NOP), true
		}
		return thumb32Branch(first, second)
	case 0x3:
		return thumb32Branch(first, second)
	}
	return thumbInstruction{}, false
}

// thumb32ModifiedImmediate decodes the data processing forms with a
// twelve-bit modified immediate.
func thumb32ModifiedImmediate(first, second uint16) (thumbInstruction, bool) {
	opcode := first >> 5 & 0x0f
	setFlags := first&0x10 != 0
	rn := uint32(first & 0x0f)
	rd := uint32(second >> 8 & 0x0f)
	value, ok := thumbExpandImmediate(int(first&0x0400)<<1 | int(second>>12&0x07)<<8 | int(second&0xff))
	if !ok {
		return thumbInstruction{}, false
	}
	test := rd == 0x0f
	move := rn == 0x0f
	var op armasm.Op
	switch opcode {
	case 0x0:
		op = armasm.AND
	case 0x1:
		op = armasm.BIC
	case 0x2:
		op = armasm.ORR
	case 0x3:
		// armasm has no ORN, so the form stays opaque.
		return thumb32Opaque("orn", rd, rn, second), true
	case 0x4:
		op = armasm.EOR
	case 0x8:
		op = armasm.ADD
	case 0xa:
		op = armasm.ADC
	case 0xb:
		op = armasm.SBC
	case 0xd:
		op = armasm.SUB
	case 0xe:
		op = armasm.RSB
	default:
		return thumbInstruction{}, false
	}
	if test {
		switch opcode {
		case 0x0:
			op = armasm.TST
		case 0x4:
			op = armasm.TEQ
		case 0x8:
			op = armasm.CMN
		case 0xd:
			op = armasm.CMP
		}
		return thumb32Set(14, op, thumb2Reg(uint16(rn)), imm(uint32(value))), true
	}
	if move {
		switch opcode {
		case 0x2:
			op = armasm.MOV
		case 0x3:
			op = armasm.MVN
		}
		return thumb32Set(14, op, thumb2Reg(uint16(rd)), imm(uint32(value))), true
	}
	if setFlags {
		op += 16
	}
	return thumb32Set(14, op, thumb2Reg(uint16(rd)), thumb2Reg(uint16(rn)), imm(uint32(value))), true
}

// thumb32PlainImmediate decodes MOVW, MOVT, ADDW, SUBW, ADR and the bitfield
// forms.
func thumb32PlainImmediate(first, second uint16) (thumbInstruction, bool) {
	opcode := first >> 4 & 0x1f
	rn := uint32(first & 0x0f)
	rd := uint32(second >> 8 & 0x0f)
	imm12 := uint32(second>>12&0x07)<<8 | uint32(second&0xff)
	imm16 := uint32(first&0x000f)<<12 | imm12
	switch opcode {
	case 0x04:
		// MOVW
		return thumb32Set(14, armasm.MOVW, thumb2Reg(uint16(rd)), imm(imm16)), true
	case 0x0c:
		// MOVT
		return thumb32Set(14, armasm.MOVT, thumb2Reg(uint16(rd)), imm(imm16)), true
	case 0x00:
		// ADDW. Rn of pc (r15) turns this into ADR.
		if rn == 0x0f {
			// ADR: the immediate is the distance from the instruction address.
			return thumb32Set(14, armasm.ADD, thumb2Reg(uint16(rd)), pcrel(int32(imm12)+1)), true
		}
		return thumb32Set(14, armasm.ADD, thumb2Reg(uint16(rd)), thumb2Reg(uint16(rn)), imm(imm12)), true
	case 0x0a:
		// SUBW
		return thumb32Set(14, armasm.SUB, thumb2Reg(uint16(rd)), thumb2Reg(uint16(rn)), imm(imm12)), true
	case 0x16:
		// BFI, and BFC when the source is r15
		lsb := uint32(second>>12&0x07)<<2 | uint32(second>>6&0x03)
		width := uint32(second&0x1f) - lsb + 1
		if rn == 0x0f {
			return thumb32Set(14, armasm.BFC, thumb2Reg(uint16(rd)), imm(lsb), imm(width)), true
		}
		return thumb32Set(14, armasm.BFI, thumb2Reg(uint16(rd)), thumb2Reg(uint16(rn)), imm(lsb), imm(width)), true
	case 0x1c:
		// UBFX
		lsb := uint32(second>>12&0x07)<<2 | uint32(second>>6&0x03)
		width := uint32(second&0x1f) + 1
		return thumb32Set(14, armasm.UBFX, thumb2Reg(uint16(rd)), thumb2Reg(uint16(rn)), imm(lsb), imm(width)), true
	case 0x14:
		// SBFX
		lsb := uint32(second>>12&0x07)<<2 | uint32(second>>6&0x03)
		width := uint32(second&0x1f) + 1
		return thumb32Set(14, armasm.SBFX, thumb2Reg(uint16(rd)), thumb2Reg(uint16(rn)), imm(lsb), imm(width)), true
	}
	return thumbInstruction{}, false
}

// thumb32Branch decodes B.W, B<c>.W, BL and BLX. The offset is a signed word
// count with two spliced-in high bits, relative to the instruction address plus
// four.
func thumb32Branch(first, second uint16) (thumbInstruction, bool) {
	sign := int32(first>>10) & 1
	imm6 := int32(first & 0x3f)
	imm10 := int32(first & 0x3ff)
	j1 := int32(second>>13) & 1
	j2 := int32(second>>11) & 1
	imm11 := int32(second & 0x7ff)

	// Bits 15 and 14 of the second halfword select the family and bit 12 the
	// form within it: 10 with bit 12 clear is B<c>.W, 10 with bit 12 set is
	// B.W, 11 with bit 12 set is BL and 11 with bit 12 clear is BLX.
	switch second >> 14 & 0x03 {
	case 0x2:
		if second&0x1000 == 0 {
			// B<c>.W: imm32 = S:J2:J1:imm6:imm11:'0'
			imm32 := sign<<20 | j2<<19 | j1<<18 | imm6<<12 | imm11<<1
			offset := (imm32 << 7) >> 7
			return thumb32Set(int(first>>6&0x0f), armasm.B, pcrel(offset)), true
		}
		// B.W: imm32 = S:I1:I2:imm10:imm11:'0'
		i1 := 1 ^ (j1 ^ sign)
		i2 := 1 ^ (j2 ^ sign)
		imm32 := sign<<24 | i1<<23 | i2<<22 | imm10<<12 | imm11<<1
		offset := (imm32 << 7) >> 7
		return thumb32Set(14, armasm.B, pcrel(offset)), true
	case 0x3:
		i1 := 1 ^ (j1 ^ sign)
		i2 := 1 ^ (j2 ^ sign)
		imm32 := sign<<24 | i1<<23 | i2<<22 | imm10<<12 | imm11<<1
		offset := (imm32 << 7) >> 7
		if second&0x1000 != 0 {
			return thumb32Set(14, armasm.BL, pcrel(offset)), true
		}
		return thumb32Set(14, armasm.BLX, pcrel(offset)), true
	}
	// B.W uses the same shape as BL.
	i1 := 1 ^ (j1 ^ sign)
	i2 := 1 ^ (j2 ^ sign)
	imm32 := sign<<24 | i1<<23 | i2<<22 | imm10<<12 | imm11<<1
	offset := (imm32 << 7) >> 7
	return thumb32Set(14, armasm.B, pcrel(offset)), true
}

// thumb32RegisterData decodes the 11111 group: load and store single, the
// register data processing forms and the multiplies. The coprocessor and
// Advanced SIMD space (first halfword 0xFC00 and above) stays opaque: the
// vector register file has no data-flow model.
func thumb32RegisterData(first, second uint16) (thumbInstruction, bool) {
	if first >= 0xfc00 {
		return thumb32Opaque("vfp", 0, 0, second), true
	}
	if first>>8 == 0xf8 || first>>8 == 0xf9 {
		return thumb32LoadStoreSingle(first, second)
	}
	return thumb32RegisterOp(first, second)
}

// thumb32LoadStoreSingle decodes the load and store forms with an immediate or
// register offset. Bits 11..8 select the access width and signedness, bits 7..4
// the form: a value of eight or above is an immediate offset, a smaller value a
// register offset.
func thumb32LoadStoreSingle(first, second uint16) (thumbInstruction, bool) {
	op := first >> 4 & 0x0f
	signed := first&0x0100 != 0
	rn := uint32(first & 0x0f)
	rd := uint32(second >> 12 & 0x0f)
	if op > 0x0d || op == 0x06 || op == 0x07 {
		// The remaining encodings of the 11111001 group are the Advanced SIMD
		// element and structure accesses: vld1..vld4, vst1..vst4. They touch
		// the vector register file, which has no data-flow model.
		return thumb32Opaque("vfp", 0, 0, second), true
	}
	var opcode armasm.Op
	switch op {
	case 0x08:
		opcode = armasm.STRB
	case 0x09:
		opcode = armasm.LDRB
		if signed {
			opcode = armasm.LDRSB
		}
	case 0x0a:
		opcode = armasm.STRH
	case 0x0b:
		opcode = armasm.LDRH
		if signed {
			opcode = armasm.LDRSH
		}
	case 0x0c:
		opcode = armasm.STR
	case 0x0d:
		opcode = armasm.LDR
	case 0x04:
		opcode = armasm.STR
	case 0x05:
		opcode = armasm.LDR
	case 0x00:
		opcode = armasm.STRB
	case 0x01:
		opcode = armasm.LDRB
	case 0x02:
		opcode = armasm.STRH
	case 0x03:
		opcode = armasm.LDRH
	default:
		return thumbInstruction{}, false
	}
	if op >= 0x08 {
		// Immediate offset form. An Rn of pc (r15) is a literal load, whose
		// offset is relative to the aligned program counter plus four.
		offset := int16(second & 0x0fff)
		if rn == 0x0f {
			return thumb32Set(14, opcode, thumb2Reg(uint16(rd)), pcrel(int32(offset))), true
		}
		memory := armasm.Mem{Base: thumb2Reg(uint16(rn)), Mode: armasm.AddrOffset, Offset: offset}
		return thumb32Set(14, opcode, thumb2Reg(uint16(rd)), memory), true
	}
	// Register offset form: Rt in bits 15..12, Rm in bits 3..0, shift in 5..4.
	shift := armasm.Shift(second >> 4 & 0x03)
	count := uint8(second >> 6 & 0x03)
	var index armasm.Arg = thumb2Reg(second & 0x0f)
	if count != 0 {
		index = armasm.RegShift{Reg: armasm.Reg(second & 0x0f), Shift: shift, Count: count}
	}
	_ = index
	memory := armasm.Mem{
		Base:  thumb2Reg(uint16(rn)),
		Mode:  armasm.AddrOffset,
		Index: armasm.Reg(second & 0x0f),
		Shift: shift,
		Count: count,
		Sign:  1,
	}
	return thumb32Set(14, opcode, thumb2Reg(uint16(rd)), memory), true
}

// thumb32RegisterOp decodes the register forms. The opcode is the seven-bit
// field at bits 10..4 of the first halfword, which the ARM ARM names op1 and
// op2 for this space.
func thumb32RegisterOp(first, second uint16) (thumbInstruction, bool) {
	op := first >> 4 & 0x7f
	rn := uint32(first & 0x0f)
	rd := uint32(second >> 8 & 0x0f)
	ra := uint32(second >> 12 & 0x0f)
	rm := uint32(second & 0x0f)

	switch op {
	case 0x30:
		// MLA or MUL: an accumulator of r15 marks the multiply.
		if ra == 0x0f {
			return thumb32Set(14, armasm.MUL, thumb2Reg(uint16(rd)), thumb2Reg(uint16(rn)), thumb2Reg(uint16(rm))), true
		}
		return thumb32Set(14, armasm.MLA, thumb2Reg(uint16(rd)), thumb2Reg(uint16(rn)), thumb2Reg(uint16(rm)), thumb2Reg(uint16(ra))), true
	case 0x31:
		return thumb32Set(14, armasm.MLS, thumb2Reg(uint16(rd)), thumb2Reg(uint16(rn)), thumb2Reg(uint16(rm)), thumb2Reg(uint16(ra))), true
	case 0x38:
		return thumb32Set(14, armasm.SMULL, thumb2Reg(uint16(ra)), thumb2Reg(uint16(rd)), thumb2Reg(uint16(rn)), thumb2Reg(uint16(rm))), true
	case 0x3a:
		return thumb32Set(14, armasm.UMULL, thumb2Reg(uint16(ra)), thumb2Reg(uint16(rd)), thumb2Reg(uint16(rn)), thumb2Reg(uint16(rm))), true
	case 0x3c:
		return thumb32Set(14, armasm.SMLAL, thumb2Reg(uint16(ra)), thumb2Reg(uint16(rd)), thumb2Reg(uint16(rn)), thumb2Reg(uint16(rm))), true
	case 0x3e:
		return thumb32Set(14, armasm.UMLAL, thumb2Reg(uint16(ra)), thumb2Reg(uint16(rd)), thumb2Reg(uint16(rn)), thumb2Reg(uint16(rm))), true
	case 0x39:
		return thumb32Set(14, armasm.SDIV, thumb2Reg(uint16(rd)), thumb2Reg(uint16(rn)), thumb2Reg(uint16(rm))), true
	case 0x3b:
		return thumb32Set(14, armasm.UDIV, thumb2Reg(uint16(rd)), thumb2Reg(uint16(rn)), thumb2Reg(uint16(rm))), true
	}

	// The register shift forms keep the shift amount in the low byte of Rn and
	// leave bits 7..4 of the second halfword clear.
	if op&0x78 == 0x20 && second&0x00f0 == 0 {
		kind := armasm.LSL
		switch op & 0x06 {
		case 0x02:
			kind = armasm.LSR
		case 0x04:
			kind = armasm.ASR
		case 0x06:
			kind = armasm.ROR
		}
		return thumb32Set(14, kind, thumb2Reg(uint16(rd)), thumb2Reg(uint16(rn)), thumb2Reg(uint16(rm))), true
	}

	// The extend, reverse and count forms keep the operation in bits 7..4 of the
	// second halfword, which the ARM ARM calls op2.
	op2 := second >> 4 & 0x0f
	switch op {
	case 0x20:
		return thumb32Set(14, armasm.SXTH, thumb2Reg(uint16(rd)), thumb2Reg(uint16(rm))), true
	case 0x21:
		return thumb32Set(14, armasm.UXTH, thumb2Reg(uint16(rd)), thumb2Reg(uint16(rm))), true
	case 0x24:
		return thumb32Set(14, armasm.SXTB, thumb2Reg(uint16(rd)), thumb2Reg(uint16(rm))), true
	case 0x25:
		return thumb32Set(14, armasm.UXTB, thumb2Reg(uint16(rd)), thumb2Reg(uint16(rm))), true
	case 0x29:
		// REV, REV16, RBIT and REVSH.
		switch op2 {
		case 0x8:
			return thumb32Set(14, armasm.REV, thumb2Reg(uint16(rd)), thumb2Reg(uint16(rm))), true
		case 0x9:
			return thumb32Set(14, armasm.REV16, thumb2Reg(uint16(rd)), thumb2Reg(uint16(rm))), true
		case 0xa:
			return thumb32Set(14, armasm.RBIT, thumb2Reg(uint16(rd)), thumb2Reg(uint16(rm))), true
		case 0xb:
			return thumb32Set(14, armasm.REVSH, thumb2Reg(uint16(rd)), thumb2Reg(uint16(rm))), true
		}
	case 0x2b:
		return thumb32Set(14, armasm.CLZ, thumb2Reg(uint16(rd)), thumb2Reg(uint16(rm))), true
	}
	return thumbInstruction{}, false
}

// thumb32ExtendReverse decodes the extend, reverse and count-leading forms.
func thumb32ExtendReverse(first, second uint16) (thumbInstruction, bool) {
	if second&0xf080 == 0xf080 {
		switch first >> 4 & 0x03 {
		case 0x0:
			return thumb32Set(14, armasm.REV, thumb2Reg(second>>8&0x0f), thumb2Reg(first&0x0f)), true
		case 0x1:
			return thumb32Set(14, armasm.REV16, thumb2Reg(second>>8&0x0f), thumb2Reg(first&0x0f)), true
		case 0x3:
			return thumb32Set(14, armasm.REVSH, thumb2Reg(second>>8&0x0f), thumb2Reg(first&0x0f)), true
		}
	}
	switch first >> 4 & 0x1f {
	case 0x1a:
		return thumb32Set(14, armasm.SXTB, thumb2Reg(second>>8&0x0f), thumb2Reg(first&0x0f)), true
	case 0x1b:
		return thumb32Set(14, armasm.UXTB, thumb2Reg(second>>8&0x0f), thumb2Reg(first&0x0f)), true
	case 0x1c:
		return thumb32Set(14, armasm.SXTH, thumb2Reg(second>>8&0x0f), thumb2Reg(first&0x0f)), true
	case 0x1d:
		return thumb32Set(14, armasm.UXTH, thumb2Reg(second>>8&0x0f), thumb2Reg(first&0x0f)), true
	}
	return thumbInstruction{}, false
}

// thumb32Opaque records an instruction that armasm cannot express. rd and rn
// are the destination and first source register, second carries the rest.
func thumb32Opaque(name string, rd, rn uint32, second uint16) thumbInstruction {
	if name == "vfp" {
		return thumbInstruction{text: "vfp", opaque: name}
	}
	return thumbInstruction{
		text:       name + " " + registerText(rd) + ", " + registerText(rn) + ", " + registerText(uint32(second&0x0f)),
		opaque:     name,
		opaqueArgs: []armasm.Arg{armasm.Reg(rd & 0x0f), armasm.Reg(rn & 0x0f), armasm.Reg(second & 0x0f)},
	}
}

// thumb32Set builds one 32-bit instruction.
func thumb32Set(cond int, op armasm.Op, args ...armasm.Arg) thumbInstruction {
	return set(cond, op, args...)
}

// thumbExpandImmediate expands the twelve-bit Thumb modified immediate.
func thumbExpandImmediate(imm12 int) (uint32, bool) {
	if imm12&0xc00 == 0 {
		switch imm12 >> 8 & 0x03 {
		case 0x0:
			return uint32(imm12 & 0xff), true
		case 0x1:
			value := uint32(imm12 & 0xff)
			return value<<16 | value, true
		case 0x2:
			value := uint32(imm12 & 0xff)
			return value<<24 | value<<8, true
		default:
			value := uint32(imm12 & 0xff)
			return value<<24 | value<<16 | value<<8 | value, true
		}
	}
	value := uint32(0x80 | imm12&0x7f)
	rotate := uint32(imm12>>7) & 0x1f
	if rotate == 0 {
		return value, true
	}
	return value>>rotate | value<<(32-rotate), true
}
