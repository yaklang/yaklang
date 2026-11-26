package riscv

import (
	"golang.org/x/arch/riscv64/riscv64asm"
)

// The RISC-V opcode space is organized by name prefix rather than by condition
// bits, so classification works on the mnemonic.

func isLoad(name string) bool {
	// The exclusive loads keep their width suffix after trimming.
	switch trimmed(name) {
	case "lb", "lh", "lw", "ld", "lbu", "lhu", "lwu", "flw", "fld", "lr", "lr.w", "lr.d":
		return true
	}
	return false
}

func isStore(name string) bool {
	switch trimmed(name) {
	case "sb", "sh", "sw", "sd", "fsw", "fsd", "sc", "sc.w", "sc.d":
		return true
	}
	return false
}

func isAtomic(name string) bool {
	base := trimmed(name)
	for _, prefix := range []string{"amoadd", "amoswap", "amoxor", "amoand", "amoor", "amomin", "amomax", "amominu", "amomaxu"} {
		if len(base) >= len(prefix) && base[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// trimmed removes the memory-ordering suffix (aq, rl, aqrl) so classification
// does not have to enumerate every combination. The width suffix is kept: it
// decides the access width and whether an opcode is a load or a store.
func trimmed(name string) string {
	base := name
	for _, suffix := range []string{".aqrl", ".aq", ".rl"} {
		if len(base) > len(suffix) && base[len(base)-len(suffix):] == suffix {
			return base[:len(base)-len(suffix)]
		}
	}
	return base
}

func isBranch(name string) bool {
	switch trimmed(name) {
	case "beq", "bne", "blt", "bge", "bltu", "bgeu",
		"beqz", "bnez", "blez", "bgez", "bltz", "bgtz":
		return true
	}
	return false
}

func isJump(name string) bool {
	switch name {
	case "jal", "jalr", "j", "jr", "call", "ret", "tail", "c.jal", "c.jalr", "c.j", "c.jr":
		return true
	}
	return false
}

func isCall(op riscv64asm.Op) bool {
	name := opName(op)
	return name == "jal" || name == "jalr" || name == "call"
}

func isTerminator(op riscv64asm.Op) bool {
	name := opName(op)
	switch name {
	case "jal", "jalr", "j", "jr", "ret", "ecall", "ebreak", "mret", "sret", "uret", "c.ebreak":
		return true
	}
	if isBranch(name) {
		return true
	}
	return false
}

func isSystemLike(name string) bool {
	switch name {
	case "ecall", "ebreak", "mret", "sret", "uret", "wfi", "fence", "fence.i", "fence.tso",
		"sfence.vma", "pause":
		return true
	}
	return false
}

func isFloat(name string) bool {
	if len(name) < 2 {
		return false
	}
	switch name[0] {
	case 'f', 'c':
		return true
	}
	return false
}

// branchTarget returns the static target of a branch or jump. A zero offset is
// a valid target (a self-loop), so it is not skipped. Only a PC-relative control
// transfer consults the relocation table: an auipc carries a relocation for the
// data address it materializes, and that address is not a branch target.
func branchTarget(ins instruction) (uint64, bool) {
	name := trimmed(opName(ins.inst.Op))
	control := isBranch(name) || name == "jal" || name == "j" || name == "c.jal" || name == "c.j"
	if !control {
		return 0, false
	}
	if ins.hasTarget {
		return ins.target, true
	}
	for _, arg := range ins.inst.Args {
		if arg == nil {
			break
		}
		if value, ok := signedImmediate(arg); ok {
			return ins.address + uint64(value), true
		}
	}
	return 0, false
}
