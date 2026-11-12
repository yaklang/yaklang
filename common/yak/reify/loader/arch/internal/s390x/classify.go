package s390x

import (
	"strings"

	"golang.org/x/arch/s390x/s390xasm"
)

// The s390x opcode space is a flat mnemonic table with no condition bits, so
// classification works on the name and on the operand shape. A memory operand
// is the reliable signal for a load or a store: the register-only forms share
// the same prefixes.

// normalize lower-cases one mnemonic.
func normalize(name string) string {
	return strings.ToLower(name)
}

// nameOf renders one opcode in lower case.
func nameOf(op s390xasm.Op) string {
	return strings.ToLower(op.String())
}

// isUnknownOpcode reports whether the decoder could not name the word. It
// renders those as "Op(n)".
func isUnknownOpcode(name string) bool {
	return strings.HasPrefix(name, "op(")
}

// hasMemoryOperand reports whether one decoded instruction reads or writes
// memory.
func hasMemoryOperand(inst s390xasm.Inst) bool {
	_, _, _, ok := memoryOperand(args(inst))
	return ok
}

// isLoad reports whether the mnemonic reads memory. The z/Architecture load
// family starts with l; the store family with st.
func isLoad(name string) bool {
	return len(name) > 0 && name[0] == 'l'
}

// isStore reports whether the mnemonic writes memory.
func isStore(name string) bool {
	return len(name) > 1 && name[0] == 's' && name[1] == 't'
}

// isCall reports whether the mnemonic is a call. s390x has no separate call
// instruction: the branch-and-save forms write a return address into a
// register, and brasl is the form the compilers emit.
func isCall(name string) bool {
	switch name {
	case "brasl", "bras", "basr", "balr", "bassm", "bsa", "bas", "bal", "bcr":
		return true
	}
	return false
}

// isBranch reports whether the mnemonic is a conditional branch. The operand
// that carries the condition mask decides whether the branch is direct.
func isBranch(name string) bool {
	switch name {
	case "brc", "brcl", "bc", "j", "jg", "je", "jne", "jh", "jhe", "jlh",
		"jle", "jo", "jno", "jp", "jnp", "jm", "jnm", "jnz", "jz", "jge",
		"jl", "jgl", "jc", "jnc":
		return true
	}
	return false
}

// isRegisterBranch reports whether the mnemonic branches through a register.
func isRegisterBranch(name string) bool {
	switch name {
	case "br", "bcr", "brx", "b", "bxh", "bxhg", "bxle", "bxleg":
		return true
	}
	return false
}

// isTerminator reports whether execution never falls through.
func isTerminator(name string) bool {
	switch name {
	case "br", "brc", "brcl", "bcr", "bc", "svc", "trap2", "trap4",
		"stosm", "ssm", "stnsm", "lpsw", "lpswe":
		return true
	}
	return false
}

// isSystemLike reports whether the mnemonic is a system or serialization
// operation that has no data-flow model.
func isSystemLike(name string) bool {
	switch name {
	case "svc", "trap2", "trap4", "nop", "nopr", "eieio", "ipm",
		"stck", "stcke", "stfle", "srst", "stosm", "ssm", "stnsm":
		return true
	}
	return false
}

// isFloat reports whether the mnemonic belongs to the floating-point unit. The
// z/Architecture puts the type letter before the register suffix, for example
// adbr, ldeb or msdbr.
func isFloat(name string) bool {
	if len(name) < 3 {
		return false
	}
	return strings.HasSuffix(name, "br") || strings.HasSuffix(name, "dr") ||
		strings.HasSuffix(name, "er") || strings.HasSuffix(name, "fr") ||
		strings.HasSuffix(name, "xbr") || strings.HasSuffix(name, "axbr")
}

// isVector reports whether the mnemonic belongs to the vector facility.
func isVector(name string) bool {
	return len(name) > 1 && name[0] == 'v'
}

// isConditionalBranch reports whether the decoded instruction carries a mask
// operand that selects a condition.
func isConditionalBranch(inst s390xasm.Inst) bool {
	if !isBranch(nameOf(inst.Op)) {
		return false
	}
	for _, arg := range inst.Args {
		if arg == nil {
			break
		}
		if _, ok := arg.(s390xasm.Mask); ok {
			return true
		}
	}
	return false
}

// branchTarget returns the static target of a branch. The immediate field holds
// a halfword count relative to the instruction address. The branch-and-save, the
// compare-and-branch and the load-address forms all share the encoding, so the
// operand type decides and the mnemonic does not.
func branchTarget(inst s390xasm.Inst, address uint64, size int) (uint64, bool) {
	return pcRelativeTarget(inst, address)
}

// pcRelativeTarget returns the PC-relative target of one instruction.
func pcRelativeTarget(inst s390xasm.Inst, address uint64) (uint64, bool) {
	for _, arg := range inst.Args {
		if arg == nil {
			break
		}
		switch value := arg.(type) {
		case s390xasm.RegIm16:
			return address + 2*uint64(int16(value)), true
		case s390xasm.RegIm32:
			return address + 2*uint64(int32(value)), true
		case s390xasm.RegIm12:
			return address + 2*uint64(int16(value)), true
		case s390xasm.RegIm24:
			return address + 2*uint64(int32(value)), true
		}
	}
	return 0, false
}
