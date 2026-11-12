package arm64

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"golang.org/x/arch/arm64/arm64asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

type instruction struct {
	address uint64
	raw     uint32
	inst    arm64asm.Inst
	size    int
	// text overrides the library rendering for encodings it cannot decode.
	text string
	// undefined marks a UDF word or an embedded data word. The lifter emits a
	// trap for it instead of guessing an instruction.
	undefined bool
	immediate uint32
}

func parse(ctx context.Context, request core.ParseRequest) (*core.ParseResult, error) {
	if request.Reader == nil || request.Reader.Size() == 0 {
		return nil, fmt.Errorf("arm64: empty code region")
	}
	diagnostics := &diagnostics{}
	decoded, err := decode(ctx, request, diagnostics)
	if err != nil {
		return nil, err
	}
	if len(decoded) == 0 {
		return nil, fmt.Errorf("arm64: no instruction decoded at 0x%x", request.Address)
	}
	functions, blockIDs := buildFunctions(decoded, request.Address, request.Address+uint64(request.Reader.Size()), diagnostics)
	ir := liftFunctions(request.Target, functions, blockIDs, diagnostics)
	result := &core.ParseResult{IR: ir, Diagnostics: diagnostics.finish()}
	if request.WithAssembly {
		result.Assembly = renderAssembly(decoded)
	}
	return result, nil
}

// decode reads one parser request. asm2lowir calls it per function boundary, so
// memory scales with the largest function, not the file.
func decode(ctx context.Context, request core.ParseRequest, diagnostics *diagnostics) ([]instruction, error) {
	if request.Reader == nil {
		return nil, fmt.Errorf("arm64: parse request has no reader")
	}
	total := request.Reader.Size()
	if total%4 != 0 {
		diagnostics.add("decode-error", fmt.Sprintf("arm64 region size %d is not a multiple of 4", total))
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
		if decoded, ok := decodePAC(raw, address); ok {
			result = append(result, decoded)
			address += 4
			continue
		}
		inst, err := arm64asm.Decode(code[:])
		if err != nil {
			if raw&0xffff0000 == 0 {
				// UDF #imm16. The linker also leaves zero and literal words in
				// __text; both reach the decoder as this shape.
				result = append(result, instruction{address: address, raw: raw, size: 4, undefined: true, immediate: raw & 0xffff})
				address += 4
				continue
			}
			diagnostics.add("decode-error", fmt.Sprintf("cannot decode instruction at 0x%x: %v", address, err))
			address += 4
			continue
		}
		result = append(result, instruction{address: address, raw: raw, inst: inst, size: 4})
		address += 4
	}
	return result, nil
}
