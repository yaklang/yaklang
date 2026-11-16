package mips

import "strings"

// MIPS classification uses the mnemonic, because the same encoding space covers
// integer, floating-point and coprocessor operations.

func isLoad(name string) bool {
	switch name {
	case "lb", "lh", "lwl", "lw", "lbu", "lhu", "lwr", "lwu", "ll", "lld",
		"ldl", "ldr", "ld", "lwc1", "ldc1", "lwc2", "ldc2", "luxc1":
		return true
	}
	return false
}

func isStore(name string) bool {
	switch name {
	case "sb", "sh", "swl", "sw", "sdl", "sdr", "swr", "sc", "scd",
		"sd", "swc1", "sdc1", "swc2", "sdc2", "suxc1":
		return true
	}
	return false
}

func isBranch(name string) bool {
	switch name {
	case "beq", "bne", "blez", "bgtz", "bltz", "bgez", "beql", "bnel",
		"blezl", "bgtzl", "bltzl", "bgezl", "bltzal", "bgezal",
		"bltzall", "bgezall", "bc1f", "bc1t", "bc1fl", "bc1tl",
		"bc2f", "bc2t":
		return true
	}
	return false
}

func isJump(name string) bool {
	switch name {
	case "j", "jal", "jalr", "jr":
		return true
	}
	return false
}

func isCall(name string) bool {
	return name == "jal" || name == "jalr" || name == "bal"
}

func isReturn(name string) bool {
	return name == "jr"
}

func isTerminator(name string) bool {
	switch name {
	case "j", "jr", "eret", "syscall", "break", "sdbbp":
		return true
	}
	return isBranch(name)
}

func isSystemLike(name string) bool {
	switch name {
	case "sync", "eret", "di", "ei", "cache", "pref", "nop", "ssnop", "ehb",
		"syscall", "break", "sdbbp", "teq", "tne", "tlt", "tltu", "tge", "tgeu":
		return true
	}
	return false
}

// isFloat reports whether the mnemonic belongs to the floating-point file.
func isFloat(name string) bool {
	switch name {
	case "mfc1", "mtc1", "cfc1", "ctc1", "dmfc1", "dmtc1",
		"lwc1", "swc1", "ldc1", "sdc1":
		return true
	}
	// Every other floating-point mnemonic carries a format suffix: add.s,
	// cvt.d.l, c.eq.s, bc1f. The dot is the reliable marker.
	return strings.Contains(name, ".")
}

// branchTarget returns the static target of a branch or jump. The decoded
// operand already holds the byte offset from the instruction.
func branchTarget(ins instruction) (uint64, bool) {
	if len(ins.operands) == 0 {
		return 0, false
	}
	switch ins.op {
	case "j", "jal":
		// The jump target is an absolute address region, not a PC offset.
		return uint64(ins.operands[len(ins.operands)-1].immediat), true
	case "bal":
		// BAL carries a PC-relative displacement like the other branches, and
		// the program counter of the delay slot is the base.
		return ins.address + 4 + uint64(ins.operands[len(ins.operands)-1].immediat), true
	}
	last := ins.operands[len(ins.operands)-1]
	if !isBranch(ins.op) {
		return 0, false
	}
	return ins.address + uint64(last.immediat), true
}
