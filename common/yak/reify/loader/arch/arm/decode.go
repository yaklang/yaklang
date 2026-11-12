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

type instruction struct {
	address uint64
	raw     uint32
	inst    armasm.Inst
	size    int
	// text overrides the library rendering for encodings it cannot decode.
	text string
	// systemRegister marks an MRC or MCR transfer, which armasm cannot decode.
	systemRegister *systemRegisterTransfer
	// undefined marks a UDF word or an embedded data word.
	undefined bool
}

// systemRegisterTransfer is one MRC or MCR coprocessor transfer: the ARMv7 way
// to read a system register such as the thread pointer.
type systemRegisterTransfer struct {
	mnemonic    string
	coprocessor uint8
	opcode1     uint8
	opcode2     uint8
	register    uint8
	crn         uint8
	crm         uint8
	read        bool
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
	var undecodable diag.Undecodable
	defer undecodable.Flush(diagnostics.add)
	var code [4]byte
	address := request.Address
	// The linker places jump tables and padding after an unconditional branch.
	// A word that fails to decode there is data, not a decoder gap.
	dataRegion := false
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
		if transfer, ok := decodeCoprocessorTransfer(raw); ok {
			result = append(result, instruction{address: address, raw: raw, size: 4, text: transfer.text(), systemRegister: transfer})
			address += 4
			continue
		}
		inst, err := armasm.Decode(code[:], armasm.ModeARM)
		if err != nil && indexRegisterBit(raw) {
			// armasm pins bit 3 of Rm in the register-offset store entry, so
			// every store whose index register is r8..r15 fails.
			var patched [4]byte
			binary.LittleEndian.PutUint32(patched[:], raw&^0x8)
			if decoded, patchedErr := armasm.Decode(patched[:], armasm.ModeARM); patchedErr == nil {
				restoreIndexRegister(&decoded)
				inst, err = decoded, nil
			}
		}
		if err != nil {
			// Keep the word as a trap so the address range is not lost. Literal
			// pools, padding and fill patterns live inside .text and reach the
			// decoder as data. They stay silent; other failures are grouped into
			// one diagnostic per contiguous run.
			result = append(result, instruction{address: address, raw: raw, size: 4, undefined: true})
			if !isDataWord(raw) && !dataRegion {
				undecodable.Add(address, 4)
			}
			address += 4
			continue
		}
		dataRegion = isUnconditionalTerminator(inst)
		result = append(result, instruction{address: address, raw: raw, inst: inst, size: 4})
		address += 4
	}
	return result, nil
}

// isUnconditionalTerminator reports whether execution never falls through to the
// next word. The words after such an instruction are padding or a jump table.
func isUnconditionalTerminator(inst armasm.Inst) bool {
	switch baseOp(inst.Op) {
	case armasm.B, armasm.BX:
		return true
	}
	// A register write to pc is a branch as well.
	for _, arg := range inst.Args {
		if register, ok := arg.(armasm.Reg); ok && register == armasm.PC {
			return true
		}
		if memory, ok := arg.(armasm.Mem); ok && memory.Base == armasm.PC {
			continue
		}
	}
	return false
}

// decodeCoprocessorTransfer decodes MRC and MCR, the ARMv7 system register
// transfers. armasm has no table for them at all.
func decodeCoprocessorTransfer(raw uint32) (*systemRegisterTransfer, bool) {
	// cond:4|1|1|1|0|opc1:3|L:1|CRn:4|Rt:4|coproc:4|opc2:3|1|CRm:4
	if raw&0x0f000000 != 0x0e000000 {
		return nil, false
	}
	if raw&0x10 == 0 {
		return nil, false
	}
	read := raw&0x00100000 != 0
	transfer := &systemRegisterTransfer{
		coprocessor: uint8(raw >> 8 & 0x0f),
		opcode1:     uint8(raw >> 21 & 0x07),
		opcode2:     uint8(raw >> 5 & 0x07),
		register:    uint8(raw >> 12 & 0x0f),
		crn:         uint8(raw >> 16 & 0x0f),
		crm:         uint8(raw & 0x0f),
		read:        read,
	}
	transfer.mnemonic = "mcr"
	if read {
		transfer.mnemonic = "mrc"
	}
	return transfer, true
}

// text renders one system register transfer in the GNU syntax.
func (t *systemRegisterTransfer) text() string {
	return fmt.Sprintf("p%d, #%d, r%d, c%d, c%d, #%d",
		t.coprocessor, t.opcode1, t.register, t.crn, t.crm, t.opcode2)
}

// indexRegisterBit reports whether the word is the register-offset store form
// whose armasm table entry pins bit 3 of the Rm field.
func indexRegisterBit(raw uint32) bool {
	// cond:4|0|1|1|P|U|0|W|0|Rn:4|Rt:4|imm5:5|type:2|0|Rm:4, store forms only.
	if raw&0x0e500010 != 0x06000000 {
		return false
	}
	return raw&0x8 != 0
}

// restoreIndexRegister adds back the bit cleared by indexRegisterBit.
func restoreIndexRegister(inst *armasm.Inst) {
	for index, arg := range inst.Args {
		memory, ok := arg.(armasm.Mem)
		if !ok || memory.Index == 0 {
			continue
		}
		memory.Index += 8
		inst.Args[index] = memory
	}
}

// isDataWord reports whether a word looks like data rather than an ARM
// instruction. The Go runtime fills unused code space with 0x01010101 and
// 0xfefefefe, and the linker leaves zero and small-integer literal pools.
func isDataWord(raw uint32) bool {
	if raw&0xff000000 == 0 {
		return true
	}
	if raw == 0x01010101 || raw == 0xfefefefe || raw == 0xffffffff {
		return true
	}
	first := byte(raw)
	return byte(raw>>8) == first && byte(raw>>16) == first && byte(raw>>24) == first
}
