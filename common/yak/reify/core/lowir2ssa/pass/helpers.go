package pass

import (
	"encoding/binary"
	"math"
	"strings"
)

type registerView struct {
	canonical     string
	width         uint16
	shift         uint16
	canonicalBits uint16
}

func (v registerView) narrow() bool {
	return v.canonicalBits > 0 && v.width > 0 && v.width < v.canonicalBits
}

func (v registerView) partial() bool {
	return v.canonicalBits > 0 && v.width > 0 && v.width < 32
}

func (v registerView) mask() uint64 {
	width := v.width
	if width == 0 || width >= 64 {
		return math.MaxUint64
	}
	return (uint64(1) << width) - 1
}

// registerViewFor maps a machine register name to one SSA variable. Partial
// registers share the canonical 64-bit variable; their width and bit offset
// decide how a read is extracted or a write is merged.
func registerViewFor(name string, bits uint16) registerView {
	lower := strings.ToLower(name)
	if lower == "" {
		return registerView{}
	}
	if family, ok := generalRegisterFamily(lower); ok {
		width := bits
		if width == 0 {
			width = generalRegisterWidth(lower)
		}
		return registerView{
			canonical:     family,
			width:         width,
			shift:         generalRegisterShift(lower),
			canonicalBits: 64,
		}
	}
	switch {
	case strings.HasPrefix(lower, "xmm"), strings.HasPrefix(lower, "ymm"), strings.HasPrefix(lower, "zmm"):
		width := bits
		if width == 0 {
			width = 128
		}
		return registerView{canonical: lower, width: width, canonicalBits: width}
	case lower == "rflags", lower == "eflags", lower == "flags":
		width := bits
		if width == 0 {
			width = 64
		}
		return registerView{canonical: "rflags", width: width, canonicalBits: 64}
	default:
		return registerView{canonical: lower, width: bits}
	}
}

func generalRegisterFamily(name string) (string, bool) {
	switch name {
	case "al", "ah", "ax", "eax", "rax":
		return "rax", true
	case "bl", "bh", "bx", "ebx", "rbx":
		return "rbx", true
	case "cl", "ch", "cx", "ecx", "rcx":
		return "rcx", true
	case "dl", "dh", "dx", "edx", "rdx":
		return "rdx", true
	case "sil", "si", "esi", "rsi":
		return "rsi", true
	case "dil", "di", "edi", "rdi":
		return "rdi", true
	case "bpl", "bp", "ebp", "rbp":
		return "rbp", true
	case "spl", "sp", "esp", "rsp":
		return "rsp", true
	case "rip":
		return "rip", true
	}
	if len(name) < 2 || name[0] != 'r' || name[1] < '1' || name[1] > '9' {
		return aarch64RegisterFamily(name)
	}
	digits := name[1:]
	if last := digits[len(digits)-1]; len(digits) > 1 && (last == 'b' || last == 'w' || last == 'd' || last == 'l') {
		digits = digits[:len(digits)-1]
	}
	if digits == "" {
		return "", false
	}
	for _, digit := range digits {
		if digit < '0' || digit > '9' {
			return "", false
		}
	}
	return "r" + digits, true
}

func generalRegisterWidth(name string) uint16 {
	switch name {
	case "ah", "bh", "ch", "dh":
		return 8
	}
	// x86asm names the 32-bit extended registers r8l..r15l.
	if len(name) > 2 && name[0] == 'r' && name[1] >= '1' && name[1] <= '9' {
		switch name[len(name)-1] {
		case 'b':
			return 8
		case 'w':
			return 16
		case 'd', 'l':
			return 32
		}
	}
	switch {
	case strings.HasSuffix(name, "b"), strings.HasSuffix(name, "l"):
		return 8
	case strings.HasSuffix(name, "w"):
		return 16
	case strings.HasSuffix(name, "d"):
		return 32
	default:
		return 64
	}
}

func generalRegisterShift(name string) uint16 {
	switch name {
	case "ah", "bh", "ch", "dh":
		return 8
	default:
		return 0
	}
}

// aarch64RegisterFamily maps a 32-bit view of an AArch64 general register to its
// 64-bit canonical name, so w0 and x0 compare as the same register. Zero
// registers (wzr/xzr) are excluded: writing them discards the value and must
// never count as a return-register write.
func aarch64RegisterFamily(name string) (string, bool) {
	if len(name) < 2 {
		return "", false
	}
	prefix, digits := name[0], name[1:]
	if prefix != 'w' && prefix != 'x' {
		return "", false
	}
	if digits == "zr" || digits == "" {
		return "", false
	}
	for _, digit := range digits {
		if digit < '0' || digit > '9' {
			return "", false
		}
	}
	return "x" + digits, true
}

func literalUint64(literal []byte) uint64 {
	var buffer [8]byte
	copy(buffer[:], literal)
	return binary.LittleEndian.Uint64(buffer[:])
}
