package arm

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"golang.org/x/arch/arm/armasm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

type instruction struct {
	address uint64
	raw     uint32
	inst    armasm.Inst
	size    int
	// text overrides the library rendering for encodings it cannot decode.
	text string
	// undefined marks a UDF word or an embedded data word.
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

func parse(ctx context.Context, request core.ParseRequest) (*core.ParseResult, error) {
	if request.Reader == nil || request.Reader.Size() == 0 {
		return nil, fmt.Errorf("arm: empty code region")
	}
	diagnostics := &diagnostics{}
	decoded, err := decode(ctx, request, diagnostics)
	if err != nil {
		return nil, err
	}
	if len(decoded) == 0 {
		return nil, fmt.Errorf("arm: no instruction decoded at 0x%x", request.Address)
	}
	end := request.Address + uint64(request.Reader.Size())
	functions, blockIDs := buildFunctions(decoded, request.Address, end, diagnostics)
	result := &core.ParseResult{
		IR:          liftFunctions(request.Target, functions, blockIDs, diagnostics),
		Diagnostics: diagnostics.finish(),
	}
	if request.WithAssembly {
		result.Assembly = renderAssembly(decoded)
	}
	return result, nil
}

// decode reads one parser request. asm2lowir calls it per function boundary, so
// memory scales with the largest function, not the file.
func decode(ctx context.Context, request core.ParseRequest, diagnostics *diagnostics) ([]instruction, error) {
	if request.Reader == nil {
		return nil, fmt.Errorf("arm: parse request has no reader")
	}
	total := request.Reader.Size()
	if total%4 != 0 {
		diagnostics.add("decode-error", fmt.Sprintf("arm region size %d is not a multiple of 4", total))
	}
	result := make([]instruction, 0, total/4)
	var code [4]byte
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
		raw := binary.LittleEndian.Uint32(code[:])
		inst, err := armasm.Decode(code[:], armasm.ModeARM)
		if err != nil {
			// Keep the word as a trap so the address range is not lost, and
			// report the failure once. The linker leaves literal pools inside
			// .text, so an undecodable word is data until proven otherwise.
			result = append(result, instruction{address: address, raw: raw, size: 4, undefined: true})
			diagnostics.add("decode-error", fmt.Sprintf("cannot decode instruction at 0x%x: %v", address, err))
			address += 4
			continue
		}
		result = append(result, instruction{address: address, raw: raw, inst: inst, size: 4})
		address += 4
	}
	return result, nil
}
