package arm64

import (
	"fmt"
	"strings"

	"golang.org/x/arch/arm64/arm64asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

type lifter struct {
	blockIDs     map[uint64]core.BlockID
	diagnostics  *diagnostics
	nextValue    core.ValueID
	values       map[core.ValueID]core.ValueType
	instructions []core.Instruction
}

func liftFunctions(target core.Target, functions []function, blockIDs map[uint64]core.BlockID, diagnostics *diagnostics) *core.LowIR {
	ir := &core.LowIR{Target: target}
	for index, body := range functions {
		ir.Functions = append(ir.Functions, liftFunction(body, core.FunctionID(index+1), blockIDs, diagnostics))
	}
	return ir
}

func liftFunction(body function, id core.FunctionID, blockIDs map[uint64]core.BlockID, diagnostics *diagnostics) core.Function {
	result := core.Function{
		ID:      id,
		Address: body.blocks[0].start,
		Blocks:  make([]core.Block, 0, len(body.blocks)),
	}
	lifter := &lifter{blockIDs: blockIDs, diagnostics: diagnostics, values: make(map[core.ValueID]core.ValueType)}
	for index, current := range body.blocks {
		lifter.instructions = make([]core.Instruction, 0, len(current.instructions)*2)
		for _, ins := range current.instructions {
			lifter.lift(ins)
		}
		if len(lifter.instructions) == 0 || !controlTransfer(lifter.instructions[len(lifter.instructions)-1].Kind) {
			end := current.start
			if len(current.instructions) > 0 {
				last := current.instructions[len(current.instructions)-1]
				end = last.address + uint64(last.size)
			}
			if index+1 < len(body.blocks) {
				lifter.emit(core.OpJump, nil, nil, []core.BlockID{blockIDs[body.blocks[index+1].start]}, core.EffectNone, end)
			} else {
				lifter.emit(core.OpTrap, []core.Operand{symbolOperand("region-end")}, nil, nil, core.EffectMayTrap|core.EffectUnknown, end)
				diagnostics.add("block-falls-off-region", fmt.Sprintf("block at 0x%x reaches the end of the decoded region", current.start))
			}
		}
		result.Blocks = append(result.Blocks, core.Block{ID: core.BlockID(index + 1), Address: current.start, Instructions: lifter.instructions})
	}
	return result
}

func controlTransfer(kind core.Opcode) bool {
	switch kind {
	case core.OpJump, core.OpBranch, core.OpReturn, core.OpTrap:
		return true
	default:
		return false
	}
}

func symbolOperand(name string) core.Operand {
	return core.Operand{Kind: core.OperandSymbol, Name: name}
}

func (l *lifter) lift(ins instruction) {
	if ins.undefined {
		l.emit(core.OpTrap, []core.Operand{symbolOperand(fmt.Sprintf("arm64.udf%#x", ins.immediate))}, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
		return
	}
	switch ins.inst.Op {
	case arm64asm.NOP:
		l.emit(core.OpNop, nil, nil, nil, core.EffectNone, ins.address)
	case arm64asm.HINT:
		// PACIA1716, BTI and other architectural hints have no data effect.
		l.emit(core.OpNop, nil, nil, nil, core.EffectNone, ins.address)
	case arm64asm.MOV, arm64asm.MOVZ, arm64asm.MOVN, arm64asm.MOVK:
		l.move(ins)
	case arm64asm.ADD, arm64asm.ADDS, arm64asm.ADC, arm64asm.ADCS:
		l.binary(ins, core.OpAdd, "add")
	case arm64asm.SUB, arm64asm.SUBS, arm64asm.SBC, arm64asm.SBCS:
		l.binary(ins, core.OpSub, "sub")
	case arm64asm.AND, arm64asm.ANDS, arm64asm.BIC, arm64asm.BICS:
		kind := core.OpAnd
		if ins.inst.Op == arm64asm.BIC || ins.inst.Op == arm64asm.BICS {
			kind = core.OpAnd
		}
		l.binary(ins, kind, "and")
	case arm64asm.ORR, arm64asm.ORN:
		if l.orrMove(ins) {
			return
		}
		l.binary(ins, core.OpOr, "orr")
	case arm64asm.EOR, arm64asm.EON:
		l.binary(ins, core.OpXor, "eor")
	case arm64asm.LSL, arm64asm.LSLV:
		l.binary(ins, core.OpShl, "")
	case arm64asm.LSR, arm64asm.LSRV:
		l.binary(ins, core.OpLShr, "")
	case arm64asm.ASR, arm64asm.ASRV:
		l.binary(ins, core.OpAShr, "")
	case arm64asm.MUL, arm64asm.MADD, arm64asm.MSUB:
		l.multiply(ins)
	case arm64asm.UDIV:
		l.binary(ins, core.OpUDiv, "")
	case arm64asm.SDIV:
		l.binary(ins, core.OpSDiv, "")
	case arm64asm.NEG, arm64asm.NEGS:
		l.unary(ins, core.OpNeg, "")
	case arm64asm.MVN:
		l.unary(ins, core.OpNot, "")
	case arm64asm.CMP, arm64asm.CMN, arm64asm.TST:
		l.compare(ins)
	case arm64asm.B, arm64asm.BL:
		l.branchOrCall(ins)
	case arm64asm.BR, arm64asm.BLR:
		l.registerTransfer(ins)
	case arm64asm.RET:
		l.emit(core.OpReturn, nil, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
	case arm64asm.CBZ, arm64asm.CBNZ, arm64asm.TBZ, arm64asm.TBNZ:
		l.conditionalBranch(ins)
	case arm64asm.LDR, arm64asm.LDRB, arm64asm.LDRH, arm64asm.LDRSW,
		arm64asm.LDUR, arm64asm.LDURB, arm64asm.LDURH,
		arm64asm.LDRSB, arm64asm.LDRSH, arm64asm.LDURSB, arm64asm.LDURSH,
		arm64asm.LDP:
		l.load(ins)
	case arm64asm.STR, arm64asm.STRB, arm64asm.STRH,
		arm64asm.STUR, arm64asm.STURB, arm64asm.STURH, arm64asm.STP:
		l.store(ins)
	case arm64asm.ADR, arm64asm.ADRP:
		l.address(ins)
	case arm64asm.SVC, arm64asm.BRK, arm64asm.HLT:
		l.emit(core.OpTrap, []core.Operand{symbolOperand("arm64." + lower(ins.inst.Op))}, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
	case arm64asm.SXTW, arm64asm.SXTB, arm64asm.SXTH, arm64asm.UXTB, arm64asm.UXTH:
		l.extend(ins)
	case arm64asm.CSEL, arm64asm.CSINC, arm64asm.CSINV, arm64asm.CSNEG,
		arm64asm.CSET, arm64asm.CSETM, arm64asm.CINC, arm64asm.CINV, arm64asm.CNEG:
		l.conditionalSelect(ins)
	case arm64asm.CCMP, arm64asm.CCMN:
		l.conditionalCompare(ins)
	case arm64asm.SBFIZ, arm64asm.SBFX, arm64asm.UBFIZ, arm64asm.UBFX,
		arm64asm.BFI, arm64asm.BFXIL, arm64asm.SBFM, arm64asm.UBFM, arm64asm.BFM:
		l.bitfield(ins)
	case arm64asm.UMULL, arm64asm.SMULL, arm64asm.UMADDL, arm64asm.SMADDL,
		arm64asm.UMSUBL, arm64asm.SMSUBL, arm64asm.SMULH, arm64asm.UMULH:
		l.longMultiply(ins)
	case arm64asm.REV, arm64asm.REV16, arm64asm.REV32, arm64asm.RBIT:
		l.reverse(ins)
	case arm64asm.ROR, arm64asm.RORV, arm64asm.EXTR:
		l.rotate(ins)
	default:
		l.unsupported(ins)
	}
}

func (l *lifter) hasVectorOperand(ins instruction) bool {
	for _, arg := range ins.inst.Args {
		if arg == nil {
			break
		}
		if name, _, ok := argRegister(arg); ok && isVectorRegister(name) {
			return true
		}
	}
	return false
}

func lower(op arm64asm.Op) string {
	return strings.ToLower(op.String())
}

// unsupported keeps the instruction in the IR as one intrinsic. The decoder
// recognized it, so dropping it would silently lose the effect. SIMD and
// floating-point instructions are modeled as opaque vector operations instead
// of being reported as unknown scalar instructions.
func (l *lifter) unsupported(ins instruction) {
	if l.hasVectorOperand(ins) {
		l.vectorOperation(ins)
		return
	}
	operands := []core.Operand{symbolOperand("arm64." + lower(ins.inst.Op))}
	for index, arg := range ins.inst.Args {
		if arg == nil {
			break
		}
		if operand, ok := l.rawOperand(ins, index, arg); ok {
			operands = append(operands, operand)
		}
	}
	l.emit(core.OpIntrinsic, operands, nil, nil, core.EffectUnknown, ins.address)
	l.diagnostics.add("unsupported-instruction", fmt.Sprintf("unsupported arm64 instruction %s at 0x%x", ins.inst.Op, ins.address))
}

func (l *lifter) rawOperand(ins instruction, index int, arg arm64asm.Arg) (core.Operand, bool) {
	if name, bits, ok := argRegister(arg); ok {
		return l.value(l.readRegister(ins.address, name, bits)), true
	}
	if value, ok := immediateValue(arg); ok {
		return l.value(l.constant(ins.address, value, 64)), true
	}
	if base, displacement, _, ok := memoryAddress(arg); ok {
		return l.value(l.addressFromBase(ins, base, displacement)), true
	}
	if offset, ok := pcRelative(arg); ok {
		return l.value(l.pointerConstant(ins.address, ins.address+uint64(offset))), true
	}
	return core.Operand{}, false
}

// --- emission helpers -------------------------------------------------------

func (l *lifter) emit(kind core.Opcode, operands []core.Operand, results []core.Value, targets []core.BlockID, effects core.Effect, address uint64) {
	l.instructions = append(l.instructions, core.Instruction{
		Kind:     kind,
		Operands: operands,
		Results:  results,
		Targets:  targets,
		Effects:  effects,
		Address:  address,
	})
}

func (l *lifter) newValue(kind core.TypeKind, bits uint16) core.Value {
	if bits == 0 {
		bits = 64
	}
	l.nextValue++
	value := core.Value{ID: l.nextValue, Type: core.ValueType{Kind: kind, Bits: bits}}
	l.values[value.ID] = value.Type
	return value
}

func (l *lifter) value(id core.ValueID) core.Operand {
	typ, ok := l.values[id]
	if !ok {
		typ = core.ValueType{Kind: core.TypeInteger, Bits: 64}
	}
	return core.Operand{Kind: core.OperandValue, Type: typ, Value: id}
}

func (l *lifter) operation(ins instruction, kind core.Opcode, operands []core.Operand, bits uint16, effects core.Effect) core.ValueID {
	result := l.newValue(core.TypeInteger, bits)
	l.emit(kind, operands, []core.Value{result}, nil, effects, ins.address)
	return result.ID
}

func (l *lifter) constant(address uint64, value uint64, bits uint16) core.ValueID {
	if bits == 0 {
		bits = 64
	}
	result := l.newValue(core.TypeInteger, bits)
	l.emit(core.OpConstant, []core.Operand{{
		Kind:    core.OperandConstant,
		Type:    result.Type,
		Literal: literal(value, bits),
	}}, []core.Value{result}, nil, core.EffectNone, address)
	return result.ID
}

// pointerOperand is a constant target operand, matching the AMD64 parser's
// rendering of direct calls and jumps.
func pointerOperand(value uint64) core.Operand {
	return core.Operand{
		Kind:    core.OperandConstant,
		Type:    core.ValueType{Kind: core.TypePointer, Bits: 64},
		Literal: literal(value, 64),
	}
}

func (l *lifter) pointerConstant(address, value uint64) core.ValueID {
	result := l.newValue(core.TypePointer, 64)
	l.emit(core.OpConstant, []core.Operand{{
		Kind:    core.OperandConstant,
		Type:    result.Type,
		Literal: literal(value, 64),
	}}, []core.Value{result}, nil, core.EffectNone, address)
	return result.ID
}

func (l *lifter) readRegister(address uint64, name string, bits uint16) core.ValueID {
	if isZeroRegister(name) {
		return l.constant(address, 0, bits)
	}
	result := l.newValue(core.TypeInteger, bits)
	l.emit(core.OpReadRegister, []core.Operand{registerOperand(name, bits)}, []core.Value{result}, nil, core.EffectReadRegister, address)
	return result.ID
}

func (l *lifter) writeRegister(address uint64, name string, bits uint16, value core.ValueID) {
	if isZeroRegister(name) {
		// Writes to the zero register are discarded by the hardware.
		return
	}
	l.emit(core.OpWriteRegister, []core.Operand{registerOperand(name, bits), l.value(value)}, nil, nil, core.EffectWriteRegister, address)
}

func (l *lifter) loadValue(address uint64, pointer core.ValueID, bits uint16) core.ValueID {
	result := l.newValue(core.TypeInteger, bits)
	l.emit(core.OpLoad, []core.Operand{l.value(pointer)}, []core.Value{result}, nil, core.EffectReadMemory, address)
	return result.ID
}

func (l *lifter) storeValue(address uint64, pointer, value core.ValueID) {
	l.emit(core.OpStore, []core.Operand{l.value(pointer), l.value(value)}, nil, nil, core.EffectWriteMemory, address)
}

func registerOperand(name string, bits uint16) core.Operand {
	return core.Operand{Kind: core.OperandRegister, Type: core.ValueType{Kind: core.TypeInteger, Bits: bits}, Name: name}
}

func literal(value uint64, bits uint16) []byte {
	size := int(bits / 8)
	if size == 0 {
		size = 1
	}
	result := make([]byte, size)
	for index := range result {
		result[index] = byte(value >> (8 * index))
	}
	return result
}

func (l *lifter) pointerFrom(address uint64, value core.ValueID) core.ValueID {
	if l.values[value].Kind == core.TypePointer {
		return value
	}
	result := l.newValue(core.TypePointer, 64)
	l.emit(core.OpCopy, []core.Operand{l.value(value)}, []core.Value{result}, nil, core.EffectNone, address)
	return result.ID
}

func (l *lifter) coerce(ins instruction, value core.ValueID, from, to uint16, signed bool) core.ValueID {
	if from == 0 || to == 0 || from == to {
		return value
	}
	if from > to {
		return l.operation(ins, core.OpTruncate, []core.Operand{l.value(value)}, to, core.EffectNone)
	}
	kind := core.OpZeroExtend
	if signed {
		kind = core.OpSignExtend
	}
	return l.operation(ins, kind, []core.Operand{l.value(value)}, to, core.EffectNone)
}

func (l *lifter) extendValue(ins instruction, value core.ValueID, kind extension, shift uint8, destinationBits uint16) (core.ValueID, bool) {
	if shift != 0 {
		count := l.constant(ins.address, uint64(shift), 64)
		value = l.operation(ins, core.OpShl, []core.Operand{l.value(value), l.value(count)}, 64, core.EffectNone)
	}
	switch kind {
	case extCopy:
		return value, true
	case extZero8:
		return l.operation(ins, core.OpZeroExtend, []core.Operand{l.value(value)}, destinationBits, core.EffectNone), true
	case extZero16:
		return l.operation(ins, core.OpZeroExtend, []core.Operand{l.value(value)}, destinationBits, core.EffectNone), true
	case extZero32:
		return l.operation(ins, core.OpZeroExtend, []core.Operand{l.value(value)}, destinationBits, core.EffectNone), true
	case extSign8, extSign16, extSign32:
		return l.operation(ins, core.OpSignExtend, []core.Operand{l.value(value)}, destinationBits, core.EffectNone), true
	case extShiftLeft:
		return value, true
	case extShiftRight:
		if shift == 0 {
			return value, true
		}
		count := l.constant(ins.address, uint64(shift), 64)
		return l.operation(ins, core.OpLShr, []core.Operand{l.value(value), l.value(count)}, 64, core.EffectNone), true
	case extShiftArith:
		if shift == 0 {
			return value, true
		}
		count := l.constant(ins.address, uint64(shift), 64)
		return l.operation(ins, core.OpAShr, []core.Operand{l.value(value), l.value(count)}, 64, core.EffectNone), true
	default:
		return 0, false
	}
}
