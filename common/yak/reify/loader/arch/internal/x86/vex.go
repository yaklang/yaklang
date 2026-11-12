package x86

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// vectorEncoding describes one VEX or EVEX encoded instruction.
//
// golang.org/x/arch treats 0xc4, 0xc5 and 0x62 as legacy opcodes. It returns a
// different instruction with a different length instead of an error, which
// desynchronizes the rest of the function. Decoding the prefix here keeps the
// instruction stream aligned and reports the operation as an opaque vector
// instruction.
type vectorEncoding struct {
	size    int
	name    string
	width   uint16
	dest    string
	sources []string
	memory  bool
	store   bool
}

// vectorPrefix reports whether the bytes start a VEX or EVEX instruction. In
// 32-bit mode the legacy LES, LDS and BOUND opcodes share these lead bytes, so
// the prefix only applies when the ModRM byte selects a register operand.
func vectorPrefix(mode int, code []byte) bool {
	if len(code) < 2 {
		return false
	}
	switch code[0] {
	case 0xc4, 0xc5, 0x62:
	default:
		return false
	}
	if mode == 64 {
		return true
	}
	return code[1]&0xc0 == 0xc0
}

// decodeVector decodes one VEX or EVEX instruction.
func decodeVector(mode int, code []byte, address uint64) (instruction, error) {
	encoding, err := parseVector(mode, code)
	if err != nil {
		return instruction{}, err
	}
	return instruction{address: address, size: encoding.size, vector: encoding}, nil
}

// parseVector decodes the prefix, the opcode and the ModRM byte of one vector
// instruction. The operand semantics stay unmodeled; the width and the register
// names are enough to keep the data flow visible.
func parseVector(mode int, code []byte) (*vectorEncoding, error) {
	var (
		prefix int
		mapID  int
		pp     int
		w      bool
		vvvv   int
		width  uint16
		regExt int
		rmExt  int
		evex   bool
	)
	switch code[0] {
	case 0xc5:
		if len(code) < 2 {
			return nil, fmt.Errorf("truncated VEX prefix")
		}
		prefix = 2
		mapID = 1
		pp = int(code[1] & 0x03)
		vvvv = int(^code[1] >> 3 & 0x0f)
		width = 128
		if code[1]&0x04 != 0 {
			width = 256
		}
		if code[1]&0x80 == 0 {
			regExt = 8
		}
	case 0xc4:
		if len(code) < 3 {
			return nil, fmt.Errorf("truncated VEX prefix")
		}
		prefix = 3
		mapID = int(code[1] & 0x1f)
		pp = int(code[2] & 0x03)
		vvvv = int(^code[2] >> 3 & 0x0f)
		w = code[2]&0x80 != 0
		width = 128
		if code[2]&0x04 != 0 {
			width = 256
		}
		if code[1]&0x80 == 0 {
			regExt = 8
		}
		if code[1]&0x20 == 0 {
			rmExt = 8
		}
	case 0x62:
		if len(code) < 4 {
			return nil, fmt.Errorf("truncated EVEX prefix")
		}
		if mode != 64 {
			return nil, fmt.Errorf("EVEX prefix outside 64-bit mode")
		}
		prefix = 4
		evex = true
		mapID = int(code[1] & 0x07)
		pp = int(code[2] & 0x03)
		w = code[2]&0x80 != 0
		vvvv = int(^code[2] >> 3 & 0x0f)
		if code[2]&0x04 == 0 {
			return nil, fmt.Errorf("EVEX prefix without the fixed bit")
		}
		if code[1]&0x80 == 0 {
			regExt = 8
		}
		if code[1]&0x40 == 0 {
			regExt |= 16
		}
		if code[1]&0x20 == 0 {
			rmExt = 8
		}
		if code[3]&0x08 == 0 {
			vvvv |= 16
		}
		// P2 holds z, L'L, b, V' and aaa; L'L is the vector length.
		switch (code[3] >> 5) & 0x03 {
		case 0:
			width = 128
		case 1:
			width = 256
		default:
			width = 512
		}
	default:
		return nil, fmt.Errorf("not a vector instruction")
	}
	pos := prefix
	if pos >= len(code) {
		return nil, fmt.Errorf("truncated vector instruction")
	}
	opcode := code[pos]
	pos++
	encoding := &vectorEncoding{width: width, name: vectorName(mapID, int(opcode), pp, w, evex)}
	if mapID == 1 && opcode == 0x77 {
		// VZEROUPPER and VZEROALL carry no ModRM byte.
		encoding.size = pos
		return encoding, nil
	}
	if pos >= len(code) {
		return nil, fmt.Errorf("truncated vector instruction")
	}
	modrm := code[pos]
	pos++
	mod := int(modrm >> 6)
	reg := int(modrm>>3&0x07) | regExt
	rm := int(modrm&0x07) | rmExt
	if mod != 3 && modrm&0x07 == 4 {
		if pos >= len(code) {
			return nil, fmt.Errorf("truncated SIB byte")
		}
		base := int(code[pos] & 0x07)
		pos++
		if mod == 0 && base == 5 {
			pos += 4
		}
	}
	switch mod {
	case 0:
		if modrm&0x07 == 5 {
			pos += 4
		}
	case 1:
		pos++
	case 2:
		pos += 4
	}
	if hasImmediate8(mapID, opcode) {
		pos++
	}
	if pos > len(code) {
		return nil, fmt.Errorf("truncated vector instruction")
	}
	encoding.size = pos
	encoding.store = vectorStore(mapID, opcode)
	encoding.memory = mod != 3
	encoding.dest = vectorRegisterName(reg, width)
	if encoding.store {
		// The store forms read ModRM.reg instead of writing it.
		encoding.sources = append(encoding.sources, encoding.dest)
	}
	if vvvv != 0 {
		encoding.sources = append(encoding.sources, vectorRegisterName(vvvv, width))
	}
	if mod == 3 {
		encoding.sources = append(encoding.sources, vectorRegisterName(rm, width))
	}
	return encoding, nil
}

