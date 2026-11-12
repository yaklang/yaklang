package ppc

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/arch/ppc64/ppc64asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// registerName converts one register to the spelling the PowerPC manual uses.
// Registers keep their rN, fN, vN or crN form so the text matches the ISA
// documentation.
func registerName(reg ppc64asm.Reg) (string, uint16, bool) {
	switch {
	case reg >= ppc64asm.R0 && reg <= ppc64asm.R31:
		return fmt.Sprintf("r%d", int(reg-ppc64asm.R0)), 0, true
	case reg >= ppc64asm.F0 && reg <= ppc64asm.F31:
		return fmt.Sprintf("f%d", int(reg-ppc64asm.F0)), 0, true
	case reg >= ppc64asm.V0 && reg <= ppc64asm.V31:
		return fmt.Sprintf("v%d", int(reg-ppc64asm.V0)), 0, true
	default:
		return "", 0, false
	}
}

// isFloatRegister reports whether the name belongs to the floating-point file.
func isFloatRegister(name string) bool {
	return len(name) > 0 && name[0] == 'f'
}

// isVectorRegister reports whether the name belongs to the vector file.
func isVectorRegister(name string) bool {
	return len(name) > 0 && name[0] == 'v'
}

// signedImmediate returns the value of an immediate or offset operand.
func signedImmediate(arg ppc64asm.Arg) (int64, bool) {
	switch value := arg.(type) {
	case ppc64asm.Imm:
		return int64(value), true
	case ppc64asm.Offset:
		return int64(value), true
	default:
		return 0, false
	}
}

// pcRelative returns the target of a branch. PowerPC keeps the PC-relative
// displacement in a dedicated operand type.
func pcRelative(arg ppc64asm.Arg) (int64, bool) {
	switch value := arg.(type) {
	case ppc64asm.PCRel:
		return int64(value), true
	case ppc64asm.Label:
		return int64(value), true
	default:
		return 0, false
	}
}

func args(ins instruction) []ppc64asm.Arg {
	result := make([]ppc64asm.Arg, 0, 6)
	for _, arg := range ins.inst.Args {
		if arg == nil {
			break
		}
		result = append(result, arg)
	}
	return result
}

func sortAddresses(addresses []uint64) {
	sort.Slice(addresses, func(i, j int) bool { return addresses[i] < addresses[j] })
}

func symbolOperand(name string) core.Operand {
	return core.Operand{Kind: core.OperandSymbol, Name: name}
}

// opName renders one opcode. PowerPC mnemonics already use lower case except
// for a few dotted forms, so only the leading character is normalized.
func opName(op ppc64asm.Op) string {
	return strings.ToLower(op.String())
}
