package loong64

import (
	"strings"

	"golang.org/x/arch/loong64/loong64asm"
)

// LoongArch opcodes are a flat table with an explicit width and type suffix, so
// classification works on the mnemonic.

// isUnknownOpcode reports whether the decoder could not name the word.
func isUnknownOpcode(name string) bool {
	return strings.HasPrefix(name, "op(")
}

// isLoad reports whether the mnemonic reads memory.
func isLoad(name string) bool {
	switch name {
	case "ld.b", "ld.h", "ld.w", "ld.d", "ld.bu", "ld.hu", "ld.wu",
		"ldx.b", "ldx.h", "ldx.w", "ldx.d", "ldx.bu", "ldx.hu", "ldx.wu",
		"fld.s", "fld.d", "fldx.s", "fldx.d", "ll.w", "ll.d":
		return true
	}
	return false
}

// isStore reports whether the mnemonic writes memory.
func isStore(name string) bool {
	switch name {
	case "st.b", "st.h", "st.w", "st.d", "stx.b", "stx.h", "stx.w", "stx.d",
		"fst.s", "fst.d", "fstx.s", "fstx.d":
		return true
	}
	return false
}

// isBranch reports whether the mnemonic is a conditional branch.
func isBranch(name string) bool {
	switch trimmed(name) {
	case "beq", "bne", "blt", "bge", "bltu", "bgeu", "beqz", "bnez",
		"bltz", "bgez", "bgtz", "blez":
		return true
	}
	return false
}

// isJump reports whether the mnemonic transfers control unconditionally.
func isJump(name string) bool {
	switch trimmed(name) {
	case "b", "bl", "jirl", "jr", "ret", "bgt", "ble", "bgtu", "bleu":
		return true
	}
	return false
}

// isCall reports whether the mnemonic writes a return address.
func isCall(name string) bool {
	return trimmed(name) == "bl"
}

// isTerminator reports whether execution never falls through.
func isTerminator(name string) bool {
	switch trimmed(name) {
	case "jirl", "jr", "ret", "b", "bl", "break", "syscall", "idle":
		return true
	}
	return false
}

// isSystemLike reports whether the mnemonic is a system operation.
func isSystemLike(name string) bool {
	switch trimmed(name) {
	case "break", "syscall", "idle", "dbar", "ibar", "preld", "cpucfg",
		"rdtime.d", "rdtimeh.w", "rdtimel.w":
		return true
	}
	return false
}

// isFloat reports whether the mnemonic belongs to the floating-point or vector
// unit. Both have a register file with no data-flow model, so they stay opaque.
func isFloat(name string) bool {
	base := trimmed(name)
	if strings.HasPrefix(base, "v") || strings.HasPrefix(base, "xv") {
		return true
	}
	// The float-condition branches and the FCC transfers belong to the
	// floating-point file as well.
	switch base {
	case "bceqz", "bcnez", "movcf2gr", "movgr2cf", "movfcsr2gr", "movgr2fcsr":
		return true
	}
	if strings.HasPrefix(base, "f") {
		return true
	}
	return false
}

// isAtomic reports whether the mnemonic is an atomic memory operation.
func isAtomic(name string) bool {
	return strings.HasPrefix(trimmed(name), "am")
}

// trimmed removes the memory-ordering suffix so classification does not have to
// enumerate every combination.
func trimmed(name string) string {
	for _, suffix := range []string{".db", ".ib"} {
		if len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix {
			return name[:len(name)-len(suffix)]
		}
	}
	return name
}

// branchTarget returns the static target of a branch or jump. The offset is a
// byte count from the instruction address, encoded in the low sixteen bits of
// the immediate, which the library reports shifted left by sixteen.
func branchTarget(name string, operands []loong64asm.Arg, address uint64) (uint64, bool) {
	if !isBranch(name) && !isJump(name) {
		return 0, false
	}
	for _, arg := range operands {
		offset, ok := arg.(loong64asm.OffsetSimm)
		if !ok {
			continue
		}
		// The decoder reports the displacement in bytes, but not for every opcode:
		// some forms scale it by sixty-four kilobytes. A displacement whose low
		// sixteen bits are clear is the scaled form, and a real branch of that size
		// does not occur inside one function.
		value := int64(offset.Imm)
		if value != 0 && value&0xffff == 0 {
			value >>= 16
		}
		return address + uint64(value), true
	}
	return 0, false
}
