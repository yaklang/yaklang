package arm

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"golang.org/x/arch/arm/armasm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/loader/arch/internal/diag"
)

// Thumb uses a separate, denser encoding: most instructions are one halfword
// and the rest are two. The ARM decoder in golang.org/x/arch rejects Thumb
// outright, so the encodings are decoded here into the same armasm.Inst values
// the ARM lifter already handles.
//
// Thumb has no condition field per instruction. Instead an IT instruction makes
// the following one to four instructions conditional, so decode carries a
// thumbState that rewrites the condition bits of each instruction after an IT.

// thumbState tracks the condition that the next instruction runs under.
type thumbState struct {
	// cond is the condition code of the current IT block, 0..14.
	cond int
	// mask is the IT block mask. Zero means no IT block is active.
	mask int
	// index is the zero-based position inside the block.
	index int
	// length is the number of instructions in the block.
	length int
}

// next returns the condition code for the current instruction and advances the
// IT state.
//
// The mask encodes the block: the lowest set bit is the last instruction, and
// every bit above it belongs to one earlier instruction. A set bit keeps the
// base condition, a clear bit selects its inverse. The first instruction always
// uses the base condition.
func (s *thumbState) next() int {
	if s.length == 0 {
		return 14
	}
	cond := s.cond
	if s.index > 0 && s.mask&(1<<uint(4-s.index)) != 0 {
		cond ^= 1
	}
	s.index++
	if s.index >= s.length {
		s.length = 0
		s.index = 0
	}
	return cond
}

// start records the IT block that the current IT instruction opens.
func (s *thumbState) start(hw uint16) {
	s.cond = int(hw>>4) & 0x0f
	s.mask = int(hw & 0x0f)
	s.index = 0
	s.length = 4 - trailingZeros(s.mask)
}

// trailingZeros counts the zero bits below the lowest set bit.
func trailingZeros(value int) int {
	count := 0
	for value&1 == 0 {
		value >>= 1
		count++
	}
	return count
}

// itSuffix renders the T and E letters of one IT block. The ARM assembler
// writes the first letter as part of the "it" mnemonic, so the returned text
// starts after it.
func itSuffix(cond, mask int) string {
	length := 4 - trailingZeros(mask)
	text := make([]byte, 0, 4)
	for index := 0; index < length; index++ {
		if index == 0 || mask&(1<<uint(4-index)) == 0 {
			text = append(text, 't')
			continue
		}
		text = append(text, 'e')
	}
	// The mnemonic is "it" plus the remaining letters.
	if len(text) == 1 {
		return " " + conditionNames[cond&0x0f]
	}
	return string(text[1:]) + " " + conditionNames[cond&0x0f]
}

// compareBranch is CBZ or CBNZ: a register test against zero fused with a
// branch. armasm has no opcode for either, so the pair is modeled here.
type compareBranch struct {
	mnemonic string
	register uint32
	target   int64
}

// thumbInstruction is one decoded Thumb instruction.
type thumbInstruction struct {
	inst armasm.Inst
	size int
	// text overrides the rendering when armasm has no opcode for the form.
	text string
	// compare marks CBZ and CBNZ.
	compare *compareBranch
	// opaque names an instruction the armasm opcode set cannot express. The
	// lifter emits it as an intrinsic with the same name.
	opaque string
	// opaqueArgs holds the arguments of an opaque instruction, in order.
	opaqueArgs []armasm.Arg
	// undefined marks a halfword that is not a valid Thumb instruction.
	undefined bool
}

// decodeThumb decodes one Thumb instruction and reports its size in bytes.
func decodeThumb(code []byte, state *thumbState) (thumbInstruction, int) {
	first := binary.LittleEndian.Uint16(code)
	// The three 32-bit prefixes leave the condition untouched: Thumb-2 wide
	// instructions are unconditional.
	if first>>11 == 0x1d || first>>11 == 0x1e || first>>11 == 0x1f {
		if len(code) < 4 {
			return thumbInstruction{undefined: true}, 2
		}
		second := binary.LittleEndian.Uint16(code[2:])
		decoded, ok := decodeThumb32(first, second)
		if !ok {
			return thumbInstruction{undefined: true}, 4
		}
		if decoded.size == 0 {
			decoded.size = 4
		}
		return decoded, 4
	}
	cond := state.next()
	if first>>8 == 0xbf && first&0x000f != 0 {
		// IT opens a block. It has no operand of its own.
		text := "it" + itSuffix(int(first>>4)&0x0f, int(first&0x0f))
		state.start(first)
		return thumbInstruction{inst: armasm.Inst{Op: armasm.NOP}, size: 2, text: text}, 2
	}
	decoded, ok := decodeThumb16(first, cond)
	if !ok {
		return thumbInstruction{undefined: true}, 2
	}
	decoded.size = 2
	return decoded, 2
}

