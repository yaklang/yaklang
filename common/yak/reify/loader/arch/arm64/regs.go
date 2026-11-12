package arm64

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/arch/arm64/arm64asm"
)

// registerName converts one decoded register to the lower-case naming used by
// the other parsers, for example x0, w1, sp or v3.
func registerName(reg arm64asm.Reg) (string, uint16, bool) {
	switch {
	case reg >= arm64asm.W0 && reg <= arm64asm.W30:
		return fmt.Sprintf("w%d", int(reg-arm64asm.W0)), 32, true
	case reg == arm64asm.WZR:
		return "wzr", 32, true
	case reg >= arm64asm.X0 && reg <= arm64asm.X30:
		return fmt.Sprintf("x%d", int(reg-arm64asm.X0)), 64, true
	case reg == arm64asm.XZR:
		return "xzr", 64, true
	case reg >= arm64asm.B0 && reg <= arm64asm.B31:
		return fmt.Sprintf("b%d", int(reg-arm64asm.B0)), 8, true
	case reg >= arm64asm.H0 && reg <= arm64asm.H31:
		return fmt.Sprintf("h%d", int(reg-arm64asm.H0)), 16, true
	case reg >= arm64asm.S0 && reg <= arm64asm.S31:
		return fmt.Sprintf("s%d", int(reg-arm64asm.S0)), 32, true
	case reg >= arm64asm.D0 && reg <= arm64asm.D31:
		return fmt.Sprintf("d%d", int(reg-arm64asm.D0)), 64, true
	case reg >= arm64asm.Q0 && reg <= arm64asm.Q31:
		return fmt.Sprintf("q%d", int(reg-arm64asm.Q0)), 128, true
	case reg >= arm64asm.V0 && reg <= arm64asm.V31:
		return fmt.Sprintf("v%d", int(reg-arm64asm.V0)), 128, true
	default:
		return "", 0, false
	}
}

// registerSPName resolves WSP and SP, which share an encoding with WZR and XZR.
func registerSPName(reg arm64asm.RegSP) (string, uint16, bool) {
	switch arm64asm.Reg(reg) {
	case arm64asm.WZR:
		return "wsp", 32, true
	case arm64asm.XZR:
		return "sp", 64, true
	default:
		return registerName(arm64asm.Reg(reg))
	}
}

func isZeroRegister(name string) bool {
	return name == "wzr" || name == "xzr"
}

// argRegister returns the register and width of one register operand. It also
// accepts an extended-register operand, whose register is the field before the
// extension keyword.
func argRegister(arg arm64asm.Arg) (string, uint16, bool) {
	switch value := arg.(type) {
	case arm64asm.Reg:
		return registerName(value)
	case arm64asm.RegSP:
		return registerSPName(value)
	case arm64asm.RegExtshiftAmount:
		name, _, _ := strings.Cut(value.String(), ",")
		return registerNameText(strings.TrimSpace(name))
	case arm64asm.RegisterWithArrangement:
		return arrangementRegister(value.String())
	case arm64asm.RegisterWithArrangementAndIndex:
		return arrangementRegister(value.String())
	default:
		return "", 0, false
	}
}

// arrangementRegister reads the register part of a printed vector operand, for
// example V0.16B or V31.2D. Lane layouts are not modeled; the register keeps
// the full 128-bit width.
func arrangementRegister(text string) (string, uint16, bool) {
	head, _, _ := strings.Cut(text, ".")
	head = strings.ToLower(strings.TrimSpace(head))
	if head == "" {
		return "", 0, false
	}
	switch head[0] {
	case 'v', 'q':
		if _, err := strconv.Atoi(head[1:]); err != nil {
			return "", 0, false
		}
		return head, 128, true
	case 'd', 's', 'h', 'b':
		if _, err := strconv.Atoi(head[1:]); err != nil {
			return "", 0, false
		}
		return head, map[byte]uint16{'b': 8, 'h': 16, 's': 32, 'd': 64}[head[0]], true
	default:
		return "", 0, false
	}
}

// registerNameText converts a printed register name back into a name and width.
func registerNameText(text string) (string, uint16, bool) {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return "", 0, false
	}
	switch text {
	case "sp", "wsp":
		return text, map[bool]uint16{true: 32, false: 64}[text == "wsp"], true
	case "xzr", "wzr":
		return text, map[bool]uint16{true: 32, false: 64}[text == "wzr"], true
	}
	bits, ok := map[byte]uint16{'w': 32, 'x': 64, 'b': 8, 'h': 16, 's': 32, 'd': 64, 'q': 128, 'v': 128}[text[0]]
	if !ok {
		return "", 0, false
	}
	if _, err := strconv.Atoi(text[1:]); err != nil {
		return "", 0, false
	}
	return text, bits, true
}

