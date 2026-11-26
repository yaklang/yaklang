package mips

import (
	"fmt"
	"sort"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// The MIPS encoding has three fixed formats plus a set of special cases. The
// decoder below follows the MIPS32/MIPS64 instruction tables: the primary
// opcode is bits 31:26, and for opcode zero the function field is bits 5:0.
const (
	opcodeSpecial  = 0x00
	opcodeSpecial2 = 0x1c
	opcodeSpecial3 = 0x1f
	opcodeRegimm   = 0x01
	opcodeCop0     = 0x10
	opcodeCop1     = 0x11
)

// decodeWord decodes one 32-bit word into a mnemonic and its operands.
func decodeWord(raw uint32, profile Profile) (instruction, bool) {
	opcode := raw >> 26
	rs := int((raw >> 21) & 0x1f)
	rt := int((raw >> 16) & 0x1f)
	rd := int((raw >> 11) & 0x1f)
	sa := int((raw >> 6) & 0x1f)
	funct := raw & 0x3f
	simm := int64(int16(raw & 0xffff))
	uimm := int64(raw & 0xffff)

	switch opcode {
	case opcodeSpecial:
		return decodeSpecial(raw, rs, rt, rd, sa, funct, profile)
	case opcodeSpecial2:
		return decodeSpecial2(raw, rs, rt, rd, sa, funct, profile)
	case opcodeSpecial3:
		return decodeSpecial3(raw, rs, rt, rd, sa, funct, profile)
	case opcodeRegimm:
		name := ""
		switch rt {
		case 0x00:
			name = "bltz"
		case 0x01:
			name = "bgez"
		case 0x02:
			name = "bltzl"
		case 0x03:
			name = "bgezl"
		case 0x10:
			name = "bltzal"
		case 0x11:
			name = "bgezal"
		case 0x12:
			name = "bltzall"
		case 0x13:
			name = "bgezall"
		default:
			return instruction{}, false
		}
		if name == "bgezal" && rs == 0 {
			// BAL is BGEZAL with a zero source: the branch is always taken, so
			// only the link write remains and the form is a call.
			name = "bal"
		}
		target := branchAddress(raw, rs)
		return instruction{op: name, operands: []operand{
			{kind: operandRegister, register: rs},
			{kind: operandImmediate, immediat: target},
		}}, true
	case opcodeCop1:
		if rs >= 0x10 {
			// rs >= 0x10 selects the arithmetic and conversion forms; the
			// mnemonic is built from the format field and the function field.
			if name, ok := coprocessor1Arithmetic(uint32(rs), funct); ok {
				format := coprocessor1Format(uint32(rs))
				return instruction{op: name + "." + format, operands: []operand{
					{kind: operandRegister, register: 32 + int((raw>>6)&0x1f)}, // fN
					{kind: operandRegister, register: 32 + int((raw>>11)&0x1f)},
					{kind: operandRegister, register: 32 + int((raw>>16)&0x1f)},
				}}, true
			}
			return instruction{}, false
		}
		name, isBranch, ok := coprocessorName(opcode, uint32(rs), uint32(rt), funct)
		if !ok {
			return instruction{}, false
		}
		if isBranch {
			return instruction{op: name, operands: []operand{
				{kind: operandImmediate, immediat: branchAddress(raw, 0)},
			}}, true
		}
		return instruction{op: name, operands: []operand{
			{kind: operandRegister, register: rt},
		}}, true
	}

	name, layout, ok := primaryInstruction(opcode)
	if !ok {
		return instruction{}, false
	}
	return buildPrimary(name, layout, raw, rs, rt, rd, sa, simm, uimm, funct), true
}

// primaryInstruction returns the mnemonic and the operand layout of one primary
// opcode.
func primaryInstruction(opcode uint32) (string, string, bool) {
	table := map[uint32]struct {
		name   string
		layout string
	}{
		0x02: {"j", "j"},
		0x03: {"jal", "j"},
		0x04: {"beq", "b"},
		0x05: {"bne", "b"},
		0x06: {"blez", "b1"},
		0x07: {"bgtz", "b1"},
		0x08: {"addi", "i"},
		0x09: {"addiu", "i"},
		0x0a: {"slti", "i"},
		0x0b: {"sltiu", "i"},
		0x0c: {"andi", "iu"},
		0x0d: {"ori", "iu"},
		0x0e: {"xori", "iu"},
		0x0f: {"lui", "lui"},
		0x14: {"beql", "b"},
		0x15: {"bnel", "b"},
		0x16: {"blezl", "b1"},
		0x17: {"bgtzl", "b1"},
		0x18: {"daddi", "i"},
		0x19: {"daddiu", "i"},
		// The unaligned doubleword forms load or store one object as two halves.
		0x1a: {"ldl", "m"},
		0x1b: {"ldr", "m"},
		0x20: {"lb", "m"},
		0x21: {"lh", "m"},
		0x22: {"lwl", "m"},
		0x23: {"lw", "m"},
		0x24: {"lbu", "m"},
		0x25: {"lhu", "m"},
		0x26: {"lwr", "m"},
		0x27: {"lwu", "m"},
		0x28: {"sb", "m"},
		0x29: {"sh", "m"},
		0x2a: {"swl", "m"},
		0x2b: {"sw", "m"},
		0x2c: {"sdl", "m"},
		0x2d: {"sdr", "m"},
		0x2e: {"swr", "m"},
		0x2f: {"cache", "m"},
		0x30: {"ll", "m"},
		0x31: {"lwc1", "m"},
		0x33: {"pref", "m"},
		0x34: {"lld", "m"},
		0x35: {"ldc1", "m"},
		0x37: {"ld", "m"},
		0x38: {"sc", "m"},
		0x39: {"swc1", "m"},
		0x3c: {"scd", "m"},
		0x3d: {"sdc1", "m"},
		0x3f: {"sd", "m"},
	}
	entry, ok := table[opcode]
	if !ok {
		return "", "", false
	}
	return entry.name, entry.layout, true
}

// buildPrimary assembles one instruction from the primary table.
func buildPrimary(name, layout string, raw uint32, rs, rt, rd, sa int, simm, uimm int64, funct uint32) instruction {
	switch layout {
	case "j":
		return instruction{op: name, operands: []operand{
			{kind: operandImmediate, immediat: jumpAddress(raw, 0)},
		}}
	case "b":
		return instruction{op: name, operands: []operand{
			{kind: operandRegister, register: rs},
			{kind: operandRegister, register: rt},
			{kind: operandImmediate, immediat: branchAddress(raw, 0)},
		}}
	case "b1":
		return instruction{op: name, operands: []operand{
			{kind: operandRegister, register: rs},
			{kind: operandImmediate, immediat: branchAddress(raw, 0)},
		}}
	case "i":
		return instruction{op: name, operands: []operand{
			{kind: operandRegister, register: rt},
			{kind: operandRegister, register: rs},
			{kind: operandImmediate, immediat: simm},
		}}
	case "iu":
		return instruction{op: name, operands: []operand{
			{kind: operandRegister, register: rt},
			{kind: operandRegister, register: rs},
			{kind: operandImmediate, immediat: uimm},
		}}
	case "lui":
		return instruction{op: name, operands: []operand{
			{kind: operandRegister, register: rt},
			{kind: operandImmediate, immediat: uimm},
		}}
	case "m":
		return instruction{op: name, operands: []operand{
			{kind: operandRegister, register: rt},
			{kind: operandMemory, base: rs, immediat: simm},
		}}
	}
	return instruction{}
}

// decodeSpecial handles opcode 0: the register-register and shift forms.
func decodeSpecial(raw uint32, rs, rt, rd, sa int, funct uint32, profile Profile) (instruction, bool) {
	// The all-zero word is the canonical NOP: sll $zero, $zero, 0. Naming it
	// keeps the delay slot readable instead of lifting three dead constants.
	if raw == 0 {
		return instruction{op: "nop"}, true
	}
	type entry struct {
		name   string
		layout string
	}
	table := map[uint32]entry{
		0x00: {"sll", "shift"},
		0x02: {"srl", "shift"},
		0x03: {"sra", "shift"},
		0x04: {"sllv", "rrr"},
		0x06: {"srlv", "rrr"},
		0x07: {"srav", "rrr"},
		0x08: {"jr", "r1"},
		0x09: {"jalr", "jalr"},
		0x01: {"movf", "rrr"},
		0x0a: {"movz", "rrr"},
		0x0b: {"movn", "rrr"},
		0x0c: {"syscall", "none"},
		0x0d: {"break", "none"},
		0x0f: {"sync", "none"},
		0x10: {"mfhi", "r1dst"},
		0x11: {"mthi", "r1"},
		0x12: {"mflo", "r1dst"},
		0x13: {"mtlo", "r1"},
		0x14: {"dsllv", "rrr"},
		0x16: {"dsrlv", "rrr"},
		0x17: {"dsrav", "rrr"},
		0x18: {"mult", "rr2"},
		0x19: {"multu", "rr2"},
		0x1a: {"div", "rr2"},
		0x1b: {"divu", "rr2"},
		0x1c: {"dmult", "rr2"},
		0x1d: {"dmultu", "rr2"},
		0x1e: {"ddiv", "rr2"},
		0x1f: {"ddivu", "rr2"},
		0x20: {"add", "rrr"},
		0x21: {"addu", "rrr"},
		0x22: {"sub", "rrr"},
		0x23: {"subu", "rrr"},
		0x24: {"and", "rrr"},
		0x25: {"or", "rrr"},
		0x26: {"xor", "rrr"},
		0x27: {"nor", "rrr"},
		0x2a: {"slt", "rrr"},
		0x2b: {"sltu", "rrr"},
		0x2c: {"dadd", "rrr"},
		0x2d: {"daddu", "rrr"},
		0x2e: {"dsub", "rrr"},
		0x2f: {"dsubu", "rrr"},
		0x30: {"tge", "rr2"},
		0x31: {"tgeu", "rr2"},
		0x32: {"tlt", "rr2"},
		0x33: {"tltu", "rr2"},
		0x34: {"teq", "rr2"},
		0x36: {"tne", "rr2"},
		0x38: {"dsll", "shift"},
		0x3a: {"dsrl", "shift"},
		0x3b: {"dsra", "shift"},
		0x3c: {"dsll32", "shift"},
		0x3e: {"dsrl32", "shift"},
		0x3f: {"dsra32", "shift"},
	}
	item, ok := table[funct]
	if !ok {
		return instruction{}, false
	}
	switch item.layout {
	case "shift":
		mnemonic := item.name
		shift := sa
		if funct == 0x3c || funct == 0x3e || funct == 0x3f {
			shift += 32
		}
		return instruction{op: mnemonic, operands: []operand{
			{kind: operandRegister, register: rd},
			{kind: operandRegister, register: rt},
			{kind: operandImmediate, immediat: int64(shift)},
		}}, true
	case "rrr":
		return instruction{op: item.name, operands: []operand{
			{kind: operandRegister, register: rd},
			{kind: operandRegister, register: rs},
			{kind: operandRegister, register: rt},
		}}, true
	case "rr2":
		return instruction{op: item.name, operands: []operand{
			{kind: operandRegister, register: rs},
			{kind: operandRegister, register: rt},
		}}, true
	case "r1":
		return instruction{op: item.name, operands: []operand{
			{kind: operandRegister, register: rs},
		}}, true
	case "r1dst":
		return instruction{op: item.name, operands: []operand{
			{kind: operandRegister, register: rd},
		}}, true
	case "jalr":
		// JALR <rd>, <rs>
		return instruction{op: item.name, operands: []operand{
			{kind: operandRegister, register: rd},
			{kind: operandRegister, register: rs},
		}}, true
	case "none":
		return instruction{op: item.name}, true
	default:
		return instruction{op: item.name}, true
	}
}

func decodeSpecial2(raw uint32, rs, rt, rd, sa int, funct uint32, profile Profile) (instruction, bool) {
	table := map[uint32]string{
		0x00: "madd",
		0x01: "maddu",
		0x02: "mul",
		0x04: "msub",
		0x05: "msubu",
		0x20: "clz",
		0x21: "clo",
		0x22: "dclz",
		0x23: "dclo",
		0x3f: "sdbbp",
	}
	name, ok := table[funct]
	if !ok {
		return instruction{}, false
	}
	switch funct {
	case 0x02:
		return instruction{op: name, operands: []operand{
			{kind: operandRegister, register: rd},
			{kind: operandRegister, register: rs},
			{kind: operandRegister, register: rt},
		}}, true
	case 0x20, 0x21, 0x22, 0x23:
		return instruction{op: name, operands: []operand{
			{kind: operandRegister, register: rd},
			{kind: operandRegister, register: rs},
		}}, true
	case 0x3f:
		return instruction{op: name}, true
	default:
		return instruction{op: name, operands: []operand{
			{kind: operandRegister, register: rs},
			{kind: operandRegister, register: rt},
		}}, true
	}
}

func decodeSpecial3(raw uint32, rs, rt, rd, sa int, funct uint32, profile Profile) (instruction, bool) {
	// SPECIAL3 splits by function: EXT/DEXT extract a bit field, INS/DINS insert
	// one, and RDHWR reads a hardware register. EXT and INS operate on 32 bits;
	// DEXT and DINS read the 64-bit register.
	switch funct {
	case 0x3b:
		// RDHWR: 011111|00000|rt:5|rd:5|00000|111011. rd receives the value
		// of the hardware register that rt selects.
		return instruction{op: "rdhwr", operands: []operand{
			{kind: operandRegister, register: rd},
			{kind: operandRegister, register: rt},
		}}, true
	case 0x00, 0x03:
		name := "ext"
		if funct == 0x03 {
			name = "dext"
		}
		return instruction{op: name, operands: []operand{
			{kind: operandRegister, register: rt},
			{kind: operandRegister, register: rs},
			{kind: operandImmediate, immediat: int64(sa)},
			{kind: operandImmediate, immediat: int64(rd + 1)},
		}}, true
	case 0x04, 0x07:
		name := "ins"
		if funct == 0x07 {
			name = "dins"
		}
		return instruction{op: name, operands: []operand{
			{kind: operandRegister, register: rt},
			{kind: operandRegister, register: rs},
			{kind: operandImmediate, immediat: int64(sa)},
			{kind: operandImmediate, immediat: int64(rd - sa + 1)},
		}}, true
	}
	return instruction{}, false
}

// func2 reads the secondary opcode of the SPECIAL3 space (bits 10:6).
func func2(raw uint32) uint32 { return (raw >> 6) & 0x1f }

// coprocessorName maps the CP0 and CP1 forms that appear in ordinary code. The
// second result reports whether the instruction is a PC-relative branch.
func coprocessorName(opcode, rs, rt uint32, funct uint32) (string, bool, bool) {
	switch opcode {
	case opcodeCop0:
		switch rs {
		case 0x00:
			return "mfc0", false, true
		case 0x04:
			return "mtc0", false, true
		case 0x0b:
			return "di", false, true
		case 0x0c:
			return "ei", false, true
		}
		return "", false, false
	case opcodeCop1:
		switch rs {
		case 0x00:
			return "mfc1", false, true
		case 0x01:
			// DMFC1/ DMTC1 move a 64-bit value between the integer and the
			// floating-point register files.
			return "dmfc1", false, true
		case 0x02:
			return "cfc1", false, true
		case 0x04:
			return "mtc1", false, true
		case 0x05:
			return "dmtc1", false, true
		case 0x06:
			return "ctc1", false, true
		case 0x08:
			name, ok := coprocessor1Branch(rt)
			return name, true, ok
		}
	}
	return "", false, false
}

// coprocessor1Arithmetic returns the mnemonic of one CP1 arithmetic form. The
// rs field is the operand format, the function field selects the operation.
func coprocessor1Arithmetic(format uint32, funct uint32) (string, bool) {
	switch funct {
	case 0x00:
		return "add", true
	case 0x01:
		return "sub", true
	case 0x02:
		return "mul", true
	case 0x03:
		return "div", true
	case 0x04:
		return "sqrt", true
	case 0x05:
		return "abs", true
	case 0x06:
		return "mov", true
	case 0x07:
		return "neg", true
	case 0x08:
		return "round.l", true
	case 0x09:
		return "trunc.l", true
	case 0x0a:
		return "ceil.l", true
	case 0x0b:
		return "floor.l", true
	case 0x0c:
		return "round.w", true
	case 0x0d:
		return "trunc.w", true
	case 0x0e:
		return "ceil.w", true
	case 0x0f:
		return "floor.w", true
	case 0x12:
		return "movz", true
	case 0x13:
		return "movn", true
	case 0x15:
		return "recip", true
	case 0x16:
		return "rsqrt", true
	case 0x20:
		return "cvt.s", true
	case 0x21:
		return "cvt.d", true
	case 0x24:
		return "cvt.w", true
	case 0x25:
		return "cvt.l", true
	}
	if funct >= 0x30 && funct <= 0x3f {
		return "c." + compareName(funct), true
	}
	return "", false
}

// compareName returns the comparison suffix of one c.cond.fmt form.
func compareName(funct uint32) string {
	names := [...]string{
		"f", "un", "eq", "ueq", "olt", "ult", "ole", "ule",
		"sf", "ngle", "seq", "ngl", "lt", "nge", "le", "ngt",
	}
	index := int(funct - 0x30)
	if index < 0 || index >= len(names) {
		return "f"
	}
	return names[index]
}

// coprocessor1Format returns the operand format letter of one CP1 form.
func coprocessor1Format(format uint32) string {
	switch format {
	case 0x10:
		return "s"
	case 0x11:
		return "d"
	case 0x14:
		return "w"
	case 0x15:
		return "l"
	default:
		return "s"
	}
}

// coprocessor1Branch maps the CP1 branch conditions.
func coprocessor1Branch(rt uint32) (string, bool) {
	switch rt {
	case 0x00:
		return "bc1f", true
	case 0x01:
		return "bc1t", true
	case 0x02:
		return "bc1fl", true
	case 0x03:
		return "bc1tl", true
	}
	return "", false
}

// branchAddress returns the absolute target of one PC-relative branch. MIPS
// counts from the delay slot, which follows the instruction.
func branchAddress(raw uint32, register int) int64 {
	offset := int64(int16(raw&0xffff)) << 2
	return offset
}

// jumpAddress returns the absolute target of one J or JAL.
func jumpAddress(raw uint32, region uint32) int64 {
	return int64(raw&0x03ffffff) << 2
}

// renderOperand formats one operand for the assembly text.
func renderOperand(item operand) string {
	switch item.kind {
	case operandRegister:
		return registerName(item.register)
	case operandImmediate:
		return fmt.Sprintf("%d", item.immediat)
	case operandMemory:
		return fmt.Sprintf("%d(%s)", item.immediat, registerName(item.base))
	default:
		return ""
	}
}

// registerName renders one MIPS register.
func registerName(index int) string {
	switch index {
	case 0:
		return "zero"
	case 1:
		return "at"
	case 2:
		return "v0"
	case 3:
		return "v1"
	case 4:
		return "a0"
	case 5:
		return "a1"
	case 6:
		return "a2"
	case 7:
		return "a3"
	case 8:
		return "t0"
	case 9:
		return "t1"
	case 10:
		return "t2"
	case 11:
		return "t3"
	case 12:
		return "t4"
	case 13:
		return "t5"
	case 14:
		return "t6"
	case 15:
		return "t7"
	case 16:
		return "s0"
	case 17:
		return "s1"
	case 18:
		return "s2"
	case 19:
		return "s3"
	case 20:
		return "s4"
	case 21:
		return "s5"
	case 22:
		return "s6"
	case 23:
		return "s7"
	case 24:
		return "t8"
	case 25:
		return "t9"
	case 26:
		return "k0"
	case 27:
		return "k1"
	case 28:
		return "gp"
	case 29:
		return "sp"
	case 30:
		return "fp"
	case 31:
		return "ra"
	default:
		return fmt.Sprintf("r%d", index)
	}
}

func sortAddresses(addresses []uint64) {
	sort.Slice(addresses, func(i, j int) bool { return addresses[i] < addresses[j] })
}

func symbolOperand(name string) core.Operand {
	return core.Operand{Kind: core.OperandSymbol, Name: name}
}