// hasImmediate8 reports whether one vector opcode carries an imm8 operand.
func hasImmediate8(mapID int, opcode byte) bool {
	if mapID == 3 {
		return true
	}
	if mapID != 1 {
		return false
	}
	switch opcode {
	case 0x70, 0x71, 0x72, 0x73, 0xc2, 0xc4, 0xc5, 0xc6:
		return true
	}
	return false
}

// vectorStore reports whether the ModRM register operand is a source, not a
// destination: the store forms write memory.
func vectorStore(mapID int, opcode byte) bool {
	if mapID != 1 {
		return false
	}
	switch opcode {
	case 0x11, 0x13, 0x17, 0x29, 0x2b, 0x7f, 0xe7:
		return true
	}
	return false
}

// vectorName returns the mnemonic of one vector opcode. Only the opcodes that
// compiled binaries use carry a name; everything else keeps the map and the
// opcode so the instruction stays traceable.
func vectorName(mapID int, opcode, pp int, w bool, evex bool) string {
	if mapID == 1 && opcode == 0x77 && pp == 0 {
		if w {
			return "vzeroall"
		}
		return "vzeroupper"
	}
	if opcode == 0x6f || opcode == 0x7f {
		// The mandatory prefix selects the aligned form. EVEX splits the forms
		// further by element size, which the W bit records.
		name := "vmovdqu"
		if pp == 1 {
			name = "vmovdqa"
		}
		if evex {
			name += "64"
		}
		if opcode == 0x7f {
			name += ".store"
		}
		return name
	}
	return fmt.Sprintf("vex.%d.%02x", mapID, opcode)
}

// vectorRegisterName returns the vector register name of one register index.
func vectorRegisterName(index int, width uint16) string {
	prefix := "xmm"
	switch width {
	case 256:
		prefix = "ymm"
	case 512:
		prefix = "zmm"
	}
	return fmt.Sprintf("%s%d", prefix, index)
}

// liftVector lifts one VEX or EVEX instruction as an opaque vector operation.
func (l *lifter) liftVector(ins instruction) {
	encoding := ins.vector
	operands := []core.Operand{symbolOperand("x86." + encoding.name)}
	var results []core.Value
	effects := core.EffectUnknown
	if encoding.dest != "" && !encoding.store {
		value := l.newValue(core.TypeInteger, encoding.width)
		results = append(results, value)
		operands = append(operands, core.Operand{Kind: core.OperandRegister, Type: value.Type, Name: encoding.dest})
		effects |= core.EffectWriteRegister
	}
	for _, name := range encoding.sources {
		operands = append(operands, core.Operand{
			Kind: core.OperandRegister,
			Type: core.ValueType{Kind: core.TypeInteger, Bits: encoding.width},
			Name: name,
		})
		effects |= core.EffectReadRegister
	}
	if encoding.memory {
		effects |= core.EffectReadMemory
		if encoding.store {
			effects |= core.EffectWriteMemory
		}
	}
	l.emit(core.OpIntrinsic, operands, results, nil, effects, ins.address)
}

// vectorText renders one VEX or EVEX instruction for the disassembly view.
func vectorText(encoding *vectorEncoding) string {
	parts := make([]string, 0, len(encoding.sources)+2)
	if encoding.dest != "" && !encoding.store {
		parts = append(parts, encoding.dest)
	}
	parts = append(parts, encoding.sources...)
	if encoding.memory {
		parts = append(parts, "[memory]")
	}
	if len(parts) == 0 {
		return encoding.name
	}
	return encoding.name + " " + strings.Join(parts, ", ")
}
