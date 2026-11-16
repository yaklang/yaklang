package mips

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// lifter converts one function body into LowIR. Registers keep their ABI names
// (a0, sp, ra, ...) because that is how MIPS assembly and the ABI refer to them.
type lifter struct {
	profile      Profile
	blockIDs     map[uint64]core.BlockID
	diagnostics  *diagnostics
	nextValue    core.ValueID
	values       map[core.ValueID]core.ValueType
	instructions []core.Instruction
}

func liftFunctions(profile Profile, target core.Target, functions []function, blockIDs map[uint64]core.BlockID, diagnostics *diagnostics) *core.LowIR {
	ir := &core.LowIR{Target: target, Frame: profile.Frame(), Registers: profile.Registers()}
	for index, body := range functions {
		ir.Functions = append(ir.Functions, liftFunction(profile, body, core.FunctionID(index+1), blockIDs, diagnostics))
	}
	return ir
}

func liftFunction(profile Profile, body function, id core.FunctionID, blockIDs map[uint64]core.BlockID, diagnostics *diagnostics) core.Function {
	result := core.Function{
		ID:      id,
		Address: body.blocks[0].start,
		Blocks:  make([]core.Block, 0, len(body.blocks)),
	}
	lifter := &lifter{
		profile:     profile,
		blockIDs:    blockIDs,
		diagnostics: diagnostics,
		values:      make(map[core.ValueID]core.ValueType),
	}
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
		l.emit(core.OpTrap, []core.Operand{symbolOperand("mips.data")}, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
		return
	}
	name := ins.op
	switch {
	case name == "nop" || name == "ssnop" || name == "ehb":
		l.emit(core.OpNop, nil, nil, nil, core.EffectNone, ins.address)
		return
	case isReturn(name):
		l.emit(core.OpReturn, nil, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
		return
	case isCall(name):
		l.call(ins)
		return
	case isJump(name):
		l.jump(ins)
		return
	case isBranch(name):
		l.branch(ins)
		return
	case isSystemLike(name):
		l.system(ins)
		return
	case isLoad(name):
		l.load(ins)
		return
	case isStore(name):
		l.store(ins)
		return
	case isFloat(name):
		l.float(ins)
		return
	}
	if value, ok := l.arithmetic(ins); ok {
		_ = value
		return
	}
	l.unsupported(ins)
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
		bits = l.profile.Bits
	}
	l.nextValue++
	value := core.Value{ID: l.nextValue, Type: core.ValueType{Kind: kind, Bits: bits}}
	l.values[value.ID] = value.Type
	return value
}

func (l *lifter) value(id core.ValueID) core.Operand {
	typ, ok := l.values[id]
	if !ok {
		typ = core.ValueType{Kind: core.TypeInteger, Bits: l.profile.Bits}
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
		bits = l.profile.Bits
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
	result := l.newValue(core.TypePointer, l.profile.Bits)
	l.emit(core.OpConstant, []core.Operand{{
		Kind:    core.OperandConstant,
		Type:    result.Type,
		Literal: literal(value, l.profile.Bits),
	}}, []core.Value{result}, nil, core.EffectNone, address)
	return result.ID
}

func (l *lifter) readRegister(address uint64, name string, bits uint16) core.ValueID {
	if name == "zero" {
		return l.constant(address, 0, bits)
	}
	result := l.newValue(core.TypeInteger, bits)
	l.emit(core.OpReadRegister, []core.Operand{registerOperand(name, bits)}, []core.Value{result}, nil, core.EffectReadRegister, address)
	return result.ID
}

func (l *lifter) writeRegister(address uint64, name string, bits uint16, value core.ValueID) {
	if name == "zero" {
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
	result := l.newValue(core.TypePointer, l.profile.Bits)
	l.emit(core.OpCopy, []core.Operand{l.value(value)}, []core.Value{result}, nil, core.EffectNone, address)
	return result.ID
}

func (l *lifter) addAddress(ins instruction, left, right core.ValueID) core.ValueID {
	result := l.newValue(core.TypePointer, l.profile.Bits)
	l.emit(core.OpAdd, []core.Operand{l.value(left), l.value(right)}, []core.Value{result}, nil, core.EffectNone, ins.address)
	return result.ID
}

func (l *lifter) addressFromBase(ins instruction, base string, displacement int64) core.ValueID {
	address := l.pointerFrom(ins.address, l.readRegister(ins.address, base, l.profile.Bits))
	if displacement == 0 {
		return address
	}
	kind := core.OpAdd
	amount := uint64(displacement)
	if displacement < 0 {
		kind = core.OpSub
		amount = uint64(-displacement)
	}
	offset := l.constant(ins.address, amount, l.profile.Bits)
	return l.operation(ins, kind, []core.Operand{l.value(address), l.value(offset)}, l.profile.Bits, core.EffectNone)
}

// unsupported keeps one instruction in the IR as an intrinsic with its raw
// operands, then reports it once.
func (l *lifter) unsupported(ins instruction) {
	operands := []core.Operand{symbolOperand("mips." + ins.op)}
	for _, item := range ins.operands {
		switch item.kind {
		case operandRegister:
			operands = append(operands, l.value(l.readRegister(ins.address, registerName(item.register), l.profile.Bits)))
		case operandImmediate:
			operands = append(operands, l.value(l.constant(ins.address, uint64(item.immediat), l.profile.Bits)))
		case operandMemory:
			operands = append(operands, l.value(l.addressFromBase(ins, registerName(item.base), item.immediat)))
		}
	}
	l.emit(core.OpIntrinsic, operands, nil, nil, core.EffectUnknown, ins.address)
	l.diagnostics.add("unsupported-instruction",
		fmt.Sprintf("unsupported mips instruction %s at 0x%x", ins.op, ins.address))
}

// controlTransfer reports whether an opcode ends a basic block.
func controlTransfer(kind core.Opcode) bool {
	switch kind {
	case core.OpJump, core.OpBranch, core.OpReturn, core.OpTrap:
		return true
	default:
		return false
	}
}

func containsBlock(blocks []core.BlockID, target core.BlockID) bool {
	for _, block := range blocks {
		if block == target {
			return true
		}
	}
	return false
}

var _ = strings.ToLower
