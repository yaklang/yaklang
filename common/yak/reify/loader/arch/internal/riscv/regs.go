package riscv

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/arch/riscv64/riscv64asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// abiNames are the standard ABI names of the integer registers. LowIR uses the
// numeric spelling (x0..x31) so the text does not depend on the ABI.
func registerName(reg riscv64asm.Reg) (string, uint16, bool) {
	switch {
	case reg >= riscv64asm.X0 && reg <= riscv64asm.X31:
		return fmt.Sprintf("x%d", int(reg-riscv64asm.X0)), 0, true
	case reg >= riscv64asm.F0 && reg <= riscv64asm.F31:
		return fmt.Sprintf("f%d", int(reg-riscv64asm.F0)), 0, true
	default:
		return "", 0, false
	}
}

// isZeroRegister reports whether the register is x0, which always reads as zero.
func isZeroRegister(name string) bool {
	return name == "x0"
}

// isFloatRegister reports whether the name belongs to the floating-point file.
func isFloatRegister(name string) bool {
	return len(name) > 1 && name[0] == 'f'
}

// signedImmediate returns the value of a Simm operand.
func signedImmediate(arg riscv64asm.Arg) (int64, bool) {
	switch value := arg.(type) {
	case riscv64asm.Simm:
		return int64(value.Imm), true
	case riscv64asm.Uimm:
		return int64(value.Imm), true
	default:
		return 0, false
	}
}

// memoryOperand returns the base register and the displacement of a load or
// store operand. The AMO and LR/SC forms use AmoReg, which is a base register
// with no displacement; scalar loads and stores use RegOffset.
// upperImmediate returns the twenty-bit upper immediate as a signed value. LUI and
// AUIPC place it in the high half of a signed address, so 0xfffff means -1, not
// one million forty-eight thousand five hundred and seventy-five.
func upperImmediate(arg riscv64asm.Arg) (int64, bool) {
	value, ok := signedImmediate(arg)
	if !ok {
		return 0, false
	}
	if value&0x80000 != 0 {
		value -= 0x100000
	}
	return value, true
}

func memoryOperand(arg riscv64asm.Arg) (string, int64, bool) {
	switch value := arg.(type) {
	case riscv64asm.RegOffset:
		base, _, ok := registerName(value.OfsReg)
		if !ok {
			return "", 0, false
		}
		return base, int64(value.Ofs.Imm), true
	case riscv64asm.AmoReg:
		base, _, ok := registerName(amoRegister(value))
		if !ok {
			return "", 0, false
		}
		return base, 0, true
	default:
		return "", 0, false
	}
}

// amoRegister exposes the base register of an AmoReg operand. The field is
// unexported, so the printed form is the stable way to read it.
func amoRegister(value riscv64asm.AmoReg) riscv64asm.Reg {
	text := value.String()
	text = strings.TrimPrefix(text, "(")
	text = strings.TrimSuffix(text, ")")
	for reg := riscv64asm.X0; reg <= riscv64asm.F31; reg++ {
		if strings.EqualFold(reg.String(), text) {
			return reg
		}
	}
	return 0
}

func args(ins instruction) []riscv64asm.Arg {
	result := make([]riscv64asm.Arg, 0, 6)
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

// opName renders one opcode in lower case.
func opName(op riscv64asm.Op) string {
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
