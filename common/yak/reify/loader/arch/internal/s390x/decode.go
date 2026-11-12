package s390x

import (
	"context"
	"errors"
	"fmt"
	"io"

	"golang.org/x/arch/s390x/s390xasm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/loader/arch/internal/diag"
)

// instruction is one decoded s390x instruction. The architecture mixes two,
// four and six byte encodings, so the size is recorded per instruction.
type instruction struct {
	address uint64
	// encoded keeps the whole instruction: s390x mixes two, four and six byte
	// forms, so a single word cannot hold the longest one.
	encoded [6]byte
	size    int
	inst    s390xasm.Inst
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
	functions, blockIDs := buildFunctions(decoded, request.Address, end, diagnostics)
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
	result := make([]instruction, 0, total/4)
	var undecodable diag.Undecodable
	defer undecodable.Flush(diagnostics.add)
	var code [6]byte
	address := request.Address
	// The linker places literal pools and jump tables after an unconditional
	// branch. A word that fails to decode there is data, not a decoder gap.
	dataRegion := false
	for offset := int64(0); offset+2 <= total; {
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
		inst, decodeErr := s390xasm.Decode(code[:n])
		if decodeErr != nil || inst.Len <= 0 || inst.Len > n {
			var encoded [6]byte
			copy(encoded[:], code[:2])
			result = append(result, instruction{address: address, encoded: encoded, size: 2, undefined: true})
			if !dataRegion {
				undecodable.Add(address, 2)
			}
			offset += 2
			address += 2
			continue
		}
		var encoded [6]byte
		copy(encoded[:], code[:n])
		decoded := instruction{address: address, encoded: encoded, size: inst.Len, inst: inst}
		dataRegion = isTerminator(nameOf(inst.Op))
		result = append(result, decoded)
		offset += int64(inst.Len)
		address += uint64(inst.Len)
	}
	return result, nil
}

func buildFunctions(decoded []instruction, start, end uint64, diagnostics *diagnostics) ([]function, map[uint64]core.BlockID) {
	starts := make(map[uint64]bool, len(decoded))
	starts[decoded[0].address] = true
	for _, ins := range decoded {
		if ins.undefined {
			starts[ins.address] = true
			continue
		}
		next := ins.address + uint64(ins.size)
		if target, ok := branchTarget(ins.inst, ins.address, ins.size); ok {
			if target >= start && target < end {
				starts[target] = true
			} else if !isCall(nameOf(ins.inst.Op)) {
				diagnostics.add("branch-target-outside-region",
					fmt.Sprintf("control transfer at 0x%x targets 0x%x outside the decoded region", ins.address, target))
			}
		}
		if isTerminator(nameOf(ins.inst.Op)) {
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

// args returns the arguments of one decoded instruction, without the trailing
// nil entries the library leaves.
func args(inst s390xasm.Inst) []s390xasm.Arg {
	result := make([]s390xasm.Arg, 0, 6)
	for _, arg := range inst.Args {
		if arg == nil {
			break
		}
		result = append(result, arg)
	}
	return result
}
