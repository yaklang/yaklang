package loong64

import (
	"fmt"
	"sort"

	"golang.org/x/arch/loong64/loong64asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// registerName converts one register to the lower-case numeric naming shared
// with the other parsers. LoongArch has separate general and floating-point
// files, so the names carry the file as a prefix.
func registerName(reg loong64asm.Reg) (string, uint16, bool) {
	switch {
	case reg >= loong64asm.R0 && reg <= loong64asm.R31:
		return fmt.Sprintf("r%d", int(reg-loong64asm.R0)), 64, true
	case reg >= loong64asm.F0 && reg <= loong64asm.F31:
		return fmt.Sprintf("f%d", int(reg-loong64asm.F0)), 64, true
	default:
		return "", 0, false
	}
}

// isZeroRegister reports whether the register is r0, which always reads as zero.
func isZeroRegister(name string) bool {
	return name == "r0"
}

// isFloatRegister reports whether the name belongs to the floating-point file.
func isFloatRegister(name string) bool {
	return len(name) > 1 && name[0] == 'f'
}

// argRegister returns the register of one operand.
func argRegister(arg loong64asm.Arg) (string, uint16, bool) {
	reg, ok := arg.(loong64asm.Reg)
	if !ok {
		return "", 0, false
	}
	return registerName(reg)
}

// signedImmediate returns the value of one signed immediate operand.
func signedImmediate(arg loong64asm.Arg) (int64, bool) {
	switch value := arg.(type) {
	case loong64asm.Simm16:
		return int64(value.Imm), true
	case loong64asm.Simm32:
		return int64(value.Imm), true
	case loong64asm.Uimm:
		return int64(value.Imm), true
	case loong64asm.OffsetSimm:
		return int64(value.Imm), true
	case loong64asm.SaSimm:
		return int64(value), true
	case loong64asm.CodeSimm:
		return int64(value), true
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

// nameOf renders one opcode in lower case.
func nameOf(op loong64asm.Op) string {
	name := op.String()
	result := make([]byte, 0, len(name))
	for index := 0; index < len(name); index++ {
		current := name[index]
		if current >= 'A' && current <= 'Z' {
			current += 'a' - 'A'
		}
		result = append(result, current)
	}
	return string(result)
}

// args returns the operands of one instruction without the trailing nils.
func args(inst loong64asm.Inst) []loong64asm.Arg {
	result := make([]loong64asm.Arg, 0, 4)
	for _, arg := range inst.Args {
		if arg == nil {
			break
		}
		result = append(result, arg)
	}
	return result
}