// applyCondition rewrites the condition bits of one opcode. armasm numbers each
// base form as sixteen consecutive entries, one per condition.
func applyCondition(op armasm.Op, cond int) armasm.Op {
	return op&^15 | armasm.Op(cond)
}

// set builds one decoded instruction. The variadic form keeps the decode tables
// readable.
func set(cond int, op armasm.Op, args ...armasm.Arg) thumbInstruction {
	var list armasm.Args
	copy(list[:], args)
	return thumbInstruction{inst: armasm.Inst{Op: applyCondition(op, cond), Args: list}}
}

// registerText renders one register index in the shared lower-case form.
func registerText(index uint32) string {
	name, _, ok := registerName(armasm.Reg(index & 0x0f))
	if !ok {
		return fmt.Sprintf("r%d", index&0x0f)
	}
	return name
}

// itoa renders a small offset in the disassembly text.
func itoa(value int64) string { return fmt.Sprintf("%d", value) }

func reg(value uint32) armasm.Reg  { return armasm.Reg(value & 0x0f) }
func imm(value uint32) armasm.Arg  { return armasm.Imm(value) }
func list(value uint32) armasm.Arg { return armasm.RegList(value) }
func pcrel(value int32) armasm.Arg { return armasm.PCRel(value) }

var _ = fmt.Sprintf

// isThumbMode reports whether one target asks for the Thumb encoding.
func isThumbMode(mode string) bool {
	switch mode {
	case "thumb", "thumb2":
		return true
	}
	return false
}

// decodeThumbRegion decodes one region in the Thumb encoding. Instructions are
// two or four bytes wide, so the loop advances by the decoded size.
func decodeThumbRegion(ctx context.Context, request core.ParseRequest, diagnostics *diagnostics) ([]instruction, error) {
	total := request.Reader.Size()
	result := make([]instruction, 0, total/2)
	var undecodable diag.Undecodable
	defer undecodable.Flush(diagnostics.add)
	var code [4]byte
	address := request.Address
	var state thumbState
	// The linker places jump tables and padding after an unconditional branch.
	// A halfword that fails to decode there is data, not a decoder gap.
	dataRegion := false
	for offset := int64(0); offset+2 <= total; {
		if offset&0xfff == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		want := 4
		if offset+4 > total {
			want = 2
		}
		n, err := request.Reader.ReadAt(code[:want], offset)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		if n < 2 {
			diagnostics.add("decode-error", fmt.Sprintf("short instruction at 0x%x", address))
			break
		}
		decoded, size := decodeThumb(code[:n], &state)
		if decoded.undefined {
			raw := uint32(binary.LittleEndian.Uint16(code[:2]))
			result = append(result, instruction{address: address, raw: raw, size: size, undefined: true})
			if !dataRegion {
				undecodable.Add(address, size)
			}
			offset += int64(size)
			address += uint64(size)
			continue
		}
		raw := uint32(binary.LittleEndian.Uint16(code[:2]))
		if size == 4 {
			raw = uint32(binary.LittleEndian.Uint16(code[2:]))<<16 | raw
		}
		decoded.inst.Enc = raw
		decoded.inst.Len = size
		result = append(result, instruction{
			address:    address,
			raw:        raw,
			inst:       decoded.inst,
			size:       size,
			text:       decoded.text,
			compare:    decoded.compare,
			opaque:     decoded.opaque,
			opaqueArgs: decoded.opaqueArgs,
		})
		dataRegion = decoded.compare == nil && !decoded.undefined &&
			(baseOp(decoded.inst.Op) == armasm.B || baseOp(decoded.inst.Op) == armasm.BX)
		offset += int64(size)
		address += uint64(size)
	}
	return result, nil
}
