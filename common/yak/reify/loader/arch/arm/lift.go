package arm

import (
	"fmt"
	"strings"

	"golang.org/x/arch/arm/armasm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// lifter converts one function body into LowIR. Registers are explicit reads
// and writes, memory operations carry their own effects, and flags are the
// pseudo register cpsr, exactly like rflags on x86.
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

func (l *lifter) lift(ins instruction) {
	if ins.undefined {
		l.emit(core.OpTrap, []core.Operand{symbolOperand("arm.data")}, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
		return
	}
	if ins.systemRegister != nil {
		l.systemRegisterTransfer(ins)
		return
	}
	if name := ins.inst.Op.String(); strings.HasSuffix(name, ".W") || strings.HasSuffix(name, ".N") {
		// Width hints do not change the operation.
	}
	base := baseOp(ins.inst.Op)
	switch base {
	case armasm.NOP, armasm.YIELD, armasm.WFE, armasm.WFI, armasm.SEV, armasm.DMB, armasm.DSB, armasm.ISB:
		l.emit(core.OpNop, nil, nil, nil, core.EffectNone, ins.address)
	case armasm.MOV, armasm.MVN:
		l.move(ins)
	case armasm.MOVW, armasm.MOVT:
		l.moveWide(ins)
	case armasm.ADD, armasm.ADC, armasm.SUB, armasm.SBC, armasm.RSB, armasm.RSC,
		armasm.AND, armasm.ORR, armasm.EOR, armasm.BIC:
		l.binary(ins)
	case armasm.MUL, armasm.MLA, armasm.MLS:
		l.multiply(ins)
	case armasm.CMP, armasm.CMN, armasm.TST, armasm.TEQ:
		l.compare(ins)
	case armasm.LSL, armasm.LSR, armasm.ASR, armasm.ROR, armasm.RRX:
		l.shift(ins)
	case armasm.CLZ, armasm.REV, armasm.REV16, armasm.REVSH:
		l.intrinsicValue(ins, "arm."+lower(base.String()))
	case armasm.SDIV, armasm.UDIV:
		l.binary(ins)
	case armasm.UXTB, armasm.UXTH, armasm.SXTB, armasm.SXTH:
		l.extend(ins)
	case armasm.UMULL, armasm.UMLAL, armasm.SMULL, armasm.SMLAL:
		l.longMultiply(ins)
	case armasm.UBFX, armasm.SBFX, armasm.BFC, armasm.BFI:
		l.bitfield(ins)
	case armasm.RBIT:
		l.intrinsicValue(ins, "arm.rbit")
	case armasm.SSAT, armasm.USAT, armasm.SSAT16, armasm.USAT16:
		l.saturate(ins)
	case armasm.SMLABB, armasm.SMLABT, armasm.SMLATB, armasm.SMLATT,
		armasm.SMLAWB, armasm.SMLAWT, armasm.SMLALBB, armasm.SMLALBT,
		armasm.SMLALTB, armasm.SMLALTT, armasm.SMLAD, armasm.SMLAD_X,
		armasm.SMLSD, armasm.SMLSD_X, armasm.SMLALD, armasm.SMLALD_X,
		armasm.SMLSLD, armasm.SMLSLD_X, armasm.SMMLA, armasm.SMMLS, armasm.SMMUL,
		armasm.SMULBB, armasm.SMULBT, armasm.SMULTB, armasm.SMULTT,
		armasm.SMULWB, armasm.SMULWT, armasm.SMUAD, armasm.SMUAD_X,
		armasm.SMUSD, armasm.SMUSD_X:
		l.multiplyLongSigned(ins)
	case armasm.MRS, armasm.MSR:
		l.statusRegister(ins)
	case armasm.B, armasm.BL:
		l.branchOrCall(ins)
	case armasm.BLX:
		// BLX takes either a register or an immediate target.
		if _, _, ok := argRegister(ins.inst.Args[0]); ok {
			l.registerTransfer(ins)
			return
		}
		l.branchOrCall(ins)
	case armasm.BX:
		l.registerTransfer(ins)
	case armasm.LDR, armasm.LDRB, armasm.LDRH, armasm.LDRSB, armasm.LDRSH:
		l.load(ins)
	case armasm.LDRD:
		l.loadPair(ins)
	case armasm.LDREXD:
		l.loadPair(ins)
	case armasm.LDREX, armasm.LDREXB, armasm.LDREXH,
		armasm.LDRT, armasm.LDRBT, armasm.LDRHT, armasm.LDRSBT, armasm.LDRSHT:
		// Exclusive and unprivileged loads read memory like LDR; the
		// exclusivity monitor is not modeled.
		l.load(ins)
	case armasm.STR, armasm.STRB, armasm.STRH:
		l.store(ins)
	case armasm.STRD:
		l.storePair(ins)
	case armasm.STREXD, armasm.STREXB, armasm.STREX, armasm.STREXH:
		l.exclusiveStore(ins)
	case armasm.STRT, armasm.STRBT, armasm.STRHT:
		l.store(ins)
	case armasm.LDM, armasm.LDMDA, armasm.LDMDB, armasm.LDMIB:
		l.loadMultiple(ins)
	case armasm.STM, armasm.STMDA, armasm.STMDB, armasm.STMIB:
		l.storeMultiple(ins)
	case armasm.PUSH:
		l.push(ins)
	case armasm.POP:
		l.pop(ins)
	case armasm.UNDEF:
		l.emit(core.OpTrap, []core.Operand{symbolOperand("arm.undef")}, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
	case armasm.SVC, armasm.BKPT:
		l.emit(core.OpTrap, []core.Operand{symbolOperand("arm." + lower(base.String()))}, nil, nil, core.EffectMayTrap|core.EffectUnknown|core.EffectCall, ins.address)
	default:
		if isCoprocessor(ins.inst.Op) {
			l.coprocessor(ins)
			return
		}
		l.unsupported(ins)
	}
}

// systemRegisterTransfer lifts one MRC or MCR. A read writes the general
// register; a write reads it.
func (l *lifter) systemRegisterTransfer(ins instruction) {
	transfer := ins.systemRegister
	register := fmt.Sprintf("r%d", transfer.register)
	operands := []core.Operand{symbolOperand("arm." + transfer.mnemonic)}
	var results []core.Value
	effects := core.EffectUnknown
	if transfer.read {
		value := l.newValue(core.TypeInteger, 32)
		results = append(results, value)
		operands = append(operands, core.Operand{Kind: core.OperandRegister, Type: value.Type, Name: register})
		effects |= core.EffectWriteRegister
	} else {
		operands = append(operands, l.value(l.readRegister(ins.address, register, 32)))
		effects |= core.EffectReadRegister
	}
	l.emit(core.OpIntrinsic, operands, results, nil, effects, ins.address)
}

// isCoprocessor reports whether the mnemonic belongs to the VFP, NEON or
// coprocessor space. armasm names those ops as a family with a data-type suffix
// (VADD_F32, VLDR, VMOV, ...) instead of the sixteen-condition scheme the
// scalar instructions use, so the name is the reliable test.
func isCoprocessor(op armasm.Op) bool {
	name := op.String()
	return len(name) > 0 && name[0] == 'V'
}

// unsupported keeps one instruction in the IR as an intrinsic with the raw
// operands, then reports it once.
func (l *lifter) unsupported(ins instruction) {
	operands := []core.Operand{symbolOperand("arm." + lower((baseOp(ins.inst.Op)).String()))}
	for index, arg := range ins.inst.Args {
		if arg == nil {
			break
		}
		if operand, ok := l.rawOperand(ins, index, arg); ok {
			operands = append(operands, operand)
		}
	}
	l.emit(core.OpIntrinsic, operands, nil, nil, core.EffectUnknown, ins.address)
	l.diagnostics.add("unsupported-instruction", fmt.Sprintf("unsupported arm instruction %s at 0x%x", ins.inst.Op, ins.address))
}

func (l *lifter) rawOperand(ins instruction, index int, arg armasm.Arg) (core.Operand, bool) {
	if name, bits, ok := argRegister(arg); ok {
		return l.value(l.readRegister(ins.address, name, bits)), true
	}
	if value, ok := immediateValue(arg); ok {
		return l.value(l.constant(ins.address, value, 32)), true
	}
	if base, _, _, offset, _, ok := memoryAddress(arg); ok {
		return l.value(l.addressFromBase(ins, base, offset)), true
	}
	if offset, ok := pcRelative(arg); ok {
		return l.value(l.pointerConstant(ins.address, ins.address+8+uint64(offset))), true
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
		bits = 32
	}
	l.nextValue++
	value := core.Value{ID: l.nextValue, Type: core.ValueType{Kind: kind, Bits: bits}}
	l.values[value.ID] = value.Type
	return value
}

func (l *lifter) value(id core.ValueID) core.Operand {
	typ, ok := l.values[id]
	if !ok {
		typ = core.ValueType{Kind: core.TypeInteger, Bits: 32}
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
		bits = 32
	}
	result := l.newValue(core.TypeInteger, bits)
	l.emit(core.OpConstant, []core.Operand{{
		Kind:    core.OperandConstant,
		Type:    result.Type,
		Literal: literal(value, bits),
	}}, []core.Value{result}, nil, core.EffectNone, address)
	return result.ID
}

func (l *lifter) pointerConstant(address, value uint64) core.ValueID {
	result := l.newValue(core.TypePointer, 32)
	l.emit(core.OpConstant, []core.Operand{{
		Kind:    core.OperandConstant,
		Type:    result.Type,
		Literal: literal(value, 32),
	}}, []core.Value{result}, nil, core.EffectNone, address)
	return result.ID
}

func (l *lifter) readRegister(address uint64, name string, bits uint16) core.ValueID {
	result := l.newValue(core.TypeInteger, bits)
	l.emit(core.OpReadRegister, []core.Operand{registerOperand(name, bits)}, []core.Value{result}, nil, core.EffectReadRegister, address)
	return result.ID
}

func (l *lifter) writeRegister(address uint64, name string, bits uint16, value core.ValueID) {
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
	result := l.newValue(core.TypePointer, 32)
	l.emit(core.OpCopy, []core.Operand{l.value(value)}, []core.Value{result}, nil, core.EffectNone, address)
	return result.ID
}

func (l *lifter) addAddress(ins instruction, left, right core.ValueID) core.ValueID {
	result := l.newValue(core.TypePointer, 32)
	l.emit(core.OpAdd, []core.Operand{l.value(left), l.value(right)}, []core.Value{result}, nil, core.EffectNone, ins.address)
	return result.ID
}

func (l *lifter) addressFromBase(ins instruction, base string, displacement int64) core.ValueID {
	address := l.pointerFrom(ins.address, l.readRegister(ins.address, base, 32))
	if displacement == 0 {
		return address
	}
	kind := core.OpAdd
	amount := uint64(displacement)
	if displacement < 0 {
		kind = core.OpSub
		amount = uint64(-displacement)
	}
	offset := l.constant(ins.address, amount, 32)
	return l.operation(ins, kind, []core.Operand{l.value(address), l.value(offset)}, 32, core.EffectNone)
}
