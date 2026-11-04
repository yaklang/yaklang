package arch

import (
	"context"
	"fmt"

	"golang.org/x/arch/x86/x86asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

type amd64Block struct {
	start        uint64
	instructions []amd64Instruction
}

type amd64Instruction struct {
	address uint64
	size    int
	inst    x86asm.Inst
	endbr   string
}

func parseAMD64(ctx context.Context, request core.ParseRequest) (*core.ParseResult, error) {
	parser := AMD64{}
	if err := parser.Check(request.Target); err != nil {
		return nil, err
	}
	if len(request.Code) == 0 {
		return nil, fmt.Errorf("amd64: empty code region")
	}
	diagnostics := &amd64Diagnostics{}
	decoded, err := decodeAMD64(ctx, request, diagnostics)
	if err != nil {
		return nil, err
	}
	if len(decoded) == 0 {
		message := fmt.Sprintf("amd64: no instruction decoded at 0x%x", request.Address)
		if len(diagnostics.items) > 0 {
			message += ": " + diagnostics.items[0].Message
		}
		return nil, fmt.Errorf("%s", message)
	}
	blocks, blockIDs := buildAMD64Blocks(decoded, diagnostics)
	return &core.ParseResult{
		IR:          liftAMD64(request.Target, blocks, blockIDs, diagnostics),
		Diagnostics: diagnostics.finish(),
	}, nil
}

func decodeAMD64(ctx context.Context, request core.ParseRequest, diagnostics *amd64Diagnostics) ([]amd64Instruction, error) {
	decoded := make([]amd64Instruction, 0, len(request.Code)/3)
	address := request.Address
	for offset := 0; offset < len(request.Code); {
		if offset&0xfff == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		instruction, size, err := decodeAMD64One(request.Code[offset:], address)
		if err != nil {
			diagnostics.add("decode-error", fmt.Sprintf("cannot decode instruction at 0x%x: %v", address, err))
			break
		}
		decoded = append(decoded, instruction)
		offset += size
		address += uint64(size)
	}
	return decoded, nil
}

func decodeAMD64One(code []byte, address uint64) (amd64Instruction, int, error) {
	if len(code) >= 4 && code[0] == 0xf3 && code[1] == 0x0f && code[2] == 0x1e {
		switch code[3] {
		case 0xfa:
			return amd64Instruction{address: address, size: 4, endbr: "endbr64"}, 4, nil
		case 0xfb:
			return amd64Instruction{address: address, size: 4, endbr: "endbr32"}, 4, nil
		}
	}
	inst, err := x86asm.Decode(code, 64)
	if err != nil {
		return amd64Instruction{}, 0, err
	}
	if inst.Len <= 0 || inst.Len > len(code) {
		return amd64Instruction{}, 0, fmt.Errorf("invalid instruction length %d", inst.Len)
	}
	if inst.Op == 0 {
		return amd64Instruction{}, 0, fmt.Errorf("unknown opcode at byte 0x%02x", code[0])
	}
	return amd64Instruction{address: address, size: inst.Len, inst: inst}, inst.Len, nil
}

func buildAMD64Blocks(decoded []amd64Instruction, diagnostics *amd64Diagnostics) ([]amd64Block, map[uint64]core.BlockID) {
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
		if amd64ConditionalJump(instruction.inst.Op) {
			if target, ok := amd64DirectTarget(instruction); ok {
				addStart(target, instruction.address)
			}
			if _, ok := byAddress[next]; ok {
				starts[next] = true
			}
			continue
		}
		if instruction.inst.Op == x86asm.JMP {
			if target, ok := amd64DirectTarget(instruction); ok {
				addStart(target, instruction.address)
			}
			if _, ok := byAddress[next]; ok {
				starts[next] = true
			}
			continue
		}
		if instruction.inst.Op == x86asm.CALL {
			if target, ok := amd64DirectTarget(instruction); ok {
				if _, exists := byAddress[target]; exists {
					starts[target] = true
				}
			}
		}
		if amd64Terminator(instruction) {
			if _, ok := byAddress[next]; ok {
				starts[next] = true
			}
		}
	}

	blocks := make([]amd64Block, 0, len(starts))
	var current *amd64Block
	for _, instruction := range decoded {
		if starts[instruction.address] || current == nil {
			blocks = append(blocks, amd64Block{start: instruction.address})
			current = &blocks[len(blocks)-1]
		}
		current.instructions = append(current.instructions, instruction)
	}
	blockIDs := make(map[uint64]core.BlockID, len(blocks))
	for index := range blocks {
		blockIDs[blocks[index].start] = core.BlockID(index + 1)
	}
	return blocks, blockIDs
}
