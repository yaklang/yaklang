package arm

import (
	"fmt"
	"strings"

	"golang.org/x/arch/arm/armasm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// registerName converts one register to the lower-case naming shared with the
// other parsers, for example r0, sp, lr or s3.
func registerName(reg armasm.Reg) (string, uint16, bool) {
	switch {
	case reg >= armasm.R0 && reg <= armasm.R12:
		return fmt.Sprintf("r%d", int(reg-armasm.R0)), 32, true
	case reg == armasm.R13:
		return "sp", 32, true
	case reg == armasm.R14:
		return "lr", 32, true
	case reg == armasm.R15:
		return "pc", 32, true
	case reg >= armasm.S0 && reg <= armasm.S31:
		return fmt.Sprintf("s%d", int(reg-armasm.S0)), 32, true
	case reg == armasm.APSR:
		return "cpsr", 32, true
	case reg == armasm.APSR_nzcv:
		return "cpsr", 32, true
	case reg == armasm.FPSCR:
		return "fpscr", 32, true
	default:
		return "", 0, false
	}
}

// argRegister returns the register name and width of one register operand.
func argRegister(arg armasm.Arg) (string, uint16, bool) {
	switch value := arg.(type) {
	case armasm.Reg:
		return registerName(value)
	case armasm.RegShift:
		return registerName(value.Reg)
	case armasm.RegShiftReg:
		return registerName(value.Reg)
	case armasm.RegX:
		return registerName(value.Reg)
	default:
		return "", 0, false
	}
}

// registerList returns the registers of a PUSH/POP/LDM/STM operand in order.
func registerList(list armasm.RegList) []string {
	result := make([]string, 0, 16)
	for index := 0; index < 16; index++ {
		if list&(1<<uint(index)) == 0 {
			continue
		}
		name, _, ok := registerName(armasm.Reg(index))
		if ok {
			result = append(result, name)
		}
	}
	return result
}

// immediateValue returns the value of an immediate operand. armasm already
// applies the ARM rotated-immediate encoding to Imm.
func immediateValue(arg armasm.Arg) (uint64, bool) {
	switch value := arg.(type) {
	case armasm.Imm:
		return uint64(value), true
	case armasm.ImmAlt:
		return uint64(value.Val), true
	default:
		return 0, false
	}
}

// shiftedRegister returns the register, the shift kind and the shift amount of
// a shift operand. An empty shift kind means the operand is not shifted.
func shiftedRegister(arg armasm.Arg) (string, string, uint64, bool) {
	switch value := arg.(type) {
	case armasm.Reg:
		name, _, ok := registerName(value)
		return name, "", 0, ok
	case armasm.RegShift:
		name, _, ok := registerName(value.Reg)
		if !ok {
			return "", "", 0, false
		}
		return name, shiftName(value.Shift), uint64(value.Count), true
	case armasm.RegShiftReg:
		name, _, ok := registerName(value.Reg)
		if !ok {
			return "", "", 0, false
		}
		return name, shiftName(value.Shift), 0, true
	default:
		return "", "", 0, false
	}
}

func shiftName(shift armasm.Shift) string {
	switch shift {
	case armasm.ShiftLeft:
		return "lsl"
	case armasm.ShiftRight:
		return "lsr"
	case armasm.ShiftRightSigned:
		return "asr"
	case armasm.RotateRight:
		return "ror"
	case armasm.RotateRightExt:
		return "rrx"
	default:
		return ""
	}
}

// pcRelative returns the branch offset. armasm encodes PCRel relative to the
// instruction address plus eight, which is the ARM program-counter value.
func pcRelative(arg armasm.Arg) (int64, bool) {
	value, ok := arg.(armasm.PCRel)
	return int64(value), ok
}

// memoryAddress describes one memory operand. A zero index means the operand is
// offset-only; otherwise the index register is scaled by two to the shift.
func memoryAddress(arg armasm.Arg) (base, index string, shift uint8, offset int64, memory armasm.Mem, ok bool) {
	memory, isMemory := arg.(armasm.Mem)
	if !isMemory {
		return "", "", 0, 0, armasm.Mem{}, false
	}
	base, _, nameOK := registerName(memory.Base)
	if !nameOK {
		return "", "", 0, 0, armasm.Mem{}, false
	}
	if memory.Index != 0 && memory.Sign != 0 {
		index, _, _ = registerName(memory.Index)
		shift = memory.Count
	}
	return base, index, shift, int64(memory.Offset), memory, true
}

// isSignedIndex reports whether the index register is subtracted.
func isSignedIndex(memory armasm.Mem) bool {
	return memory.Sign < 0
}

func symbolOperand(name string) core.Operand {
	return core.Operand{Kind: core.OperandSymbol, Name: name}
}

func lower(text string) string { return strings.ToLower(text) }
