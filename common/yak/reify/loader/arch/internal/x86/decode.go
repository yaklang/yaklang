package x86

import (
	"context"
	"errors"
	"fmt"
	"io"

	"golang.org/x/arch/x86/x86asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

type block struct {
	start        uint64
	instructions []instruction
}

type instruction struct {
	address uint64
	size    int
	inst    x86asm.Inst
	endbr   string
	raw     []byte
}

func parse(ctx context.Context, profile Profile, request core.ParseRequest) (*core.ParseResult, error) {
	if request.Reader == nil || request.Reader.Size() == 0 {
		return nil, fmt.Errorf("%s: empty code region", profile.Name)
	}
	diagnostics := &diagnostics{profile: profile}
	decoded, err := decodeInstructions(ctx, profile, request, diagnostics, request.WithAssembly)
	if err != nil {
		return nil, err
	}
	if len(decoded) == 0 {
		message := fmt.Sprintf("%s: no instruction decoded at 0x%x", profile.Name, request.Address)
		if len(diagnostics.items) > 0 {
			message += ": " + diagnostics.items[0].Message
		}
		return nil, fmt.Errorf("%s", message)
	}
	functions, blockIDs := buildFunctions(decoded, diagnostics)
	result := &core.ParseResult{
		IR:          liftFunctions(profile, request.Target, functions, blockIDs, diagnostics),
		Diagnostics: diagnostics.finish(),
	}
	if request.WithAssembly {
		result.Assembly = renderAssembly(profile, decoded, request.Syntax)
	}
	return result, nil
}

// decodeInstructions decodes one parser request. asm2lowir issues one request per
// function boundary, so memory scales with the largest function, not the file.
func decodeInstructions(ctx context.Context, profile Profile, request core.ParseRequest, diagnostics *diagnostics, keepRaw bool) ([]instruction, error) {
	if request.Reader == nil {
		return nil, fmt.Errorf("%s: parse request has no reader", profile.Name)
	}
	result := make([]instruction, 0)
	var code [15]byte
	total := request.Reader.Size()
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
		if n == 0 {
			diagnostics.add("decode-error", fmt.Sprintf("reader returned no bytes at 0x%x", address))
			break
		}
		instruction, size, err := decodeInstruction(profile, code[:n], address)
		if err != nil {
			diagnostics.add("decode-error", fmt.Sprintf("cannot decode instruction at 0x%x: %v", address, err))
			break
		}
		if keepRaw {
			instruction.raw = append([]byte(nil), code[:size]...)
		}
		result = append(result, instruction)
		offset += int64(size)
		address += uint64(size)
	}
	return result, nil
}

func decodeInstruction(profile Profile, code []byte, address uint64) (instruction, int, error) {
	if len(code) >= 4 && code[0] == 0xf3 && code[1] == 0x0f && code[2] == 0x1e {
		switch code[3] {
		case 0xfa:
			name := "endbr64"
			if profile.Mode == 32 {
				name = "endbr32"
			}
			return instruction{address: address, size: 4, endbr: name}, 4, nil
		case 0xfb:
			return instruction{address: address, size: 4, endbr: "endbr32"}, 4, nil
		}
	}
	inst, err := x86asm.Decode(code, profile.Mode)
	if err != nil {
		return instruction{}, 0, err
	}
	if inst.Len <= 0 || inst.Len > len(code) {
		return instruction{}, 0, fmt.Errorf("invalid instruction length %d", inst.Len)
	}
	if inst.Op == 0 {
		return instruction{}, 0, fmt.Errorf("unknown opcode at byte 0x%02x", code[0])
	}
	return instruction{address: address, size: inst.Len, inst: inst}, inst.Len, nil
}

type function struct {
	start  uint64
	blocks []block
}

func buildFunctions(decoded []instruction, diagnostics *diagnostics) ([]function, map[uint64]core.BlockID) {
	byAddress := make(map[uint64]int, len(decoded))
	for index, instruction := range decoded {
		byAddress[instruction.address] = index
	}
	start := decoded[0].address
	end := decoded[len(decoded)-1].address + uint64(decoded[len(decoded)-1].size)
	starts := map[uint64]bool{start: true}
	addStart := func(target uint64, source uint64) {
		if _, ok := byAddress[target]; ok {
			starts[target] = true
			return
		}
		if target < start || target >= end {
			diagnostics.add("branch-target-outside-region", fmt.Sprintf("control transfer at 0x%x targets 0x%x outside the decoded region", source, target))
			return
		}
		diagnostics.add("branch-target-misaligned", fmt.Sprintf("control transfer at 0x%x targets 0x%x inside an instruction", source, target))
	}
	for _, instruction := range decoded {
		next := instruction.address + uint64(instruction.size)
		if instruction.endbr != "" {
			continue
		}
		if isConditionalJump(instruction.inst.Op) {
			if target, ok := directTarget(instruction); ok {
				addStart(target, instruction.address)
			}
			if _, ok := byAddress[next]; ok {
				starts[next] = true
			}
			continue
		}
		if instruction.inst.Op == x86asm.JMP {
			if target, ok := directTarget(instruction); ok {
				addStart(target, instruction.address)
			}
			if _, ok := byAddress[next]; ok {
				starts[next] = true
			}
			continue
		}
		if instruction.inst.Op == x86asm.CALL {
			if target, ok := directTarget(instruction); ok {
				if _, exists := byAddress[target]; exists {
					starts[target] = true
				}
			}
		}
		if isTerminator(instruction) {
			if _, ok := byAddress[next]; ok {
				starts[next] = true
			}
		}
	}

	blocks := make([]block, 0, len(starts))
	var current *block
	for _, instruction := range decoded {
		if starts[instruction.address] || current == nil {
			blocks = append(blocks, block{start: instruction.address})
			current = &blocks[len(blocks)-1]
		}
		current.instructions = append(current.instructions, instruction)
	}
	blockIDs := make(map[uint64]core.BlockID, len(blocks))
	for index := range blocks {
		blockIDs[blocks[index].start] = core.BlockID(index + 1)
	}
	return []function{{start: blocks[0].start, blocks: blocks}}, blockIDs
}
