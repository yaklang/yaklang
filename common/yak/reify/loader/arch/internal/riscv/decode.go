package riscv

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"golang.org/x/arch/riscv64/riscv64asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/loader/arch/internal/diag"
)

// instruction is one decoded RISC-V instruction. RISC-V allows 2-byte
// compressed forms, so the size is recorded per instruction.
type instruction struct {
	address uint64
	raw     uint32
	size    int
	inst    riscv64asm.Inst
	// target is the link-time destination of a PC-relative control transfer. A
	// relocatable object leaves the encoded displacement zero and records the
	// real target only in a relocation.
	target    uint64
	hasTarget bool
	// undefined marks a word the decoder rejected.
	undefined bool
}

type block struct {
	start        uint64
	instructions []instruction
}

type function struct {
	start  uint64
	blocks []block
}

func parse(ctx context.Context, profile Profile, request core.ParseRequest) (*core.ParseResult, error) {
	if request.Reader == nil || request.Reader.Size() == 0 {
		return nil, fmt.Errorf("%s: empty code region", profile.Name)
	}
	diagnostics := &diagnostics{profile: profile}
	decoded, err := decodeInstructions(ctx, profile, request, diagnostics)
	if err != nil {
		return nil, err
	}
	if len(decoded) == 0 {
		return nil, fmt.Errorf("%s: no instruction decoded at 0x%x", profile.Name, request.Address)
	}
	end := request.Address + uint64(request.Reader.Size())
	functions, blockIDs := buildFunctions(profile, decoded, request.Address, end, diagnostics)
	result := &core.ParseResult{
		IR:          liftFunctions(profile, request.Target, functions, blockIDs, diagnostics),
		Diagnostics: diagnostics.finish(),
	}
	if request.WithAssembly {
		result.Assembly = renderAssembly(decoded)
	}
	return result, nil
}

// decodeInstructions reads one parser request. asm2lowir calls it per function
// boundary, so memory scales with the largest function, not the file.
func decodeInstructions(ctx context.Context, profile Profile, request core.ParseRequest, diagnostics *diagnostics) ([]instruction, error) {
	if request.Reader == nil {
		return nil, fmt.Errorf("%s: parse request has no reader", profile.Name)
	}
	total := request.Reader.Size()
	result := make([]instruction, 0, total/2)
	var undecodable diag.Undecodable
	defer undecodable.Flush(diagnostics.add)
	var code [4]byte
	address := request.Address
	for offset := int64(0); offset < total; {
		if offset&0xfff == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		n, err := request.Reader.ReadAt(code[:], offset)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		if n < 2 {
			diagnostics.add("decode-error", fmt.Sprintf("short instruction at 0x%x", address))
			break
		}
		var inst riscv64asm.Inst
		var decodeErr error
		if profile.Bits == 32 {
			// C.JAL exists on RV32 only; the RV64 decoder reads the same halfword
			// as an add-immediate-word, or rejects it outright.
			if jal, ok := compressedJAL(code[:2]); ok {
				inst, decodeErr = jal, nil
			}
		}
		if decodeErr == nil && inst.Len <= 0 {
			inst, decodeErr = riscv64asm.Decode(code[:n])
		}
		if decodeErr != nil || inst.Len <= 0 {
			// A rejected word is data inside the code region: padding, a literal
			// pool or an unsupported extension. Its length still follows the
			// encoding: the low two bits are 11 only for a four-byte instruction.
			size := 4
			if code[0]&0x3 != 0x3 {
				size = 2
			}
			result = append(result, instruction{address: address, raw: binary.LittleEndian.Uint32(code[:]), size: size, undefined: true})
			undecodable.Add(address, size)
			offset += int64(size)
			address += uint64(size)
			continue
		}
		raw := binary.LittleEndian.Uint32(code[:])
		decoded := instruction{address: address, raw: raw, inst: inst, size: inst.Len}
		if target, ok := relocationTarget(request.Relocations, address); ok {
			decoded.target = target
			decoded.hasTarget = true
		}
		result = append(result, decoded)
		offset += int64(inst.Len)
		address += uint64(inst.Len)
	}
	return result, nil
}

// relocationTarget returns the address a relocation at one instruction address
// resolves to. RISC-V records every PC-relative branch and jump this way, even
// when the target is a local label inside the same object.
func relocationTarget(relocations []core.Relocation, address uint64) (uint64, bool) {
	for _, relocation := range relocations {
		if relocation.Address == address && relocation.Value != 0 {
			return relocation.Value, true
		}
	}
	return 0, false
}

func buildFunctions(profile Profile, decoded []instruction, start, end uint64, diagnostics *diagnostics) ([]function, map[uint64]core.BlockID) {
	starts := make(map[uint64]bool, len(decoded))
	starts[decoded[0].address] = true
	for _, ins := range decoded {
		if ins.undefined {
			starts[ins.address] = true
			continue
		}
		next := ins.address + uint64(ins.size)
		if target, ok := branchTarget(ins); ok {
			if target >= start && target < end {
				starts[target] = true
			} else if !isCall(ins.inst.Op) {
				diagnostics.add("branch-target-outside-region",
					fmt.Sprintf("control transfer at 0x%x targets 0x%x outside the decoded region", ins.address, target))
			}
		}
		if isTerminator(ins.inst.Op) {
			starts[next] = true
		}
	}
	addresses := make([]uint64, 0, len(starts))
	for address := range starts {
		addresses = append(addresses, address)
	}
	sortAddresses(addresses)
	blocks := make([]block, 0, len(addresses))
	var current *block
	for _, ins := range decoded {
		if starts[ins.address] || current == nil {
			blocks = append(blocks, block{start: ins.address})
			current = &blocks[len(blocks)-1]
		}
		current.instructions = append(current.instructions, ins)
	}
	blockIDs := make(map[uint64]core.BlockID, len(blocks))
	for index := range blocks {
		blockIDs[blocks[index].start] = core.BlockID(index + 1)
	}
	return []function{{start: blocks[0].start, blocks: blocks}}, blockIDs
}

// compressedJAL decodes the RV32 C.JAL form, which the RV64 decoder maps to a
// different instruction. The immediate uses the C.J layout, and the form always
// links through x1.
func compressedJAL(code []byte) (riscv64asm.Inst, bool) {
	if len(code) < 2 {
		return riscv64asm.Inst{}, false
	}
	half := binary.LittleEndian.Uint16(code)
	if half&0xe003 != 0x2001 {
		return riscv64asm.Inst{}, false
	}
	// The immediate is reordered across the halfword; reassemble it in the order
	// the JAL argument expects.
	offset := int32(0)
	offset |= int32((half>>12)&1) << 11
	offset |= int32((half>>11)&1) << 4
	offset |= int32((half>>9)&0x3) << 8
	offset |= int32((half>>8)&1) << 10
	offset |= int32((half>>7)&1) << 6
	offset |= int32((half>>6)&1) << 7
	offset |= int32((half>>3)&0x7) << 1
	offset |= int32((half>>2)&1) << 5
	// Sign-extend from bit eleven.
	offset = (offset << 20) >> 20
	return riscv64asm.Inst{
		Op:   riscv64asm.JAL,
		Args: riscv64asm.Args{riscv64asm.Reg(1), riscv64asm.Simm{Imm: offset, Width: 12}},
		Len:  2,
	}, true
}
