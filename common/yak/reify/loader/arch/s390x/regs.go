package s390x

import (
	"fmt"
	"sort"

	"golang.org/x/arch/s390x/s390xasm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// registerName converts one register to the lower-case naming shared with the
// other parsers. s390x has separate general, floating-point, access and control
// register files, so the names carry the file as a prefix.
func registerName(reg s390xasm.Reg) (string, uint16, bool) {
	switch {
	case reg >= s390xasm.R0 && reg <= s390xasm.R15:
		return fmt.Sprintf("r%d", int(reg-s390xasm.R0)), 64, true
	case reg >= s390xasm.F0 && reg <= s390xasm.F15:
		return fmt.Sprintf("f%d", int(reg-s390xasm.F0)), 64, true
	case reg >= s390xasm.A0 && reg <= s390xasm.A15:
		return fmt.Sprintf("a%d", int(reg-s390xasm.A0)), 32, true
	case reg >= s390xasm.C0 && reg <= s390xasm.C15:
		return fmt.Sprintf("c%d", int(reg-s390xasm.C0)), 32, true
	default:
		return "", 0, false
	}
}

// vectorRegisterName converts one vector register.
func vectorRegisterName(reg s390xasm.VReg) (string, uint16, bool) {
	if reg < s390xasm.V0 || reg > s390xasm.V31 {
		return "", 0, false
	}
	return fmt.Sprintf("v%d", int(reg-s390xasm.V0)), 128, true
}

// baseRegisterName converts the base field of a memory operand. B0 means the
// operand has no base register.
func baseRegisterName(base s390xasm.Base) (string, bool) {
	if base == s390xasm.B0 {
		return "", false
	}
	return fmt.Sprintf("r%d", int(base-s390xasm.B0)), true
}

// indexRegisterName converts the index field of a memory operand. X0 means the
// operand has no index register.
func indexRegisterName(index s390xasm.Index) (string, bool) {
	if index == s390xasm.X0 {
		return "", false
	}
	return fmt.Sprintf("r%d", int(index-s390xasm.X0)), true
}

// memoryOperand reports whether a run of arguments forms a memory operand and
// returns its base, index and displacement. s390x writes the displacement
// first, then the index and the base.
func memoryOperand(args []s390xasm.Arg) (base, index string, displacement int64, ok bool) {
	for position, arg := range args {
		switch value := arg.(type) {
		case s390xasm.Disp20:
			displacement = int64(int32(value<<12) >> 12)
		case s390xasm.Disp12:
			displacement = int64(value)
		default:
			continue
		}
		// The index and base follow the displacement.
		rest := args[position+1:]
		if len(rest) > 0 {
			if register, isIndex := rest[0].(s390xasm.Index); isIndex {
				index, _ = indexRegisterName(register)
				rest = rest[1:]
			}
		}
		if len(rest) > 0 {
			if register, isBase := rest[0].(s390xasm.Base); isBase {
				base, _ = baseRegisterName(register)
			}
		}
		if base == "" {
			// A displacement with no base register reads absolute memory,
			// which the low address space makes unusual but not invalid.
			base = "zero"
		}
		return base, index, displacement, true
	}
	return "", "", 0, false
}

// bitsOf returns the width of one register file operand.
func bitsOf(arg s390xasm.Arg) (uint16, bool) {
	switch value := arg.(type) {
	case s390xasm.Reg:
		_, bits, ok := registerName(value)
		return bits, ok
	case s390xasm.VReg:
		_, bits, ok := vectorRegisterName(value)
		return bits, ok
	default:
		return 0, false
	}
}

// sortAddresses orders addresses ascending.
func sortAddresses(addresses []uint64) {
	sort.Slice(addresses, func(i, j int) bool { return addresses[i] < addresses[j] })
}

// symbolOperand builds one intrinsic name operand.
func symbolOperand(name string) core.Operand {
	return core.Operand{Kind: core.OperandSymbol, Name: name}
}
