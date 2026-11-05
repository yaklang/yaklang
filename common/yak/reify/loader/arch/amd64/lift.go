package amd64

import (
	"fmt"
	"strings"

	"golang.org/x/arch/x86/x86asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

type amd64Lifter struct {
	blockIDs     map[uint64]core.BlockID
	diagnostics  *amd64Diagnostics
	nextValue    core.ValueID
	values       map[core.ValueID]core.ValueType
	instructions []core.Instruction
}

type amd64Place struct {
	reg     string
	bits    uint16
	memory  bool
	address core.ValueID
}

func liftAMD64(target core.Target, blocks []amd64Block, blockIDs map[uint64]core.BlockID, diagnostics *amd64Diagnostics) *core.LowIR {
	ir := &core.LowIR{Target: target}
	if len(blocks) == 0 {
		return ir
	}
	function := core.Function{
		ID:      1,
		Address: blocks[0].start,
		Blocks:  make([]core.Block, 0, len(blocks)),
	}
	lifter := &amd64Lifter{blockIDs: blockIDs, diagnostics: diagnostics, values: make(map[core.ValueID]core.ValueType)}
	for index, block := range blocks {
		lifter.instructions = make([]core.Instruction, 0, len(block.instructions)*2)
		for _, instruction := range block.instructions {
			lifter.lift(instruction)
		}
		if len(lifter.instructions) == 0 || !amd64ControlTransfer(lifter.instructions[len(lifter.instructions)-1].Kind) {
			end := block.start
			if len(block.instructions) > 0 {
				last := block.instructions[len(block.instructions)-1]
				end = last.address + uint64(last.size)
			}
			if index+1 < len(blocks) {
				next := blockIDs[blocks[index+1].start]
				lifter.emit(core.OpJump, nil, nil, []core.BlockID{next}, core.EffectNone, end)
			} else {
				lifter.emit(core.OpTrap, []core.Operand{amd64Symbol("region-end")}, nil, nil, core.EffectMayTrap|core.EffectUnknown, end)
				diagnostics.add("block-falls-off-region", fmt.Sprintf("block at 0x%x reaches the end of the decoded region", block.start))
			}
		}
		function.Blocks = append(function.Blocks, core.Block{ID: core.BlockID(index + 1), Address: block.start, Instructions: lifter.instructions})
	}
	ir.Functions = append(ir.Functions, function)
	return ir
}

func (l *amd64Lifter) lift(ins amd64Instruction) {
	if ins.endbr != "" {
		l.emit(core.OpIntrinsic, []core.Operand{amd64Symbol("x86." + ins.endbr)}, nil, nil, core.EffectNone, ins.address)
		return
	}
	if amd64HasPrefix(ins, x86asm.PrefixLOCK) {
		l.diagnostics.add("atomic-prefix-unmodeled", fmt.Sprintf("LOCK prefix at 0x%x is not modeled as an atomic effect", ins.address))
	}
	switch ins.inst.Op {
	case x86asm.NOP:
		l.emit(core.OpNop, nil, nil, nil, core.EffectNone, ins.address)
	case x86asm.MOV, x86asm.MOVAPS, x86asm.MOVUPS, x86asm.MOVDQA, x86asm.MOVDQU, x86asm.MOVAPD, x86asm.MOVUPD, x86asm.MOVD, x86asm.MOVQ:
		l.move(ins)
	case x86asm.MOVZX:
		l.extend(ins, core.OpZeroExtend)
	case x86asm.MOVSX, x86asm.MOVSXD:
		l.extend(ins, core.OpSignExtend)
	case x86asm.LEA:
		l.lea(ins)
	case x86asm.ADD:
		l.binary(ins, core.OpAdd, "add")
	case x86asm.SUB:
		l.binary(ins, core.OpSub, "sub")
	case x86asm.AND:
		l.binary(ins, core.OpAnd, "and")
	case x86asm.OR:
		l.binary(ins, core.OpOr, "or")
	case x86asm.XOR:
		l.binary(ins, core.OpXor, "xor")
	case x86asm.PXOR, x86asm.XORPS, x86asm.XORPD, x86asm.ANDPS, x86asm.ANDPD, x86asm.ORPS, x86asm.ORPD:
		l.vectorBinary(ins)
	case x86asm.CMP:
		l.compare(ins, "cmp")
	case x86asm.TEST:
		l.compare(ins, "test")
	case x86asm.BT:
		l.bitTest(ins)
	case x86asm.MUL:
		l.multiply(ins)
	case x86asm.DIV:
		l.divide(ins, "div")
	case x86asm.IDIV:
		l.divide(ins, "idiv")
	case x86asm.PUNPCKLQDQ:
		l.vectorIntrinsic(ins, "x86.punpcklqdq")
	case x86asm.INC:
		l.increment(ins, core.OpAdd, "inc")
	case x86asm.DEC:
		l.increment(ins, core.OpSub, "dec")
	case x86asm.NEG:
		l.unary(ins, core.OpNeg, "neg")
	case x86asm.NOT:
		l.unary(ins, core.OpNot, "")
	case x86asm.IMUL:
		l.imul(ins)
	case x86asm.SHL:
		l.shift(ins, core.OpShl, "shl")
	case x86asm.SHR:
		l.shift(ins, core.OpLShr, "shr")
	case x86asm.SAR:
		l.shift(ins, core.OpAShr, "sar")
	case x86asm.PUSH:
		l.push(ins)
	case x86asm.POP:
		l.pop(ins)
	case x86asm.PUSHFQ:
		l.pushValue(ins, l.readRegister(ins.address, "rflags", 64))
	case x86asm.POPFQ:
		l.popIntoRegister(ins, "rflags", 64)
	case x86asm.LEAVE:
		l.leave(ins)
	case x86asm.XCHG:
		l.xchg(ins)
	case x86asm.CDQ:
		l.signExtendAccumulator(ins, "eax", "edx", 32)
	case x86asm.CQO:
		l.signExtendAccumulator(ins, "rax", "rdx", 64)
	case x86asm.CWD:
		l.signExtendAccumulator(ins, "ax", "dx", 16)
	case x86asm.CDQE:
		l.extendFixed(ins, "eax", 32, "rax", 64, core.OpSignExtend)
	case x86asm.CWDE:
		l.extendFixed(ins, "ax", 16, "eax", 32, core.OpSignExtend)
	case x86asm.JMP:
		l.jump(ins)
	case x86asm.CALL:
		l.call(ins)
	case x86asm.RET:
		l.emit(core.OpReturn, nil, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
	case x86asm.SYSCALL, x86asm.SYSENTER:
		l.emit(core.OpIntrinsic, []core.Operand{amd64Symbol("x86." + strings.ToLower(ins.inst.Op.String()))}, nil, nil, core.EffectCall|core.EffectMayTrap|core.EffectUnknown, ins.address)
	case x86asm.INT:
		if immediate, ok := ins.inst.Args[0].(x86asm.Imm); ok && immediate == 3 {
			l.trap(ins, "int3")
		} else {
			l.unsupported(ins)
		}
	case x86asm.UD2, x86asm.HLT:
		l.trap(ins, strings.ToLower(ins.inst.Op.String()))
	case x86asm.IRET, x86asm.IRETD, x86asm.IRETQ:
		l.emit(core.OpReturn, nil, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
	default:
		name := ins.inst.Op.String()
		switch {
		case strings.HasPrefix(name, "J") && name != "JMP":
			l.branch(ins)
		case strings.HasPrefix(name, "SET"):
			l.setCondition(ins)
		case strings.HasPrefix(name, "CMOV"):
			l.conditionalMove(ins)
		default:
			l.unsupported(ins)
		}
	}
}

func (l *amd64Lifter) move(ins amd64Instruction) {
	args := amd64Args(ins)
	if len(args) != 2 || !amd64Movable(args[0]) || !amd64Source(args[1]) {
		l.unsupported(ins)
		return
	}
	destinationBits := l.argBits(ins, args[0], 64)
	sourceBits := l.argBits(ins, args[1], destinationBits)
	destination, ok := l.place(ins, args[0], destinationBits)
	if !ok {
		l.unsupported(ins)
		return
	}
	value, ok := l.readSource(ins, args[1], sourceBits)
	if !ok {
		l.unsupported(ins)
		return
	}
	if sourceBits != destinationBits {
		_, immediate := args[1].(x86asm.Imm)
		value = l.coerce(ins, value, sourceBits, destinationBits, immediate)
	}
	l.writePlace(ins, destination, value)
}

func (l *amd64Lifter) extend(ins amd64Instruction, kind core.Opcode) {
	args := amd64Args(ins)
	if len(args) != 2 || !amd64GPRDestination(args[0]) || !amd64Source(args[1]) {
		l.unsupported(ins)
		return
	}
	destinationBits := l.argBits(ins, args[0], 64)
	sourceBits := l.argBits(ins, args[1], destinationBits)
	destination, ok := l.place(ins, args[0], destinationBits)
	if !ok {
		l.unsupported(ins)
		return
	}
	value, ok := l.readSource(ins, args[1], sourceBits)
	if !ok {
		l.unsupported(ins)
		return
	}
	extended := l.operation(ins, kind, []core.Operand{l.value(value)}, destinationBits, core.EffectNone)
	l.writePlace(ins, destination, extended)
}

func (l *amd64Lifter) lea(ins amd64Instruction) {
	args := amd64Args(ins)
	if len(args) != 2 || !amd64GPRDestination(args[0]) {
		l.unsupported(ins)
		return
	}
	memory, ok := args[1].(x86asm.Mem)
	if !ok {
		l.unsupported(ins)
		return
	}
	destinationBits := l.argBits(ins, args[0], 64)
	destination, ok := l.place(ins, args[0], destinationBits)
	if !ok {
		l.unsupported(ins)
		return
	}
	address := l.address(ins, memory)
	if destinationBits != 64 {
		address = l.operation(ins, core.OpTruncate, []core.Operand{l.value(address)}, destinationBits, core.EffectNone)
	}
	l.writePlace(ins, destination, address)
}

func (l *amd64Lifter) binary(ins amd64Instruction, kind core.Opcode, flag string) {
	args := amd64Args(ins)
	if len(args) != 2 || !amd64GPRDestination(args[0]) || !amd64Source(args[1]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], 64)
	destination, ok := l.place(ins, args[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	left, ok := l.readPlace(ins, destination)
	if !ok {
		l.unsupported(ins)
		return
	}
	right, ok := l.readSource(ins, args[1], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	result := l.operation(ins, kind, []core.Operand{l.value(left), l.value(right)}, bits, core.EffectNone)
	l.writePlace(ins, destination, result)
	if flag != "" {
		l.setFlags(ins, flag, left, right, result)
	}
}

func (l *amd64Lifter) vectorBinary(ins amd64Instruction) {
	args := amd64Args(ins)
	if len(args) != 2 || !amd64Movable(args[0]) || !amd64Source(args[1]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], 128)
	destination, ok := l.place(ins, args[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	left, ok := l.readPlace(ins, destination)
	if !ok {
		l.unsupported(ins)
		return
	}
	right, ok := l.readSource(ins, args[1], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	kind := core.OpXor
	switch ins.inst.Op {
	case x86asm.ANDPS, x86asm.ANDPD:
		kind = core.OpAnd
	case x86asm.ORPS, x86asm.ORPD:
		kind = core.OpOr
	}
	result := l.operation(ins, kind, []core.Operand{l.value(left), l.value(right)}, bits, core.EffectNone)
	l.writePlace(ins, destination, result)
}

func (l *amd64Lifter) compare(ins amd64Instruction, flag string) {
	args := amd64Args(ins)
	if len(args) != 2 || !amd64GPRDestination(args[0]) || !amd64Source(args[1]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], 64)
	left, ok := l.readSource(ins, args[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	right, ok := l.readSource(ins, args[1], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	l.setFlags(ins, flag, left, right)
}

func (l *amd64Lifter) bitTest(ins amd64Instruction) {
	args := amd64Args(ins)
	if len(args) != 2 || !amd64Source(args[0]) || !amd64Source(args[1]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], 64)
	value, ok := l.readSource(ins, args[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	index, ok := l.readSource(ins, args[1], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	l.setFlags(ins, "bt", value, index)
}

func (l *amd64Lifter) multiply(ins amd64Instruction) {
	args := amd64Args(ins)
	if len(args) != 1 || !amd64Source(args[0]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], 64)
	accumulator, high := amd64AccumulatorNames(bits)
	left := l.readRegister(ins.address, accumulator, bits)
	right, ok := l.readSource(ins, args[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	low := l.operation(ins, core.OpIntrinsic, []core.Operand{amd64Symbol("x86.mul.lo"), l.value(left), l.value(right)}, bits, core.EffectNone)
	upper := l.operation(ins, core.OpIntrinsic, []core.Operand{amd64Symbol("x86.mul.hi"), l.value(left), l.value(right)}, bits, core.EffectNone)
	l.writeRegister(ins.address, accumulator, bits, low)
	l.writeRegister(ins.address, high, bits, upper)
	l.setFlags(ins, "mul", low, upper)
}

func (l *amd64Lifter) divide(ins amd64Instruction, name string) {
	args := amd64Args(ins)
	if len(args) != 1 || !amd64Source(args[0]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], 64)
	quotient, remainder := amd64AccumulatorNames(bits)
	low := l.readRegister(ins.address, quotient, bits)
	high := l.readRegister(ins.address, remainder, bits)
	divisor, ok := l.readSource(ins, args[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	operands := []core.Operand{l.value(high), l.value(low), l.value(divisor)}
	quot := l.operation(ins, core.OpIntrinsic, append([]core.Operand{amd64Symbol("x86." + name + ".quot")}, operands...), bits, core.EffectMayTrap)
	rem := l.operation(ins, core.OpIntrinsic, append([]core.Operand{amd64Symbol("x86." + name + ".rem")}, operands...), bits, core.EffectMayTrap)
	l.writeRegister(ins.address, quotient, bits, quot)
	l.writeRegister(ins.address, remainder, bits, rem)
}

func (l *amd64Lifter) vectorIntrinsic(ins amd64Instruction, symbol string) {
	args := amd64Args(ins)
	if len(args) != 2 || !amd64Movable(args[0]) || !amd64Source(args[1]) {
		l.unsupported(ins)
		return
	}
	destination, ok := l.place(ins, args[0], 128)
	if !ok {
		l.unsupported(ins)
		return
	}
	current, ok := l.readPlace(ins, destination)
	if !ok {
		l.unsupported(ins)
		return
	}
	source, ok := l.readSource(ins, args[1], 128)
	if !ok {
		l.unsupported(ins)
		return
	}
	result := l.operation(ins, core.OpIntrinsic, []core.Operand{amd64Symbol(symbol), l.value(current), l.value(source)}, 128, core.EffectNone)
	l.writePlace(ins, destination, result)
}

func (l *amd64Lifter) increment(ins amd64Instruction, kind core.Opcode, flag string) {
	args := amd64Args(ins)
	if len(args) != 1 || !amd64GPRDestination(args[0]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], 64)
	destination, ok := l.place(ins, args[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	left, ok := l.readPlace(ins, destination)
	if !ok {
		l.unsupported(ins)
		return
	}
	one := l.constant(ins.address, 1, bits)
	result := l.operation(ins, kind, []core.Operand{l.value(left), l.value(one)}, bits, core.EffectNone)
	l.writePlace(ins, destination, result)
	l.setFlags(ins, flag, left, one, result)
}

func (l *amd64Lifter) unary(ins amd64Instruction, kind core.Opcode, flag string) {
	args := amd64Args(ins)
	if len(args) != 1 || !amd64GPRDestination(args[0]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], 64)
	destination, ok := l.place(ins, args[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	value, ok := l.readPlace(ins, destination)
	if !ok {
		l.unsupported(ins)
		return
	}
	result := l.operation(ins, kind, []core.Operand{l.value(value)}, bits, core.EffectNone)
	l.writePlace(ins, destination, result)
	if flag != "" {
		l.setFlags(ins, flag, value)
	}
}

func (l *amd64Lifter) imul(ins amd64Instruction) {
	args := amd64Args(ins)
	if len(args) != 2 && len(args) != 3 {
		l.unsupported(ins)
		return
	}
	if !amd64GPRDestination(args[0]) || !amd64Source(args[1]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], 64)
	destination, ok := l.place(ins, args[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	left, ok := l.readSource(ins, args[1], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	right := left
	if len(args) == 3 {
		multiplier, ok := l.readSource(ins, args[2], bits)
		if !ok {
			l.unsupported(ins)
			return
		}
		right = multiplier
	} else {
		current, ok := l.readPlace(ins, destination)
		if !ok {
			l.unsupported(ins)
			return
		}
		left = current
	}
	result := l.operation(ins, core.OpMul, []core.Operand{l.value(left), l.value(right)}, bits, core.EffectNone)
	l.writePlace(ins, destination, result)
	l.setFlags(ins, "imul", left, right, result)
}

func (l *amd64Lifter) shift(ins amd64Instruction, kind core.Opcode, flag string) {
	args := amd64Args(ins)
	if len(args) != 2 || !amd64GPRDestination(args[0]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], 64)
	destination, ok := l.place(ins, args[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	value, ok := l.readPlace(ins, destination)
	if !ok {
		l.unsupported(ins)
		return
	}
	count, ok := l.readSource(ins, args[1], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	result := l.operation(ins, kind, []core.Operand{l.value(value), l.value(count)}, bits, core.EffectNone)
	l.writePlace(ins, destination, result)
	l.setFlags(ins, flag, value, count, result)
}

func (l *amd64Lifter) push(ins amd64Instruction) {
	args := amd64Args(ins)
	if len(args) != 1 || !amd64Source(args[0]) {
		l.unsupported(ins)
		return
	}
	value, ok := l.readSource(ins, args[0], 64)
	if !ok {
		l.unsupported(ins)
		return
	}
	l.pushValue(ins, value)
}

func (l *amd64Lifter) pushValue(ins amd64Instruction, value core.ValueID) {
	rsp := l.readRegister(ins.address, "rsp", 64)
	eight := l.constant(ins.address, 8, 64)
	next := l.operation(ins, core.OpSub, []core.Operand{l.value(rsp), l.value(eight)}, 64, core.EffectNone)
	l.writeRegister(ins.address, "rsp", 64, next)
	l.store(ins.address, l.pointerFrom(ins.address, next), value, 64)
}

func (l *amd64Lifter) pop(ins amd64Instruction) {
	args := amd64Args(ins)
	if len(args) != 1 || (!amd64GPRDestination(args[0]) && !amd64Memory(args[0])) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], 64)
	destination, ok := l.place(ins, args[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	rsp := l.readRegister(ins.address, "rsp", 64)
	value := l.load(ins.address, l.pointerFrom(ins.address, rsp), bits)
	l.writePlace(ins, destination, value)
	if destination.memory || destination.reg != "rsp" {
		eight := l.constant(ins.address, 8, 64)
		next := l.operation(ins, core.OpAdd, []core.Operand{l.value(rsp), l.value(eight)}, 64, core.EffectNone)
		l.writeRegister(ins.address, "rsp", 64, next)
	}
}

func (l *amd64Lifter) popIntoRegister(ins amd64Instruction, name string, bits uint16) {
	rsp := l.readRegister(ins.address, "rsp", 64)
	value := l.load(ins.address, l.pointerFrom(ins.address, rsp), bits)
	l.writeRegister(ins.address, name, bits, value)
	eight := l.constant(ins.address, 8, 64)
	next := l.operation(ins, core.OpAdd, []core.Operand{l.value(rsp), l.value(eight)}, 64, core.EffectNone)
	l.writeRegister(ins.address, "rsp", 64, next)
}

func (l *amd64Lifter) leave(ins amd64Instruction) {
	frame := l.readRegister(ins.address, "rbp", 64)
	l.writeRegister(ins.address, "rsp", 64, frame)
	l.popIntoRegister(ins, "rbp", 64)
}

func (l *amd64Lifter) xchg(ins amd64Instruction) {
	args := amd64Args(ins)
	if len(args) != 2 || (!amd64GPRDestination(args[0]) && !amd64Memory(args[0])) || (!amd64GPRDestination(args[1]) && !amd64Memory(args[1])) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], 64)
	leftPlace, ok := l.place(ins, args[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	rightPlace, ok := l.place(ins, args[1], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	left, ok := l.readPlace(ins, leftPlace)
	if !ok {
		l.unsupported(ins)
		return
	}
	right, ok := l.readPlace(ins, rightPlace)
	if !ok {
		l.unsupported(ins)
		return
	}
	l.writePlace(ins, leftPlace, right)
	l.writePlace(ins, rightPlace, left)
}

func (l *amd64Lifter) signExtendAccumulator(ins amd64Instruction, source, destination string, bits uint16) {
	value := l.readRegister(ins.address, source, bits)
	shift := l.constant(ins.address, uint64(bits-1), bits)
	sign := l.operation(ins, core.OpAShr, []core.Operand{l.value(value), l.value(shift)}, bits, core.EffectNone)
	l.writeRegister(ins.address, destination, bits, sign)
}

func (l *amd64Lifter) extendFixed(ins amd64Instruction, source string, sourceBits uint16, destination string, destinationBits uint16, kind core.Opcode) {
	value := l.readRegister(ins.address, source, sourceBits)
	extended := l.operation(ins, kind, []core.Operand{l.value(value)}, destinationBits, core.EffectNone)
	l.writeRegister(ins.address, destination, destinationBits, extended)
}

func (l *amd64Lifter) jump(ins amd64Instruction) {
	if target, ok := amd64DirectTarget(ins); ok {
		if block, ok := l.blockIDs[target]; ok {
			l.emit(core.OpJump, nil, nil, []core.BlockID{block}, core.EffectNone, ins.address)
			return
		}
		l.emit(core.OpJump, []core.Operand{amd64Pointer(target)}, nil, nil, core.EffectUnknown, ins.address)
		return
	}
	args := amd64Args(ins)
	if len(args) != 1 {
		l.unsupported(ins)
		return
	}
	value, ok := l.readSource(ins, args[0], 64)
	if !ok {
		l.unsupported(ins)
		return
	}
	value = l.pointerFrom(ins.address, value)
	l.emit(core.OpJump, []core.Operand{l.value(value)}, nil, nil, core.EffectUnknown, ins.address)
}

func (l *amd64Lifter) branch(ins amd64Instruction) {
	condition := l.condition(ins)
	if condition == 0 {
		return
	}
	operands := []core.Operand{l.value(condition)}
	var targets []core.BlockID
	if target, ok := amd64DirectTarget(ins); ok {
		if block, ok := l.blockIDs[target]; ok {
			targets = append(targets, block)
		} else {
			operands = append(operands, amd64Pointer(target))
		}
	}
	next := ins.address + uint64(ins.size)
	if block, ok := l.blockIDs[next]; ok && !containsBlock(targets, block) {
		targets = append(targets, block)
	}
	effects := core.EffectNone
	if len(targets) == 0 {
		effects = core.EffectUnknown
	}
	l.emit(core.OpBranch, operands, nil, targets, effects, ins.address)
}

func (l *amd64Lifter) call(ins amd64Instruction) {
	if target, ok := amd64DirectTarget(ins); ok {
		l.emit(core.OpCall, []core.Operand{amd64Pointer(target)}, nil, nil, core.EffectCall|core.EffectMayTrap|core.EffectUnknown, ins.address)
		return
	}
	args := amd64Args(ins)
	if len(args) != 1 {
		l.unsupported(ins)
		return
	}
	value, ok := l.readSource(ins, args[0], 64)
	if !ok {
		l.unsupported(ins)
		return
	}
	value = l.pointerFrom(ins.address, value)
	l.emit(core.OpCall, []core.Operand{l.value(value)}, nil, nil, core.EffectCall|core.EffectMayTrap|core.EffectUnknown, ins.address)
}

func (l *amd64Lifter) setCondition(ins amd64Instruction) {
	condition := l.condition(ins)
	if condition == 0 {
		return
	}
	args := amd64Args(ins)
	if len(args) != 1 || (!amd64GPRDestination(args[0]) && !amd64Memory(args[0])) {
		l.unsupported(ins)
		return
	}
	destination, ok := l.place(ins, args[0], 8)
	if !ok {
		l.unsupported(ins)
		return
	}
	value := l.operation(ins, core.OpZeroExtend, []core.Operand{l.value(condition)}, 8, core.EffectNone)
	l.writePlace(ins, destination, value)
}

func (l *amd64Lifter) conditionalMove(ins amd64Instruction) {
	condition := l.condition(ins)
	if condition == 0 {
		return
	}
	args := amd64Args(ins)
	if len(args) != 2 || !amd64GPRDestination(args[0]) || !amd64Source(args[1]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], 64)
	destination, ok := l.place(ins, args[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	current, ok := l.readPlace(ins, destination)
	if !ok {
		l.unsupported(ins)
		return
	}
	candidate, ok := l.readSource(ins, args[1], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	result := l.operation(ins, core.OpSelect, []core.Operand{l.value(condition), l.value(candidate), l.value(current)}, bits, core.EffectNone)
	l.writePlace(ins, destination, result)
}

func (l *amd64Lifter) condition(ins amd64Instruction) core.ValueID {
	code := amd64Condition(ins.inst.Op)
	if code == "" {
		return 0
	}
	if code == "rcxz" || code == "ecxz" || code == "cxz" {
		name := "rcx"
		bits := uint16(64)
		switch code {
		case "ecxz":
			name, bits = "ecx", 32
		case "cxz":
			name, bits = "cx", 16
		}
		value := l.readRegister(ins.address, name, bits)
		zero := l.constant(ins.address, 0, bits)
		return l.operation(ins, core.OpEqual, []core.Operand{l.value(value), l.value(zero)}, 1, core.EffectNone)
	}
	flags := l.readRegister(ins.address, "rflags", 64)
	return l.operation(ins, core.OpIntrinsic, []core.Operand{amd64Symbol("x86.cond." + code), l.value(flags)}, 1, core.EffectReadRegister)
}

func (l *amd64Lifter) setFlags(ins amd64Instruction, name string, values ...core.ValueID) {
	operands := make([]core.Operand, 0, len(values)+1)
	operands = append(operands, amd64Symbol("x86.flags."+name))
	for _, value := range values {
		operands = append(operands, l.value(value))
	}
	flags := l.operation(ins, core.OpIntrinsic, operands, 64, core.EffectWriteRegister)
	l.writeRegister(ins.address, "rflags", 64, flags)
}

func (l *amd64Lifter) trap(ins amd64Instruction, name string) {
	l.emit(core.OpTrap, []core.Operand{amd64Symbol("x86." + name)}, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
}

func (l *amd64Lifter) unsupported(ins amd64Instruction) {
	operands := make([]core.Operand, 0, 4)
	for _, arg := range amd64Args(ins) {
		if arg == nil {
			continue
		}
		operand, ok := l.rawOperand(ins, arg)
		if ok {
			operands = append(operands, operand)
		}
	}
	l.emit(core.OpIntrinsic, append([]core.Operand{amd64Symbol("x86." + strings.ToLower(ins.inst.Op.String()))}, operands...), nil, nil, core.EffectUnknown, ins.address)
	l.diagnostics.add("unsupported-instruction", fmt.Sprintf("unsupported amd64 instruction %s at 0x%x", ins.inst.Op, ins.address))
}

func (l *amd64Lifter) rawOperand(ins amd64Instruction, arg x86asm.Arg) (core.Operand, bool) {
	switch value := arg.(type) {
	case x86asm.Reg:
		valueID, ok := l.readRegisterOperand(ins.address, value)
		if !ok {
			return core.Operand{}, false
		}
		return l.value(valueID), true
	case x86asm.Imm:
		valueID := l.constant(ins.address, uint64(int64(value)), 64)
		return l.value(valueID), true
	case x86asm.Mem:
		return l.value(l.address(ins, value)), true
	case x86asm.Rel:
		return amd64Pointer(ins.address + uint64(ins.size) + uint64(int64(value))), true
	default:
		return core.Operand{}, false
	}
}

func (l *amd64Lifter) place(ins amd64Instruction, arg x86asm.Arg, bits uint16) (amd64Place, bool) {
	switch value := arg.(type) {
	case x86asm.Reg:
		name, registerBits, ok := amd64Register(value)
		if !ok {
			return amd64Place{}, false
		}
		if bits == 0 {
			bits = registerBits
		}
		return amd64Place{reg: name, bits: bits}, true
	case x86asm.Mem:
		if bits == 0 {
			bits = l.argBits(ins, value, 64)
		}
		return amd64Place{memory: true, bits: bits, address: l.address(ins, value)}, true
	default:
		return amd64Place{}, false
	}
}

func (l *amd64Lifter) readPlace(ins amd64Instruction, place amd64Place) (core.ValueID, bool) {
	if place.memory {
		return l.load(ins.address, place.address, place.bits), true
	}
	return l.readRegister(ins.address, place.reg, place.bits), true
}

func (l *amd64Lifter) writePlace(ins amd64Instruction, place amd64Place, value core.ValueID) {
	if place.memory {
		l.store(ins.address, place.address, value, place.bits)
		return
	}
	l.writeRegister(ins.address, place.reg, place.bits, value)
}

func (l *amd64Lifter) readSource(ins amd64Instruction, arg x86asm.Arg, bits uint16) (core.ValueID, bool) {
	switch value := arg.(type) {
	case x86asm.Imm:
		return l.constant(ins.address, uint64(int64(value)), bits), true
	case x86asm.Reg:
		return l.readRegisterOperand(ins.address, value)
	case x86asm.Mem:
		if bits == 0 {
			bits = l.argBits(ins, value, 64)
		}
		return l.load(ins.address, l.address(ins, value), bits), true
	case x86asm.Rel:
		return l.constant(ins.address, ins.address+uint64(ins.size)+uint64(int64(value)), 64), true
	default:
		return 0, false
	}
}

// RIP-relative displacements are relative to the end of the instruction.
func (l *amd64Lifter) address(ins amd64Instruction, memory x86asm.Mem) core.ValueID {
	var result core.ValueID
	if memory.Base == x86asm.RIP {
		result = l.pointerConstant(ins.address, ins.address+uint64(ins.size)+uint64(memory.Disp))
	} else if memory.Base != 0 {
		base, ok := l.readAddressRegister(ins, memory.Base)
		if ok {
			result = l.pointerFrom(ins.address, base)
		}
	}
	if memory.Index != 0 && memory.Scale != 0 {
		index, ok := l.readAddressRegister(ins, memory.Index)
		if ok {
			if memory.Scale > 1 {
				scale := l.constant(ins.address, uint64(memory.Scale), 64)
				index = l.operation(ins, core.OpMul, []core.Operand{l.value(index), l.value(scale)}, 64, core.EffectNone)
			}
			result = l.addAddress(ins, result, index)
		}
	}
	if memory.Base != x86asm.RIP && memory.Disp != 0 {
		displacement := l.constant(ins.address, uint64(memory.Disp), 64)
		result = l.addAddress(ins, result, displacement)
	}
	if result == 0 {
		result = l.pointerConstant(ins.address, 0)
	}
	if memory.Segment != 0 {
		segment := strings.ToLower(memory.Segment.String())
		result = l.operation(ins, core.OpIntrinsic, []core.Operand{amd64Symbol("x86.segment." + segment), l.value(result)}, 64, core.EffectReadRegister|core.EffectUnknown)
	}
	return result
}

func (l *amd64Lifter) addAddress(ins amd64Instruction, left, right core.ValueID) core.ValueID {
	if left == 0 {
		return l.pointerFrom(ins.address, right)
	}
	result := l.newValue(core.TypePointer, 64)
	l.emit(core.OpAdd, []core.Operand{l.value(left), l.value(right)}, []core.Value{result}, nil, core.EffectNone, ins.address)
	return result.ID
}

func (l *amd64Lifter) readAddressRegister(ins amd64Instruction, reg x86asm.Reg) (core.ValueID, bool) {
	name, bits, ok := amd64Register(reg)
	if !ok {
		return 0, false
	}
	value := l.readRegister(ins.address, name, bits)
	if bits < 64 {
		value = l.coerce(ins, value, bits, 64, false)
	}
	return value, true
}

func (l *amd64Lifter) pointerFrom(address uint64, value core.ValueID) core.ValueID {
	if l.values[value].Kind == core.TypePointer {
		return value
	}
	result := l.newValue(core.TypePointer, 64)
	l.emit(core.OpCopy, []core.Operand{l.value(value)}, []core.Value{result}, nil, core.EffectNone, address)
	return result.ID
}

func (l *amd64Lifter) readRegisterOperand(address uint64, reg x86asm.Reg) (core.ValueID, bool) {
	name, bits, ok := amd64Register(reg)
	if !ok {
		return 0, false
	}
	return l.readRegister(address, name, bits), true
}

func (l *amd64Lifter) coerce(ins amd64Instruction, value core.ValueID, from, to uint16, signed bool) core.ValueID {
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

func (l *amd64Lifter) argBits(ins amd64Instruction, arg x86asm.Arg, fallback uint16) uint16 {
	switch value := arg.(type) {
	case x86asm.Reg:
		if _, bits, ok := amd64Register(value); ok {
			return bits
		}
	case x86asm.Mem:
		if ins.inst.MemBytes > 0 {
			return uint16(ins.inst.MemBytes * 8)
		}
	case x86asm.Imm:
		if ins.inst.DataSize > 0 {
			return uint16(ins.inst.DataSize)
		}
	}
	return fallback
}

func (l *amd64Lifter) constant(address uint64, value uint64, bits uint16) core.ValueID {
	if bits == 0 {
		bits = 64
	}
	result := l.newValue(core.TypeInteger, bits)
	l.emit(core.OpConstant, []core.Operand{{
		Kind:    core.OperandConstant,
		Type:    result.Type,
		Literal: amd64Literal(value, bits),
	}}, []core.Value{result}, nil, core.EffectNone, address)
	return result.ID
}

func (l *amd64Lifter) pointerConstant(address, value uint64) core.ValueID {
	result := l.newValue(core.TypePointer, 64)
	l.emit(core.OpConstant, []core.Operand{{
		Kind:    core.OperandConstant,
		Type:    result.Type,
		Literal: amd64Literal(value, 64),
	}}, []core.Value{result}, nil, core.EffectNone, address)
	return result.ID
}

func (l *amd64Lifter) readRegister(address uint64, name string, bits uint16) core.ValueID {
	result := l.newValue(core.TypeInteger, bits)
	l.emit(core.OpReadRegister, []core.Operand{amd64RegisterOperand(name, bits)}, []core.Value{result}, nil, core.EffectReadRegister, address)
	return result.ID
}

func (l *amd64Lifter) writeRegister(address uint64, name string, bits uint16, value core.ValueID) {
	l.emit(core.OpWriteRegister, []core.Operand{amd64RegisterOperand(name, bits), l.value(value)}, nil, nil, core.EffectWriteRegister, address)
}

func (l *amd64Lifter) load(address uint64, pointer core.ValueID, bits uint16) core.ValueID {
	result := l.newValue(core.TypeInteger, bits)
	l.emit(core.OpLoad, []core.Operand{l.value(pointer)}, []core.Value{result}, nil, core.EffectReadMemory, address)
	return result.ID
}

func (l *amd64Lifter) store(address uint64, pointer, value core.ValueID, bits uint16) {
	l.emit(core.OpStore, []core.Operand{l.value(pointer), l.value(value)}, nil, nil, core.EffectWriteMemory, address)
}

func (l *amd64Lifter) operation(ins amd64Instruction, kind core.Opcode, operands []core.Operand, bits uint16, effects core.Effect) core.ValueID {
	result := l.newValue(core.TypeInteger, bits)
	l.emit(kind, operands, []core.Value{result}, nil, effects, ins.address)
	return result.ID
}

func (l *amd64Lifter) newValue(kind core.TypeKind, bits uint16) core.Value {
	if bits == 0 {
		bits = 64
	}
	l.nextValue++
	value := core.Value{ID: l.nextValue, Type: core.ValueType{Kind: kind, Bits: bits}}
	l.values[value.ID] = value.Type
	return value
}

func (l *amd64Lifter) emit(kind core.Opcode, operands []core.Operand, results []core.Value, targets []core.BlockID, effects core.Effect, address uint64) {
	l.instructions = append(l.instructions, core.Instruction{
		Kind:     kind,
		Operands: operands,
		Results:  results,
		Targets:  targets,
		Effects:  effects,
		Address:  address,
	})
}

func amd64Args(ins amd64Instruction) []x86asm.Arg {
	args := make([]x86asm.Arg, 0, 4)
	for _, arg := range ins.inst.Args {
		if arg == nil {
			break
		}
		args = append(args, arg)
	}
	return args
}

func amd64Movable(arg x86asm.Arg) bool {
	switch value := arg.(type) {
	case x86asm.Reg:
		return amd64GPR(value) || (value >= x86asm.X0 && value <= x86asm.X15)
	case x86asm.Mem:
		return true
	default:
		return false
	}
}

func amd64Source(arg x86asm.Arg) bool {
	switch value := arg.(type) {
	case x86asm.Imm:
		return true
	case x86asm.Reg:
		return amd64GPR(value) || (value >= x86asm.X0 && value <= x86asm.X15)
	case x86asm.Mem:
		return true
	default:
		return false
	}
}

func amd64GPRDestination(arg x86asm.Arg) bool {
	switch value := arg.(type) {
	case x86asm.Reg:
		return amd64GPR(value)
	case x86asm.Mem:
		return true
	default:
		return false
	}
}

func amd64Memory(arg x86asm.Arg) bool {
	_, ok := arg.(x86asm.Mem)
	return ok
}

func amd64RegisterOperand(name string, bits uint16) core.Operand {
	return core.Operand{Kind: core.OperandRegister, Type: core.ValueType{Kind: core.TypeInteger, Bits: bits}, Name: name}
}

func (l *amd64Lifter) value(id core.ValueID) core.Operand {
	typ, ok := l.values[id]
	if !ok {
		typ = core.ValueType{Kind: core.TypeInteger, Bits: 64}
	}
	return core.Operand{Kind: core.OperandValue, Type: typ, Value: id}
}

func amd64Pointer(value uint64) core.Operand {
	return core.Operand{Kind: core.OperandConstant, Type: core.ValueType{Kind: core.TypePointer, Bits: 64}, Literal: amd64Literal(value, 64)}
}

func amd64AccumulatorNames(bits uint16) (string, string) {
	switch bits {
	case 8:
		return "al", "ah"
	case 16:
		return "ax", "dx"
	case 32:
		return "eax", "edx"
	default:
		return "rax", "rdx"
	}
}

func amd64Symbol(name string) core.Operand {
	return core.Operand{Kind: core.OperandSymbol, Name: name}
}

func amd64Literal(value uint64, bits uint16) []byte {
	size := int(bits / 8)
	if size == 0 {
		size = 1
	}
	literal := make([]byte, size)
	for index := range literal {
		literal[index] = byte(value >> (8 * index))
	}
	return literal
}

func containsBlock(blocks []core.BlockID, target core.BlockID) bool {
	for _, block := range blocks {
		if block == target {
			return true
		}
	}
	return false
}
