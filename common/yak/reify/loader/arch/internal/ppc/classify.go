package ppc

import (
	"strings"

	"golang.org/x/arch/ppc64/ppc64asm"
)

// The PowerPC opcode space is classified by mnemonic prefix and by the operand
// pattern rather than by bit fields, because the same mnemonic covers several
// widths.

func isLoad(name string) bool {
	for _, prefix := range []string{"lb", "lh", "lw", "ld", "lq", "lwa", "lwarx", "ldarx", "lharx", "lbarx", "lfi", "lf", "lx", "lv", "lxs", "lxd"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// normalize strips the record-form dot so "and." matches "and".
func normalize(name string) string {
	return strings.TrimSuffix(name, ".")
}

// isUnknownOpcode reports whether ppc64asm could not name the word. It renders
// those as "Op(n)".
func isUnknownOpcode(name string) bool {
	return len(name) >= 3 && name[:3] == "op("
}

func isStore(name string) bool {
	for _, prefix := range []string{"stb", "sth", "stw", "std", "stq", "stfi", "stf", "stx", "stv", "stwx", "stxs", "stxd"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func isBranch(name string) bool {
	name = normalize(name)
	switch name {
	case "b", "bl", "ba", "bla", "bc", "bcl", "bca", "bcla", "bdnz", "bdz", "bdnzl", "bdzl":
		return true
	}
	return strings.HasPrefix(name, "bc") || strings.HasPrefix(name, "bdn")
}

func isJump(name string) bool {
	switch name {
	case "b", "ba":
		return true
	}
	return false
}

// isIndirectBranch reports whether the branch target comes from a register.
func isIndirectBranch(name string) bool {
	switch name {
	case "bcctr", "bctr", "bclrl", "bcctrl":
		return true
	}
	return false
}

func isCall(op ppc64asm.Op) bool {
	name := normalize(opName(op))
	switch name {
	case "bl", "bla", "bcl", "bcla", "bctrl", "blrl", "bcctrl":
		return true
	}
	return false
}

// isReturn reports whether the instruction returns to the caller.
func isReturn(name string) bool {
	switch name {
	case "blr", "bclr", "blrl", "bclrl":
		return true
	}
	return false
}

func isTerminator(op ppc64asm.Op) bool {
	name := normalize(opName(op))
	if isUnknownOpcode(name) {
		return true
	}
	switch name {
	case "b", "ba", "bc", "bca", "blr", "bclr", "bctrl", "bcctrl", "bcctr",
		"bctr", "rfi", "trap", "tw", "td", "twi", "tdi":
		return true
	}
	return false
}

func isSystemLike(name string) bool {
	name = normalize(name)
	switch name {
	case "sync", "lwsync", "isync", "eieio", "sc", "nop", "dcbf", "dcbst",
		"dcbt", "dcbtst", "icbi", "dcbz", "tw", "td", "twi", "tdi",
		"crand", "cror", "crxor", "crnor", "creqv", "crandc", "crorc",
		"cmpb", "popcntb", "popcntw", "popcntd":
		return true
	}
	return false
}

// isVector reports whether the mnemonic belongs to the vector or floating-point
// space, which operates on lanes rather than integers.
func isVector(name string) bool {
	if len(name) < 2 {
		return false
	}
	switch name[0] {
	case 'f', 'v', 'x', 'm':
		return true
	}
	return false
}

// isVectorIndexed reports whether the mnemonic is a vector indexed load or
// store. Those use the X-form addressing with two registers and no offset.
func isVectorIndexed(name string) bool {
	for _, prefix := range []string{"lxvd", "stxvd", "lxsdx", "stxsdx", "lxvdsx", "lxssp", "lxv"} {
		if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// branchTarget returns the static target of a branch.
func branchTarget(ins instruction) (uint64, bool) {
	for _, arg := range ins.inst.Args {
		if arg == nil {
			break
		}
		if offset, ok := pcRelative(arg); ok {
			// PowerPC reports the displacement from the branch instruction.
			return ins.address + uint64(offset), true
		}
	}
	return 0, false
}
