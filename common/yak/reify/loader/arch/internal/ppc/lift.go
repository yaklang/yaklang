package ppc

import (
	"fmt"

	"golang.org/x/arch/ppc64/ppc64asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// lifter converts one function body into LowIR. Registers are explicit reads and
// writes; r1 is the stack pointer and the condition register CR0 carries the
// comparison flags.
type lifter struct {
	// pushedPC is the address a branch-to-next pushed into the link register, and
	// hasPushedPC says whether a read of that register may still return it.
	pushedPC    uint64
	hasPushedPC bool
	// next is the instruction that follows the one being lifted. It tells the
	// program-counter idiom from a plain branch.
	next *instruction
	// flagsValue is the comparison this block wrote last, and hasFlagsValue says a
	// branch may use it. A reader that looks the comparison up by register value
	// loses it across blocks; the branch can name it directly while the compare is
	// in the same block.
	flagsValue    core.ValueID
	hasFlagsValue bool
	profile       Profile
	blockIDs      map[uint64]core.BlockID
	diagnostics   *diagnostics
	nextValue     core.ValueID
	values        map[core.ValueID]core.ValueType
	instructions  []core.Instruction
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
		for index, ins := range current.instructions {
			lifter.next = nil
			if index+1 < len(current.instructions) {
				lifter.next = &current.instructions[index+1]
			}
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
	if _, start := l.blockIDs[ins.address]; start {
		// A comparison only describes a branch in its own block; after a block
		// boundary another path may have written the condition register.
		l.hasFlagsValue = false
	}
	if ins.undefined {
		l.emit(core.OpTrap, []core.Operand{symbolOperand("ppc.data")}, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
		return
	}
	// A pending pushed address belongs to the instruction right after the branch.
	if l.hasPushedPC && ins.address != l.pushedPC {
		l.hasPushedPC = false
	}
	name := normalize(opName(ins.inst.Op))
	// Position-independent code finds its own address with a branch to the next
	// instruction and a read of the link register. The branch pushes the address;
	// it is not a call.
	if (name == "bcl" || name == "bcl+" || name == "bcla") && l.isPCIdiom(ins) {
		l.pushedPC = ins.address + 4
		l.hasPushedPC = true
		// The instruction is a block terminator even though it is not a branch to
		// another function, so the block still needs its fallthrough edge.
		if next, ok := l.blockIDs[ins.address+4]; ok {
			l.emit(core.OpJump, nil, nil, []core.BlockID{next}, core.EffectNone, ins.address)
		}
		return
	}
	if isUnknownOpcode(name) || name == "" {
		// Zero words are alignment padding or data inside .text; the decoder
		// has no mnemonic for them.
		l.emit(core.OpTrap, []core.Operand{symbolOperand("ppc.data")}, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
		return
	}
	switch {
	case isReturn(name):
		l.emit(core.OpReturn, nil, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
		return
	case isIndirectBranch(name):
		// The target comes from a register; keep it as an indirect transfer.
		l.emit(core.OpJump, []core.Operand{symbolOperand("ppc." + name)}, nil, nil, core.EffectUnknown, ins.address)
		return
	case isCall(ins.inst.Op):
		l.call(ins)
		return
	case isBranch(name):
		l.branch(ins)
		return
	case isJump(name):
		l.jump(ins)
		return
	case isVectorIndexed(name):
		l.vector(ins)
		return
	case isLoad(name):
		l.load(ins)
		return
	case isStore(name):
		l.store(ins)
		return
	case isSystemLike(name):
		l.system(ins)
		return
	}
	if value, ok := l.arithmetic(ins); ok {
		_ = value
		return
	}
	if isVector(name) {
		l.vector(ins)
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
	operands := []core.Operand{symbolOperand("ppc." + opName(ins.inst.Op))}
	for index, arg := range ins.inst.Args {
		if arg == nil {
			break
		}
		if operand, ok := l.rawOperand(ins, index, arg); ok {
			operands = append(operands, operand)
		}
	}
	l.emit(core.OpIntrinsic, operands, nil, nil, core.EffectUnknown, ins.address)
	l.diagnostics.add("unsupported-instruction",
		fmt.Sprintf("unsupported ppc instruction %s at 0x%x", ins.inst.Op, ins.address))
}

func (l *lifter) rawOperand(ins instruction, index int, arg ppc64asm.Arg) (core.Operand, bool) {
	if reg, ok := arg.(ppc64asm.Reg); ok {
		if name, _, ok := registerName(reg); ok {
			if isFloatRegister(name) || isVectorRegister(name) {
				return core.Operand{
					Kind: core.OperandRegister,
					Type: core.ValueType{Kind: core.TypeFloat, Bits: l.profile.Bits},
					Name: name,
				}, true
			}
			return l.value(l.readRegister(ins.address, name, l.profile.Bits)), true
		}
	}
	if value, ok := signedImmediate(arg); ok {
		return l.value(l.constant(ins.address, uint64(value), l.profile.Bits)), true
	}
	if offset, ok := pcRelative(arg); ok {
		return l.value(l.pointerConstant(ins.address, ins.address+uint64(offset))), true
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

// isPCIdiom reports whether the branch is the way position-independent code reads
// its own address. The branch targets the instruction that follows it, so taking
// it and falling through are the same continuation.
func (l *lifter) isPCIdiom(ins instruction) bool {
	target, ok := staticTarget(ins.inst, ins.address)
	return ok && target == ins.address+4
}
