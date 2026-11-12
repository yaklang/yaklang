package ppc

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"golang.org/x/arch/ppc64/ppc64asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/loader/arch/internal/diag"
)

// instruction is one decoded PowerPC instruction. Instruction length is four
// bytes for the classic encoding; the prefixed forms are eight.
type instruction struct {
	address uint64
	raw     uint32
	size    int
	inst    ppc64asm.Inst
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
	var code [8]byte
	address := request.Address
	for offset := int64(0); offset+4 <= total; {
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
		inst, decodeErr := ppc64asm.Decode(code[:n], order)
		if decodeErr != nil || inst.Len <= 0 {
			// A rejected word is data inside the code region: padding, a
			// literal pool or a jump table.
			raw := order.Uint32(code[:4])
			result = append(result, instruction{address: address, raw: raw, size: 4, undefined: true})
			undecodable.Add(address, 4)
			offset += 4
			address += 4
			continue
		}
		raw := order.Uint32(code[:4])
		result = append(result, instruction{address: address, raw: raw, size: inst.Len, inst: inst})
		offset += int64(inst.Len)
		address += uint64(inst.Len)
	}
	return result, nil
}

// byteOrder returns the byte order PowerPC code uses. Big-endian is the classic
// encoding; the ppc64le ABI is little-endian and the loader reports it.
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
