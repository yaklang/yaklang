package s390x

import (
	"fmt"
	"strings"

	"golang.org/x/arch/s390x/s390xasm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// lifter converts one function body into LowIR. Registers keep their z/Architecture
// names (r0..r15, f0..f15, ...) because that is how the assembly refers to them.
type lifter struct {
	blockIDs     map[uint64]core.BlockID
	diagnostics  *diagnostics
	nextValue    core.ValueID
	values       map[core.ValueID]core.ValueType
	instructions []core.Instruction
}

func liftFunctions(target core.Target, functions []function, blockIDs map[uint64]core.BlockID, diagnostics *diagnostics) *core.LowIR {
	ir := &core.LowIR{Target: target, Frame: frame(), Registers: registers()}
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
		l.emit(core.OpTrap, []core.Operand{symbolOperand("s390x.data")}, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
		return
	}
	name := nameOf(ins.inst.Op)
	switch {
	case isUnknownOpcode(name):
		// The decoder matched no format. An unmapped word inside .text is
		// data: padding, a literal or a jump table entry.
		l.emit(core.OpTrap, []core.Operand{symbolOperand("s390x.data")}, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
		return
	case isStoreImmediate(name):
		// mvghi, mvi and the moves write a constant into memory. They must be
		// routed before the memory dispatch, whose load and store tests look at
		// the mnemonic prefix only.
		if base, index, displacement, ok := memoryOperand(args(ins.inst)); ok {
			l.memory(ins, base, index, displacement)
			return
		}
		fallthrough
	case isSystemLike(name):
		l.system(ins)
		return
	case name == "br" || (name == "bcr" && isUnconditionalMask(ins)):
		// br and the unconditional bcr share the encoding. A mask of zero is
		// the "no branch" form, which is a no-op.
		if isZeroMask(ins) {
			l.emit(core.OpNop, nil, nil, nil, core.EffectNone, ins.address)
			return
		}
		l.registerBranch(ins)
		return
	case isCall(name):
		l.call(ins)
		return
	case isBranch(name) && isConditionalBranch(ins.inst) && !isCompareBranch(name):
		l.branch(ins)
		return
	case isRegisterBranch(name):
		l.registerBranch(ins)
		return
	case isVector(name):
		l.vector(ins)
		return
	case isFloat(name):
		l.float(ins)
		return
	}
	if _, ok := shiftOpcode(name); ok {
		if value, ok := l.shift(ins); ok {
			_ = value
			return
		}
	}
	if value, ok := l.branchOnCount(ins); ok {
		_ = value
		return
	}
	if value, ok := l.immediateForm(ins); ok {
		_ = value
		return
	}

	// A displacement operand decides: the memory forms read or write the
	// address it describes, and the register forms have none. Testing memory
	// first keeps the base register of mvghi from being read as a destination.
	if base, index, displacement, ok := memoryOperand(args(ins.inst)); ok {
		l.memory(ins, base, index, displacement)
		return
	}
	if value, ok := l.registers(ins); ok {
		_ = value
		return
	}
	if value, ok := l.opaqueRegisterForm(ins); ok {
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
func (l *lifter) addressFromBase(ins instruction, base, index string, displacement int64) core.ValueID {
	var address core.ValueID
	if base == "" || base == "zero" {
		address = l.pointerConstant(ins.address, uint64(displacement))
	} else {
		address = l.pointerFrom(ins.address, l.readRegister(ins.address, base, addressBits))
		if displacement != 0 {
			kind := core.OpAdd
			amount := uint64(displacement)
			if displacement < 0 {
				kind = core.OpSub
				amount = uint64(-displacement)
			}
			offset := l.constant(ins.address, amount, addressBits)
			address = l.operation(ins, kind, []core.Operand{l.value(address), l.value(offset)}, addressBits, core.EffectNone)
		}
	}
	if index != "" {
		indexValue := l.readRegister(ins.address, index, addressBits)
		address = l.addAddress(ins, address, indexValue)
	}
	return address
}

// unsupported keeps one instruction in the IR as an intrinsic with its raw
// operands, then reports it once.
func (l *lifter) unsupported(ins instruction) {
	operands := []core.Operand{symbolOperand("s390x." + nameOf(ins.inst.Op))}
	for _, arg := range args(ins.inst) {
		if operand, ok := l.rawOperand(ins, arg); ok {
			operands = append(operands, operand)
		}
	}
	l.emit(core.OpIntrinsic, operands, nil, nil, core.EffectUnknown, ins.address)
	l.diagnostics.add("unsupported-instruction",
		fmt.Sprintf("unsupported s390x instruction %s at 0x%x", nameOf(ins.inst.Op), ins.address))
}

// rawOperand converts one library argument into a LowIR operand.
func (l *lifter) rawOperand(ins instruction, arg s390xasm.Arg) (core.Operand, bool) {
	switch value := arg.(type) {
	case s390xasm.Reg:
		name, bits, ok := registerName(value)
		if !ok {
			return core.Operand{}, false
		}
		return l.value(l.readRegister(ins.address, name, bits)), true
	case s390xasm.VReg:
		name, bits, ok := vectorRegisterName(value)
		if !ok {
			return core.Operand{}, false
		}
		return l.value(l.readRegister(ins.address, name, bits)), true
	case s390xasm.Sign8:
		return l.value(l.constant(ins.address, uint64(int64(int8(value))), addressBits)), true
	case s390xasm.Sign16:
		return l.value(l.constant(ins.address, uint64(int64(int16(value))), addressBits)), true
	case s390xasm.Sign32:
		return l.value(l.constant(ins.address, uint64(int64(int32(value))), addressBits)), true
	case s390xasm.Imm:
		return l.value(l.constant(ins.address, uint64(uint32(value)), addressBits)), true
	default:
		return core.Operand{}, false
	}
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

// --- control flow -----------------------------------------------------------

// call lifts the branch-and-save forms. The return address register is the
// first operand; brasl writes the target into the second.
func (l *lifter) call(ins instruction) {
	target, ok := branchTarget(ins.inst, ins.address, ins.size)
	if !ok {
		// A branch-and-save through a register: the target is in a register, so
		// the call stays dynamic.
		arguments := args(ins.inst)
		for _, arg := range arguments {
			register, isRegister := arg.(s390xasm.Reg)
			if !isRegister {
				continue
			}
			name, bits, ok := registerName(register)
			if !ok {
				continue
			}
			value := l.pointerFrom(ins.address, l.readRegister(ins.address, name, bits))
			l.emit(core.OpCall, []core.Operand{l.value(value)}, nil, nil,
				core.EffectCall|core.EffectMayTrap|core.EffectUnknown, ins.address)
			return
		}
		l.unsupported(ins)
		return
	}
	l.emit(core.OpCall, []core.Operand{pointerOperand(target)}, nil, nil,
		core.EffectCall|core.EffectMayTrap|core.EffectUnknown, ins.address)
}

// branch lifts a conditional branch. The mask operand selects the condition,
// so the edge stays conditional.
func (l *lifter) branch(ins instruction) {
	target, ok := branchTarget(ins.inst, ins.address, ins.size)
	if !ok {
		l.unsupported(ins)
		return
	}
	mask := l.conditionMask(ins)
	conditionValue := l.operation(ins, core.OpIntrinsic, []core.Operand{
		symbolOperand("cond." + conditionName(mask)), l.value(l.readRegister(ins.address, "cc", 2)),
	}, 1, core.EffectReadRegister)
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

// registerBranch lifts br and its conditional form. The target comes from a
// register, so the edge stays dynamic.
func (l *lifter) registerBranch(ins instruction) {
	arguments := args(ins.inst)
	// The condition mask comes first for bcr, so the register is the last one.
	target := core.ValueID(0)
	for _, arg := range arguments {
		register, ok := arg.(s390xasm.Reg)
		if !ok {
			continue
		}
		name, bits, ok := registerName(register)
		if !ok {
			continue
		}
		if name == "r14" {
			// br %r14 is the function epilogue.
			l.emit(core.OpReturn, nil, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
			return
		}
		target = l.pointerFrom(ins.address, l.readRegister(ins.address, name, bits))
	}
	if target == 0 {
		l.unsupported(ins)
		return
	}
	if isConditionalBranch(ins.inst) {
		mask := l.conditionMask(ins)
		conditionValue := l.operation(ins, core.OpIntrinsic, []core.Operand{
			symbolOperand("cond." + conditionName(mask)), l.value(l.readRegister(ins.address, "cc", 2)),
		}, 1, core.EffectReadRegister)
		l.emit(core.OpBranch, []core.Operand{l.value(conditionValue), l.value(target)}, nil, nil, core.EffectUnknown, ins.address)
		return
	}
	l.emit(core.OpJump, []core.Operand{l.value(target)}, nil, nil, core.EffectUnknown, ins.address)
}

// maskOf returns the condition mask of one branch, or -1 when it has none.
func maskOf(ins instruction) int {
	for _, arg := range args(ins.inst) {
		if mask, ok := arg.(s390xasm.Mask); ok {
			return int(uint8(mask))
		}
	}
	return -1
}

// isUnconditionalMask reports whether the mask selects every condition.
func isUnconditionalMask(ins instruction) bool {
	return maskOf(ins) == 15
}

// isZeroMask reports whether the mask suppresses the branch.
func isZeroMask(ins instruction) bool {
	return maskOf(ins) == 0
}

// conditionMask renders the mask operand of one branch.
func (l *lifter) conditionMask(ins instruction) string {
	for _, arg := range args(ins.inst) {
		if mask, ok := arg.(s390xasm.Mask); ok {
			return fmt.Sprintf("%d", uint8(mask))
		}
	}
	return "0"
}

// system lifts a system or serialization instruction. A supervisor call is a
// trap; the rest keep their mnemonic.
func (l *lifter) system(ins instruction) {
	name := nameOf(ins.inst.Op)
	switch name {
	case "svc", "trap2", "trap4":
		l.emit(core.OpTrap, []core.Operand{symbolOperand("s390x." + name)}, nil, nil,
			core.EffectMayTrap|core.EffectUnknown|core.EffectCall, ins.address)
	case "nop", "nopr":
		l.emit(core.OpNop, nil, nil, nil, core.EffectNone, ins.address)
	default:
		operands := []core.Operand{symbolOperand("s390x." + name)}
		for _, arg := range args(ins.inst) {
			if operand, ok := l.rawOperand(ins, arg); ok {
				operands = append(operands, operand)
			}
		}
		l.emit(core.OpIntrinsic, operands, nil, nil, core.EffectUnknown, ins.address)
	}
}

// float lifts one floating-point instruction as an opaque operation: the FPU
// register file has no data-flow model.
func (l *lifter) float(ins instruction) {
	operands := []core.Operand{symbolOperand("s390x." + nameOf(ins.inst.Op))}
	var results []core.Value
	effects := core.EffectUnknown
	for index, arg := range args(ins.inst) {
		operand, ok := l.rawOperand(ins, arg)
		if !ok {
			continue
		}
		if index == 0 && operand.Kind == core.OperandValue {
			value := l.newValue(core.TypeFloat, 64)
			results = append(results, value)
			operands = append(operands, core.Operand{Kind: core.OperandValue, Type: value.Type, Value: value.ID})
			effects |= core.EffectWriteRegister
			continue
		}
		operands = append(operands, operand)
		effects |= core.EffectReadRegister
	}
	l.emit(core.OpIntrinsic, operands, results, nil, effects, ins.address)
}

// vector lifts one vector instruction as an opaque operation.
func (l *lifter) vector(ins instruction) {
	operands := []core.Operand{symbolOperand("s390x." + nameOf(ins.inst.Op))}
	effects := core.EffectUnknown
	for index, arg := range args(ins.inst) {
		if register, ok := arg.(s390xasm.VReg); ok {
			name, bits, ok := vectorRegisterName(register)
			if !ok {
				continue
			}
			if index == 0 {
				value := l.newValue(core.TypeInteger, bits)
				operands = append(operands, core.Operand{Kind: core.OperandRegister, Type: value.Type, Name: name})
				l.emit(core.OpIntrinsic, operands, []core.Value{value}, nil, effects|core.EffectWriteRegister, ins.address)
				return
			}
			operands = append(operands, core.Operand{
				Kind: core.OperandRegister,
				Type: core.ValueType{Kind: core.TypeInteger, Bits: bits},
				Name: name,
			})
			effects |= core.EffectReadRegister
			continue
		}
		if operand, ok := l.rawOperand(ins, arg); ok {
			operands = append(operands, operand)
		}
	}
	l.emit(core.OpIntrinsic, operands, nil, nil, effects, ins.address)
}

// pointerOperand builds one pointer constant operand.
func pointerOperand(value uint64) core.Operand {
	return core.Operand{
		Kind:    core.OperandConstant,
		Type:    core.ValueType{Kind: core.TypePointer, Bits: 64},
		Literal: literal(value, 64),
	}
}

// --- immediate forms --------------------------------------------------------

// immediateForm lifts the register-and-immediate forms. The z/Architecture
// writes them with an i in the mnemonic: lghi, aghi, clfi, tmll and so on.
func (l *lifter) immediateForm(ins instruction) (core.ValueID, bool) {
	name := nameOf(ins.inst.Op)
	arguments := args(ins.inst)
	if len(arguments) < 2 {
		return 0, false
	}
	destination, bits, ok := firstRegister(arguments)
	if !ok {
		bits = addressBits
	}
	switch name {
	case "stgrl":
		// The store-and-immediate forms write the register to memory; the
		// addressing is handled by the memory path.
		return 0, false
	case "lghi", "lhi", "lgrl", "lgfrl", "llgfrl", "llghrl", "llgcrl", "lgbrl",
		"lrl", "llhrl":
		value, ok := l.immediateValue(ins, arguments, bits)
		if !ok {
			return 0, false
		}
		l.writeRegister(ins.address, destination, bits, value)
		return value, true
	case "larl":
		// LARL loads the address of one PC-relative location: the immediate is
		// a halfword count from the instruction address.
		target, ok := pcRelativeTarget(ins.inst, ins.address)
		if !ok {
			return 0, false
		}
		value := l.pointerConstant(ins.address, target)
		l.writeRegister(ins.address, destination, bits, value)
		return value, true
	}
	right, hasImmediate := l.immediateValue(ins, arguments, bits)
	if !hasImmediate {
		return 0, false
	}
	kind, ok := immediateOpcode(name)
	if !ok {
		// An mnemonic with no ALU mapping but a register and an immediate is
		// still a well-formed operation: keep it as an intrinsic.
		left, ok := l.readRegisterValue(ins, destination, bits)
		if !ok {
			return 0, false
		}
		result := l.operation(ins, core.OpIntrinsic, []core.Operand{
			symbolOperand("s390x." + name), l.value(left), l.value(right),
		}, bits, core.EffectNone)
		if destination != "" {
			l.writeRegister(ins.address, destination, bits, result)
		}
		return result, true
	}
	left, ok := l.readRegisterValue(ins, destination, bits)
	if !ok {
		return 0, false
	}
	result := l.operation(ins, kind, []core.Operand{l.value(left), l.value(right)}, bits, core.EffectNone)
	l.writeRegister(ins.address, destination, bits, result)
	return result, true
}

// immediateValue materializes the immediate operand of one instruction.
func (l *lifter) immediateValue(ins instruction, arguments []s390xasm.Arg, bits uint16) (core.ValueID, bool) {
	for _, arg := range arguments[1:] {
		switch value := arg.(type) {
		case s390xasm.Sign8:
			return l.constant(ins.address, uint64(int64(int8(value))), bits), true
		case s390xasm.Sign16:
			return l.constant(ins.address, uint64(int64(int16(value))), bits), true
		case s390xasm.Sign32:
			return l.constant(ins.address, uint64(int64(int32(value))), bits), true
		case s390xasm.Imm:
			return l.constant(ins.address, uint64(uint32(value)), bits), true
		case s390xasm.RegIm16:
			return l.constant(ins.address, 2*uint64(int16(value)), bits), true
		case s390xasm.RegIm32:
			return l.constant(ins.address, 2*uint64(int32(value)), bits), true
		case s390xasm.RegIm12:
			return l.constant(ins.address, 2*uint64(int16(value)), bits), true
		case s390xasm.RegIm24:
			return l.constant(ins.address, 2*uint64(int32(value)), bits), true
		}
	}
	return 0, false
}

// immediateOpcode maps one register-and-immediate mnemonic to a LowIR opcode.
// The table is explicit because the suffix rule cannot separate agrk from ag.
func immediateOpcode(name string) (core.Opcode, bool) {
	switch name {
	case "agrk", "ark", "ahik", "aghik", "alrk", "algrk", "alcr", "alcfr",
		"afi", "agfi", "aghi", "ahi", "aih", "aghl", "ay", "ag", "ah", "a":
		return core.OpAdd, true
	case "sgrk", "srk", "slrk", "slgrk", "sgr", "sg", "sh", "sr", "s",
		"sfi", "sgfi", "slfi", "slgfi", "sgrl":
		return core.OpSub, true
	case "ngrk", "nrk", "ngr", "ng", "nr", "n", "nilf", "nilh", "nill",
		"ni", "niy", "ngrl":
		return core.OpAnd, true
	case "ogrk", "ork", "ogr", "og", "o", "oill", "oilf", "oilh", "oi":
		return core.OpOr, true
	case "xgrk", "xrk", "xgr", "xg", "xr", "x", "xilf", "xilh", "xill", "xi":
		return core.OpXor, true
	case "lgfi", "llilf", "llilh", "llill", "llihf", "llihh", "llihl":
		return core.OpOr, true
	}
	return "", false
}

// isCompareBranch reports whether the mnemonic compares two operands and then
// branches: cgij, clgrj, cij and their relatives.
func isCompareBranch(name string) bool {
	base := strings.TrimSuffix(name, "j")
	switch base {
	case "ci", "cgi", "cli", "clfi", "clgi", "cgr", "clgr", "cr", "clr",
		"cg", "clg", "c", "cl", "ch", "clh", "chi", "clhi", "cy", "cly":
		return true
	}
	return false
}

// branchOnCount lifts the compare-and-branch forms: cgij, clgrj, cij and their
// relatives. They compare two operands and branch when the comparison matches.
func (l *lifter) branchOnCount(ins instruction) (core.ValueID, bool) {
	name := nameOf(ins.inst.Op)
	if !isCompareBranch(name) {
		return 0, false
	}
	arguments := args(ins.inst)
	if len(arguments) < 2 {
		return 0, false
	}
	target, ok := branchTarget(ins.inst, ins.address, ins.size)
	if !ok {
		return 0, false
	}
	left, ok := l.sourceValue(ins, arguments[0], addressBits)
	if !ok {
		return 0, false
	}
	right, ok := l.sourceValue(ins, arguments[1], addressBits)
	if !ok {
		return 0, false
	}
	kind := core.OpSub
	if strings.HasPrefix(name, "cl") {
		kind = core.OpSub
	}
	flags := l.operation(ins, kind, []core.Operand{l.value(left), l.value(right)}, addressBits, core.EffectNone)
	mask := l.conditionMask(ins)
	conditionValue := l.operation(ins, core.OpIntrinsic, []core.Operand{
		symbolOperand("cond." + conditionName(mask)), l.value(flags),
	}, 1, core.EffectReadRegister)
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
	return conditionValue, true
}

// opaqueRegisterForm lifts one instruction that carries only register operands
// and has no ALU mapping: the select and insert forms, the compare forms and the
// branch-and-save forms that did not resolve to a target. The destination is the
// first register.
func (l *lifter) opaqueRegisterForm(ins instruction) (core.ValueID, bool) {
	name := nameOf(ins.inst.Op)
	arguments := args(ins.inst)
	registers := make([]string, 0, len(arguments))
	for _, arg := range arguments {
		switch value := arg.(type) {
		case s390xasm.Reg:
			register, _, ok := registerName(value)
			if !ok {
				continue
			}
			registers = append(registers, register)
		}
	}
	if len(registers) == 0 {
		return 0, false
	}
	operands := []core.Operand{symbolOperand("s390x." + name)}
	for index, register := range registers {
		if index == 0 {
			value := l.newValue(core.TypeInteger, addressBits)
			operands = append(operands, core.Operand{Kind: core.OperandRegister, Type: value.Type, Name: register})
			l.emit(core.OpIntrinsic, operands, []core.Value{value}, nil,
				core.EffectReadRegister|core.EffectWriteRegister|core.EffectUnknown, ins.address)
			for _, source := range registers[1:] {
				_ = source
			}
			// The remaining sources are recorded on the same instruction.
			for _, source := range registers[1:] {
				l.instructions[len(l.instructions)-1].Operands = append(
					l.instructions[len(l.instructions)-1].Operands,
					core.Operand{Kind: core.OperandRegister, Type: core.ValueType{Kind: core.TypeInteger, Bits: addressBits}, Name: source})
			}
			return value.ID, true
		}
	}
	return 0, false
}

// conditionName maps one branch mask to the comparison it tests. A mask may name
// several conditions at once, and a few name none of the six comparisons, so a
// mask without a single equivalent keeps its numeric form and the consumer leaves
// the branch unresolved instead of inventing a comparison.
func conditionName(mask string) string {
	switch mask {
	case "8":
		return "e"
	case "7":
		return "ne"
	case "2":
		return "hi"
	case "4":
		return "lo"
	case "9":
		return "hs"
	case "12":
		return "ls"
	case "3":
		return "g"
	case "5":
		return "l"
	case "10":
		return "ge"
	case "13":
		return "le"
	}
	return mask
}
