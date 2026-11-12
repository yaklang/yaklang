package mips

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/loader/arch/internal/diag"
)

// instruction is one decoded MIPS instruction. MIPS has fixed-width 4-byte
// instructions plus the 16-bit MIPS16 and microMIPS forms, which are not
// decoded here.
type instruction struct {
	address uint64
	raw     uint32
	size    int
	// op is the formatted mnemonic, for example "addiu" or "lw".
	op string
	// operands holds the decoded operands in assembly order.
	operands []operand
	// undefined marks a word the decoder rejected.
	undefined bool
}

// operandKind distinguishes register, immediate and memory operands.
type operandKind uint8

const (
	operandRegister operandKind = iota
	operandImmediate
	operandMemory
)

// operand is one decoded operand. Memory operands keep the base register and
// the displacement separately.
type operand struct {
	kind     operandKind
	register int
	immediat int64
	base     int
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
		result.Assembly = renderAssembly(decoded, request.Target.ByteOrder)
	}
	return result, nil
}

// decodeInstructions reads one parser request. asm2lowir calls it per function
// boundary, so memory scales with the largest function, not the file.
func decodeInstructions(ctx context.Context, profile Profile, request core.ParseRequest, diagnostics *diagnostics) ([]instruction, error) {
	if request.Reader == nil {
		return nil, fmt.Errorf("%s: parse request has no reader", profile.Name)
	}
	order := byteOrder(request.Target)
	total := request.Reader.Size()
	result := make([]instruction, 0, total/4)
	var undecodable diag.Undecodable
	defer undecodable.Flush(diagnostics.add)
	var code [4]byte
	// The linker places a jump table right after an unconditional jump.
	dataRegion := false
	address := request.Address
	for offset := int64(0); offset+4 <= total; offset += 4 {
		if offset&0xfff == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		n, err := request.Reader.ReadAt(code[:], offset)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		if n < 4 {
			diagnostics.add("decode-error", fmt.Sprintf("short instruction at 0x%x", address))
			break
		}
		raw := order.Uint32(code[:])
		decoded, ok := decodeWord(raw, profile)
		if !ok {
			// Jump tables and literal pools live inside .text. A word that fails
			// to decode after an unconditional branch is data, not a gap.
			result = append(result, instruction{address: address, raw: raw, size: 4, undefined: true})
			if !dataRegion {
				undecodable.Add(address, 4)
			}
			address += 4
			continue
		}
		decoded.address = address
		decoded.raw = raw
		decoded.size = 4
		dataRegion = isUnconditionalTerminator(decoded)
		result = append(result, decoded)
		address += 4
	}
	return result, nil
}

// byteOrder returns the byte order MIPS code uses. MIPS supports both; the
// loader reports which one the file uses.
func byteOrder(target core.Target) binary.ByteOrder {
	if target.ByteOrder == core.LittleEndian {
		return binary.LittleEndian
	}
	return binary.BigEndian
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
			} else if !isCall(ins.op) {
				diagnostics.add("branch-target-outside-region",
					fmt.Sprintf("control transfer at 0x%x targets 0x%x outside the decoded region", ins.address, target))
			}
		}
		if isTerminator(ins.op) {
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

// isUnconditionalTerminator reports whether execution never falls through to the
// next word. MIPS executes a delay slot first, so the word after a jump is still
// code; the run of data starts after it.
func isUnconditionalTerminator(ins instruction) bool {
	switch ins.op {
	case "j", "jr":
		return true
	}
	return false
}
