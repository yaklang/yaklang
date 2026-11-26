package pass

import (
	"encoding/binary"
	"math"
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
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

// registerViewFor maps a machine register name to the variable a pass matches on.
// The alias rule comes from the register model, so a pass never switches on an
// architecture: x86 shares one variable between the 8, 16 and 32-bit views of a
// register, AArch64 shares one between Wn and Xn, and every other target names
// one register with one spelling.
func registerViewFor(facts core.ArchFacts, name string, bits uint16) registerView {
	model := facts.Registers
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return registerView{}
	}
	alias, ok := model.AliasOf(lower)
	if !ok {
		switch {
		case strings.HasPrefix(lower, "xmm"), strings.HasPrefix(lower, "ymm"), strings.HasPrefix(lower, "zmm"):
			width := bits
			if width == 0 {
				width = 128
			}
			return registerView{canonical: lower, width: width, canonicalBits: width}
		default:
			return registerView{canonical: lower, width: bits}
		}
	}
	width := bits
	if width == 0 || model.Naming == core.RegisterNamingAArch64 {
		// The name of an AArch64 alias fixes the width: Wn is always the low half
		// of Xn, whatever width the operand reports.
		width = alias.Width
	}
	canonicalBits := uint16(64)
	if model.Naming == core.RegisterNamingPlain {
		// Plain naming has no partial views, so one name is one whole variable.
		canonicalBits = 0
	}
	return registerView{canonical: alias.Name, width: width, shift: alias.Shift, canonicalBits: canonicalBits}
}

func literalUint64(literal []byte) uint64 {
	var buffer [8]byte
	copy(buffer[:], literal)
	return binary.LittleEndian.Uint64(buffer[:])
}
