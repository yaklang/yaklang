package mips

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func disassemble(ctx context.Context, profile Profile, request core.ParseRequest) (*core.Disassembly, error) {
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
	_ = liftFunctions(profile, request.Target, functions, blockIDs, diagnostics)
	return &core.Disassembly{
		Text:         renderAssembly(decoded, request.Target.ByteOrder),
		Instructions: len(decoded),
		Diagnostics:  diagnostics.finish(),
	}, nil
}

// renderAssembly prints one instruction per line.
func renderAssembly(decoded []instruction, order core.ByteOrder) string {
	var text strings.Builder
	var byteOrder binary.ByteOrder = binary.BigEndian
	if order == core.LittleEndian {
		byteOrder = binary.LittleEndian
	}
	for _, ins := range decoded {
		mnemonic := formatMnemonic(ins)
		var buffer [4]byte
		byteOrder.PutUint32(buffer[:], ins.raw)
		fmt.Fprintf(&text, "0x%x  %02x %02x %02x %02x  %s\n",
			ins.address, buffer[0], buffer[1], buffer[2], buffer[3], mnemonic)
	}
	return text.String()
}

// formatMnemonic renders one instruction as mnemonic plus operands.
func formatMnemonic(ins instruction) string {
	if ins.undefined {
		return fmt.Sprintf(".word 0x%08x", ins.raw)
	}
	if len(ins.operands) == 0 {
		return ins.op
	}
	parts := make([]string, 0, len(ins.operands))
	for _, item := range ins.operands {
		parts = append(parts, renderOperand(item))
	}
	return ins.op + " " + strings.Join(parts, ", ")
}