// extensionOf returns the extension applied to one register operand. The
// encoding is one of the ZExt/Sext kinds below; Copy means no extension.
func extensionOf(arg arm64asm.Arg) (extension, uint8, bool) {
	value, ok := arg.(arm64asm.RegExtshiftAmount)
	if !ok {
		return extCopy, 0, true
	}
	text := value.String()
	if !strings.Contains(text, ",") {
		return extCopy, 0, true
	}
	fields := strings.Split(text, ",")
	kind := strings.TrimSpace(fields[1])
	name, amountText, hasAmount := strings.Cut(kind, "#")
	shift := uint8(0)
	if hasAmount {
		parsed, err := strconv.ParseUint(strings.TrimSpace(amountText), 10, 8)
		if err != nil {
			return extInvalid, 0, false
		}
		shift = uint8(parsed)
	}
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "UXTB":
		return extZero8, shift, true
	case "UXTH":
		return extZero16, shift, true
	case "UXTW":
		return extZero32, shift, true
	case "UXTX":
		return extCopy, shift, true
	case "SXTB":
		return extSign8, shift, true
	case "SXTH":
		return extSign16, shift, true
	case "SXTW":
		return extSign32, shift, true
	case "SXTX":
		return extCopy, shift, true
	case "LSL":
		return extShiftLeft, shift, true
	case "LSR":
		return extShiftRight, shift, true
	case "ASR":
		return extShiftArith, shift, true
	case "ROR":
		return extRotate, shift, true
	default:
		return extInvalid, 0, false
	}
}

type extension uint8

const (
	extInvalid extension = iota
	extCopy
	extZero8
	extZero16
	extZero32
	extSign8
	extSign16
	extSign32
	extShiftLeft
	extShiftRight
	extShiftArith
	extRotate
)

// immediateShift returns the immediate and the LSL amount of an ImmShift
// operand. arm64asm keeps both fields unexported, so its own rendering is the
// stable way to read them.
func immediateShift(arg arm64asm.Arg) (uint64, uint8, bool) {
	shift, ok := arg.(arm64asm.ImmShift)
	if !ok {
		return 0, 0, false
	}
	text := shift.String()
	body, rest, _ := strings.Cut(text, ",")
	value, err := parseNumber(body)
	if err != nil {
		return 0, 0, false
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return value, 0, true
	}
	keyword, number, _ := strings.Cut(rest, "#")
	if strings.TrimSpace(keyword) != "LSL" {
		return 0, 0, false
	}
	amount, err := strconv.ParseUint(strings.TrimSpace(number), 10, 8)
	if err != nil {
		return 0, 0, false
	}
	return value, uint8(amount), true
}

// immediateValue returns the value of an immediate operand.
func immediateValue(arg arm64asm.Arg) (uint64, bool) {
	switch value := arg.(type) {
	case arm64asm.Imm:
		return uint64(value.Imm), true
	case arm64asm.Imm64:
		return value.Imm, true
	case arm64asm.ImmShift:
		raw, _, ok := immediateShift(value)
		return raw, ok
	default:
		return 0, false
	}
}

func parseNumber(text string) (uint64, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "#")
	if value, err := strconv.ParseUint(text, 0, 64); err == nil {
		return value, nil
	}
	signed, err := strconv.ParseInt(text, 0, 64)
	if err != nil {
		return 0, err
	}
	return uint64(signed), nil
}

// memoryAddress returns the base register, the displacement and the addressing
// mode of one memory operand. arm64asm hides the displacement, so the printed
// form is parsed; the mode itself is exported.
func memoryAddress(arg arm64asm.Arg) (string, int64, arm64asm.AddrMode, bool) {
	switch value := arg.(type) {
	case arm64asm.MemImmediate:
		base, _, ok := registerSPName(value.Base)
		if !ok {
			return "", 0, 0, false
		}
		displacement, ok := memImmediateDisplacement(value)
		if !ok {
			return "", 0, 0, false
		}
		return base, displacement, value.Mode, true
	case arm64asm.MemExtend:
		base, _, ok := registerSPName(value.Base)
		if !ok {
			return "", 0, 0, false
		}
		return base, 0, arm64asm.AddrOffset, true
	default:
		return "", 0, 0, false
	}
}

func memImmediateDisplacement(memory arm64asm.MemImmediate) (int64, bool) {
	text := memory.String()
	open := strings.Index(text, "[")
	close := strings.Index(text, "]")
	if open < 0 || close < open {
		return 0, false
	}
	inside := text[open+1 : close]
	rest := strings.TrimSpace(text[close+1:])
	_, displacement, hasInside := strings.Cut(inside, ",")
	if memory.Mode == arm64asm.AddrPostIndex {
		displacement = strings.TrimPrefix(rest, ",")
		hasInside = displacement != ""
	}
	if !hasInside {
		return 0, true
	}
	number := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(displacement), "#"))
	result, err := strconv.ParseInt(number, 0, 64)
	if err != nil {
		return 0, false
	}
	return result, true
}

func pcRelative(arg arm64asm.Arg) (int64, bool) {
	value, ok := arg.(arm64asm.PCRel)
	return int64(value), ok
}

func conditionCode(arg arm64asm.Arg) (string, bool) {
	cond, ok := arg.(arm64asm.Cond)
	if !ok {
		return "", false
	}
	return strings.ToLower(cond.String()), true
}
