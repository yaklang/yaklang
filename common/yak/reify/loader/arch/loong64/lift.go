package loong64

import (
	"fmt"
	"strings"

	"golang.org/x/arch/loong64/loong64asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// lifter converts one function body into LowIR. Registers keep their numeric
// names (r0..r31, f0..f31) so the text does not depend on the ABI spelling.
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
	lifter := &lifter{
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
		l.emit(core.OpTrap, []core.Operand{symbolOperand("loong64.data")}, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
		return
	}
	name := nameOf(ins.inst.Op)
	switch {
	case isUnknownOpcode(name):
		l.emit(core.OpTrap, []core.Operand{symbolOperand("loong64.data")}, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
		return
	case isSystemLike(name):
		l.system(ins)
		return
	case isAtomic(name):
		l.atomic(ins)
		return
	case name == "ll.w" || name == "ll.d" || name == "sc.w" || name == "sc.d":
		// The load-linked and store-conditional forms read and write memory.
		if base, displacement, ok := memoryOperand(args(ins.inst)); ok {
			_ = base
			_ = displacement
		}
		l.atomic(ins)
		return
	case name == "maskeqz" || name == "masknez":
		l.selectMask(ins, name)
		return
	case isCall(name):
		l.call(ins)
		return
	case isBranch(name):
		l.branch(ins)
		return
	case isJump(name):
		l.jump(ins)
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
		bits = addressBits
	}
	l.nextValue++
	value := core.Value{ID: l.nextValue, Type: core.ValueType{Kind: kind, Bits: bits}}
	l.values[value.ID] = value.Type
	return value
}

func (l *lifter) value(id core.ValueID) core.Operand {
	typ, ok := l.values[id]
	if !ok {
		typ = core.ValueType{Kind: core.TypeInteger, Bits: addressBits}
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
		bits = addressBits
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
	result := l.newValue(core.TypePointer, addressBits)
	l.emit(core.OpConstant, []core.Operand{{
		Kind:    core.OperandConstant,
		Type:    result.Type,
		Literal: literal(value, addressBits),
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
	if size > 8 {
		size = 8
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
	result := l.newValue(core.TypePointer, addressBits)
	l.emit(core.OpCopy, []core.Operand{l.value(value)}, []core.Value{result}, nil, core.EffectNone, address)
	return result.ID
}

func (l *lifter) addAddress(ins instruction, left, right core.ValueID) core.ValueID {
	result := l.newValue(core.TypePointer, addressBits)
	l.emit(core.OpAdd, []core.Operand{l.value(left), l.value(right)}, []core.Value{result}, nil, core.EffectNone, ins.address)
	return result.ID
}

// addressFromBase builds the effective address of one memory operand.
func (l *lifter) addressFromBase(ins instruction, base string, displacement int64) core.ValueID {
	address := l.pointerFrom(ins.address, l.readRegister(ins.address, base, addressBits))
	if displacement == 0 {
		return address
	}
	kind := core.OpAdd
	amount := uint64(displacement)
	if displacement < 0 {
		kind = core.OpSub
		amount = uint64(-displacement)
	}
	offset := l.constant(ins.address, amount, addressBits)
	return l.operation(ins, kind, []core.Operand{l.value(address), l.value(offset)}, addressBits, core.EffectNone)
}

// unsupported keeps one instruction in the IR as an intrinsic with its raw
// operands, then reports it once.
func (l *lifter) unsupported(ins instruction) {
	operands := []core.Operand{symbolOperand("loong64." + nameOf(ins.inst.Op))}
	for _, arg := range args(ins.inst) {
		if operand, ok := l.rawOperand(ins, arg); ok {
			operands = append(operands, operand)
		}
	}
	l.emit(core.OpIntrinsic, operands, nil, nil, core.EffectUnknown, ins.address)
	l.diagnostics.add("unsupported-instruction",
		fmt.Sprintf("unsupported loong64 instruction %s at 0x%x", nameOf(ins.inst.Op), ins.address))
}

// rawOperand converts one library argument into a LowIR operand.
func (l *lifter) rawOperand(ins instruction, arg loong64asm.Arg) (core.Operand, bool) {
	if name, bits, ok := argRegister(arg); ok {
		return l.value(l.readRegister(ins.address, name, bits)), true
	}
	if value, ok := signedImmediate(arg); ok {
		return l.value(l.constant(ins.address, uint64(value), addressBits)), true
	}
	return core.Operand{}, false
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

// --- control flow -----------------------------------------------------------

// call lifts a call. LoongArch writes the return address into r1.
func (l *lifter) call(ins instruction) {
	target, ok := branchTarget(nameOf(ins.inst.Op), args(ins.inst), ins.address)
	if !ok {
		l.unsupported(ins)
		return
	}
	l.emit(core.OpCall, []core.Operand{pointerOperand(target)}, nil, nil,
		core.EffectCall|core.EffectMayTrap|core.EffectUnknown, ins.address)
}

// branch lifts a conditional branch.
func (l *lifter) branch(ins instruction) {
	target, ok := branchTarget(nameOf(ins.inst.Op), args(ins.inst), ins.address)
	if !ok {
		l.unsupported(ins)
		return
	}
	operands := args(ins.inst)
	name := nameOf(ins.inst.Op)
	conditionValue, ok := l.condition(ins, name, operands)
	if !ok {
		l.unsupported(ins)
		return
	}
	targets := make([]core.BlockID, 0, 2)
	if block, ok := l.blockIDs[target]; ok {
		targets = append(targets, block)
	}
	next := ins.address + uint64(ins.size)
	if block, ok := l.blockIDs[next]; ok && !containsBlock(targets, block) {
		targets = append(targets, block)
	}
	effects := core.EffectNone
	if len(targets) == 0 {
		effects = core.EffectUnknown
	}
	l.emit(core.OpBranch, []core.Operand{l.value(conditionValue)}, nil, targets, effects, ins.address)
}

// condition materializes the comparison one branch performs.
func (l *lifter) condition(ins instruction, name string, operands []loong64asm.Arg) (core.ValueID, bool) {
	kind := core.OpEqual
	negated := false
	switch trimmed(name) {
	case "beq", "beqz":
		kind = core.OpEqual
	case "bceqz":
		// The floating-point condition test reads the FCC register, which has
		// no data-flow model.
		kind = core.OpEqual
	case "bne", "bnez", "bcnez":
		kind = core.OpEqual
		negated = true
	case "blt", "bltz":
		kind = core.OpSLess
	case "bge", "bgez":
		kind = core.OpSLess
		negated = true
	case "bltu":
		kind = core.OpULess
	case "bgeu":
		kind = core.OpULess
		negated = true
	case "bgtz":
		kind = core.OpSLess
		// bgtz rX is r0 < rX, so the operands swap below.
	case "blez":
		kind = core.OpSLess
		negated = true
	}
	registers := make([]core.ValueID, 0, 2)
	for _, arg := range operands {
		if _, _, ok := argRegister(arg); !ok {
			continue
		}
		value, ok := l.source(ins, arg, addressBits)
		if !ok {
			continue
		}
		registers = append(registers, value)
	}
	if len(registers) == 0 {
		return 0, false
	}
	left := registers[0]
	right := core.ValueID(0)
	if len(registers) >= 2 {
		right = registers[1]
	} else {
		right = l.constant(ins.address, 0, addressBits)
	}
	switch trimmed(name) {
	case "bgtz":
		left, right = right, left
	case "blez":
		left, right = right, left
		negated = false
	}
	result := l.operation(ins, kind, []core.Operand{l.value(left), l.value(right)}, 1, core.EffectNone)
	if negated {
		result = l.operation(ins, core.OpNot, []core.Operand{l.value(result)}, 1, core.EffectNone)
	}
	return result, true
}

// jump lifts an unconditional transfer.
func (l *lifter) jump(ins instruction) {
	name := nameOf(ins.inst.Op)
	if trimmed(name) == "jr" || trimmed(name) == "ret" {
		// A jump through the return address register is the epilogue.
		operands := args(ins.inst)
		if len(operands) == 0 || isReturnRegister(operands[0]) {
			l.emit(core.OpReturn, nil, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
			return
		}
	}
	if trimmed(name) == "jirl" {
		operands := args(ins.inst)
		if len(operands) >= 2 && isReturnRegister(operands[1]) {
			l.emit(core.OpReturn, nil, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
			return
		}
		if len(operands) >= 2 {
			if value, ok := l.source(ins, operands[1], addressBits); ok {
				pointer := l.pointerFrom(ins.address, value)
				l.emit(core.OpJump, []core.Operand{l.value(pointer)}, nil, nil, core.EffectUnknown, ins.address)
				return
			}
		}
		l.unsupported(ins)
		return
	}
	target, ok := branchTarget(name, args(ins.inst), ins.address)
	if !ok {
		l.unsupported(ins)
		return
	}
	if block, ok := l.blockIDs[target]; ok {
		l.emit(core.OpJump, nil, nil, []core.BlockID{block}, core.EffectNone, ins.address)
		return
	}
	l.emit(core.OpJump, []core.Operand{pointerOperand(target)}, nil, nil, core.EffectUnknown, ins.address)
}

// isReturnRegister reports whether one operand is the link register r1.
func isReturnRegister(arg loong64asm.Arg) bool {
	reg, ok := arg.(loong64asm.Reg)
	if !ok {
		return false
	}
	return reg == loong64asm.R1
}

// selectMask lifts maskeqz and masknez: the result is the first source when the
// second is zero (or non-zero), and zero otherwise.
func (l *lifter) selectMask(ins instruction, name string) {
	operands := args(ins.inst)
	if len(operands) != 3 {
		l.unsupported(ins)
		return
	}
	destination, bits, ok := argRegister(operands[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	source, ok := l.source(ins, operands[1], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	mask, ok := l.source(ins, operands[2], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	result := l.operation(ins, core.OpIntrinsic, []core.Operand{
		symbolOperand("loong64." + name), l.value(source), l.value(mask),
	}, bits, core.EffectNone)
	l.writeRegister(ins.address, destination, bits, result)
}

// system lifts one system instruction.
func (l *lifter) system(ins instruction) {
	name := nameOf(ins.inst.Op)
	switch trimmed(name) {
	case "break", "syscall":
		l.emit(core.OpTrap, []core.Operand{symbolOperand("loong64." + name)}, nil, nil,
			core.EffectMayTrap|core.EffectUnknown|core.EffectCall, ins.address)
	case "dbar", "ibar", "idle", "preld":
		l.emit(core.OpNop, nil, nil, nil, core.EffectNone, ins.address)
	default:
		operands := []core.Operand{symbolOperand("loong64." + name)}
		for _, arg := range args(ins.inst) {
			if operand, ok := l.rawOperand(ins, arg); ok {
				operands = append(operands, operand)
			}
		}
		l.emit(core.OpIntrinsic, operands, nil, nil, core.EffectUnknown, ins.address)
	}
}

// atomic lifts one atomic memory operation.
func (l *lifter) atomic(ins instruction) {
	operands := []core.Operand{symbolOperand("loong64." + nameOf(ins.inst.Op))}
	var results []core.Value
	effects := core.EffectUnknown | core.EffectAtomic
	for index, arg := range args(ins.inst) {
		value, ok := l.source(ins, arg, addressBits)
		if !ok {
			continue
		}
		operands = append(operands, l.value(value))
		if index == 0 {
			result := l.newValue(core.TypeInteger, addressBits)
			results = append(results, result)
			effects |= core.EffectWriteRegister
		}
	}
	l.emit(core.OpIntrinsic, operands, results, nil, effects, ins.address)
}

// float lifts one floating-point or vector instruction as an opaque operation.
func (l *lifter) float(ins instruction) {
	operands := []core.Operand{symbolOperand("loong64." + nameOf(ins.inst.Op))}
	var results []core.Value
	effects := core.EffectUnknown
	for index, arg := range args(ins.inst) {
		name, bits, ok := argRegister(arg)
		if !ok {
			if value, immediate := signedImmediate(arg); immediate {
				operands = append(operands, l.value(l.constant(ins.address, uint64(value), addressBits)))
			}
			continue
		}
		if index == 0 {
			value := l.newValue(core.TypeInteger, bits)
			results = append(results, value)
			operands = append(operands, core.Operand{Kind: core.OperandRegister, Type: value.Type, Name: name})
			effects |= core.EffectWriteRegister
			continue
		}
		operands = append(operands, core.Operand{
			Kind: core.OperandRegister,
			Type: core.ValueType{Kind: core.TypeInteger, Bits: bits},
			Name: name,
		})
		effects |= core.EffectReadRegister
	}
	l.emit(core.OpIntrinsic, operands, results, nil, effects, ins.address)
}

// pointerOperand builds one pointer constant operand.
func pointerOperand(value uint64) core.Operand {
	return core.Operand{
		Kind:    core.OperandConstant,
		Type:    core.ValueType{Kind: core.TypePointer, Bits: 64},
		Literal: literal(value, 64),
	}
}
