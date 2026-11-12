package x86

import (
	"fmt"
	"strings"

	"golang.org/x/arch/x86/x86asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

type lifter struct {
	profile      Profile
	blockIDs     map[uint64]core.BlockID
	diagnostics  *diagnostics
	nextValue    core.ValueID
	values       map[core.ValueID]core.ValueType
	instructions []core.Instruction
	variadic     bool
}

type place struct {
	reg     string
	bits    uint16
	memory  bool
	address core.ValueID
}

func liftFunctions(profile Profile, target core.Target, functions []function, blockIDs map[uint64]core.BlockID, diagnostics *diagnostics) *core.LowIR {
	ir := &core.LowIR{Target: target}
	for index, functionBody := range functions {
		ir.Functions = append(ir.Functions, liftFunction(profile, target, functionBody, core.FunctionID(index+1), blockIDs, diagnostics))
	}
	return ir
}

func liftFunction(profile Profile, target core.Target, functionBody function, id core.FunctionID, blockIDs map[uint64]core.BlockID, diagnostics *diagnostics) core.Function {
	blocks := functionBody.blocks
	function := core.Function{
		ID:      id,
		Address: blocks[0].start,
		Blocks:  make([]core.Block, 0, len(blocks)),
	}
	lifter := &lifter{profile: profile, blockIDs: blockIDs, diagnostics: diagnostics, values: make(map[core.ValueID]core.ValueType)}
	for index, block := range blocks {
		lifter.instructions = make([]core.Instruction, 0, len(block.instructions)*2)
		for _, instruction := range block.instructions {
			lifter.lift(instruction)
		}
		if len(lifter.instructions) == 0 || !controlTransfer(lifter.instructions[len(lifter.instructions)-1].Kind) {
			end := block.start
			if len(block.instructions) > 0 {
				last := block.instructions[len(block.instructions)-1]
				end = last.address + uint64(last.size)
			}
			if index+1 < len(blocks) {
				next := blockIDs[blocks[index+1].start]
				lifter.emit(core.OpJump, nil, nil, []core.BlockID{next}, core.EffectNone, end)
			} else {
				lifter.emit(core.OpTrap, []core.Operand{symbolOperand("region-end")}, nil, nil, core.EffectMayTrap|core.EffectUnknown, end)
				diagnostics.add("block-falls-off-region", fmt.Sprintf("block at 0x%x reaches the end of the decoded region", block.start))
			}
		}
		function.Blocks = append(function.Blocks, core.Block{ID: core.BlockID(index + 1), Address: block.start, Instructions: lifter.instructions})
	}
	return function
}

func variadicSetup(ins instruction) bool {
	args := args(ins)
	if len(args) != 2 {
		return false
	}
	destination, ok := args[0].(x86asm.Reg)
	if !ok || (destination != x86asm.AL && destination != x86asm.EAX && destination != x86asm.RAX) {
		return false
	}
	if source, ok := args[1].(x86asm.Reg); ok {
		return source == destination && ins.inst.Op == x86asm.XOR
	}
	if source, ok := args[1].(x86asm.Imm); ok {
		return source >= 0 && source <= 8
	}
	return false
}

func (l *lifter) lift(ins instruction) {
	if variadicSetup(ins) {
		l.variadic = true
	}
	if ins.endbr != "" {
		l.emit(core.OpIntrinsic, []core.Operand{symbolOperand("x86." + ins.endbr)}, nil, nil, core.EffectNone, ins.address)
		return
	}
	if ins.vector != nil {
		l.liftVector(ins)
		return
	}
	if hasPrefix(ins, x86asm.PrefixLOCK) {
		l.diagnostics.add("atomic-prefix-unmodeled", fmt.Sprintf("LOCK prefix at 0x%x is not modeled as an atomic effect", ins.address))
	}
	switch ins.inst.Op {
	case x86asm.NOP:
		l.emit(core.OpNop, nil, nil, nil, core.EffectNone, ins.address)
	case x86asm.MOV, x86asm.MOVAPS, x86asm.MOVUPS, x86asm.MOVDQA, x86asm.MOVDQU, x86asm.MOVAPD, x86asm.MOVUPD, x86asm.MOVD, x86asm.MOVQ:
		// MOVSD and MOVQ are also the names of the string and MMX moves. The
		// operand shape decides which one this is.
		if isStringMove(ins) {
			l.stringOperation(ins)
			return
		}
		if isVectorOpcode(ins.inst) {
			l.vectorOperation(ins)
			return
		}
		l.move(ins)
	case x86asm.MOVZX:
		l.extend(ins, core.OpZeroExtend)
	case x86asm.MOVSX, x86asm.MOVSXD:
		l.extend(ins, core.OpSignExtend)
	case x86asm.LEA:
		l.lea(ins)
	case x86asm.ADD:
		l.binary(ins, core.OpAdd, "add")
	case x86asm.ADC:
		l.carryBinary(ins, core.OpAdd, "adc")
	case x86asm.SUB:
		l.binary(ins, core.OpSub, "sub")
	case x86asm.SBB:
		l.carryBinary(ins, core.OpSub, "sbb")
	case x86asm.INC:
		l.increment(ins, core.OpAdd, "inc")
	case x86asm.DEC:
		l.increment(ins, core.OpSub, "dec")
	case x86asm.ROL:
		l.rotate(ins, "rol")
	case x86asm.ROR:
		l.rotate(ins, "ror")
	case x86asm.RCL:
		l.rotate(ins, "rcl")
	case x86asm.RCR:
		l.rotate(ins, "rcr")
	case x86asm.BSF:
		l.bitScan(ins, "bsf")
	case x86asm.BSR:
		l.bitScan(ins, "bsr")
	case x86asm.BSWAP:
		l.byteSwap(ins)
	case x86asm.XADD:
		l.readModifyWrite(ins, "xadd")
	case x86asm.CMPXCHG:
		l.readModifyWrite(ins, "cmpxchg")
	case x86asm.MOVSB, x86asm.MOVSW, x86asm.MOVSD, x86asm.MOVSQ,
		x86asm.STOSB, x86asm.STOSW, x86asm.STOSQ,
		x86asm.LODSB, x86asm.LODSW, x86asm.LODSQ,
		x86asm.SCASB, x86asm.SCASW, x86asm.SCASQ,
		x86asm.CMPSB, x86asm.CMPSW, x86asm.CMPSQ:
		l.stringOperation(ins)
	case x86asm.STOSD, x86asm.SCASD, x86asm.CMPSD:
		l.stringOperation(ins)
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
	case x86asm.BTS:
		l.bitModify(ins, "bts")
	case x86asm.BTR:
		l.bitModify(ins, "btr")
	case x86asm.BTC:
		l.bitModify(ins, "btc")
	case x86asm.RDTSC, x86asm.RDTSCP, x86asm.RDPMC, x86asm.CPUID:
		l.timestamp(ins)
	case x86asm.MFENCE, x86asm.LFENCE, x86asm.SFENCE, x86asm.PAUSE:
	case x86asm.MUL:
		l.multiply(ins)
	case x86asm.DIV:
		l.divide(ins, "div")
	case x86asm.IDIV:
		l.divide(ins, "idiv")
	case x86asm.PUNPCKLQDQ:
		l.vectorIntrinsic(ins, "x86.punpcklqdq")
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
	case x86asm.PUSHFQ, x86asm.PUSHF, x86asm.PUSHFD:
		width := l.profile.Bits
		if ins.inst.Op == x86asm.PUSHFQ {
			width = 64
		}
		l.pushValue(ins, l.readRegister(ins.address, l.profile.Flags, width))
	case x86asm.POPFQ, x86asm.POPF, x86asm.POPFD:
		width := l.profile.Bits
		if ins.inst.Op == x86asm.POPFQ {
			width = 64
		}
		l.popIntoRegister(ins, l.profile.Flags, width)
	case x86asm.CLD, x86asm.STD, x86asm.CLI, x86asm.STI, x86asm.CLC, x86asm.STC, x86asm.CMC:
		// Direction, interrupt and carry control. They change no register the
		// decompiler tracks, so the flag write stays visible as an intrinsic.
		l.emit(core.OpIntrinsic, []core.Operand{symbolOperand("x86." + x86OpName(ins.inst.Op))}, nil, nil,
			core.EffectWriteRegister|core.EffectUnknown, ins.address)
	case x86asm.POPCNT, x86asm.LZCNT, x86asm.TZCNT:
		l.bitCount(ins)
	case x86asm.PUSHAD:
		l.pushAll(ins)
	case x86asm.POPAD:
		l.popAll(ins)
	case x86asm.CMPXCHG8B, x86asm.CMPXCHG16B:
		l.compareExchange(ins)
	case x86asm.EMMS:
		// Empty MMX state: a no-op for the modeled registers.
		l.emit(core.OpNop, nil, nil, nil, core.EffectNone, ins.address)
	case x86asm.OUT, x86asm.OUTSB, x86asm.OUTSW, x86asm.OUTSD,
		x86asm.IN, x86asm.INSB, x86asm.INSW, x86asm.INSD,
		x86asm.XGETBV, x86asm.XSETBV, x86asm.RDRAND,
		x86asm.LGDT, x86asm.LIDT, x86asm.SGDT, x86asm.SIDT, x86asm.LLDT,
		x86asm.SLDT, x86asm.LTR, x86asm.STR, x86asm.LMSW, x86asm.SMSW,
		x86asm.LAR, x86asm.LSL, x86asm.VERR, x86asm.VERW, x86asm.CLTS,
		x86asm.INVD, x86asm.WBINVD, x86asm.SWAPGS, x86asm.SYSRET,
		x86asm.SYSEXIT, x86asm.INVLPG:
		// Privileged or architectural-register access: keep the mnemonic, the
		// registers it touches stay unknown.
		l.architectural(ins)
	case x86asm.VZEROUPPER:
		l.emit(core.OpIntrinsic, []core.Operand{symbolOperand("x86." + x86OpName(ins.inst.Op))}, nil, nil,
			core.EffectWriteRegister|core.EffectUnknown, ins.address)
	case x86asm.PREFETCHNTA, x86asm.PREFETCHT0, x86asm.PREFETCHT1, x86asm.PREFETCHT2,
		x86asm.CLFLUSH:
		l.emit(core.OpIntrinsic, []core.Operand{symbolOperand("x86." + x86OpName(ins.inst.Op))}, nil, nil,
			core.EffectReadMemory|core.EffectUnknown, ins.address)
	case x86asm.XLATB:
		l.emit(core.OpIntrinsic, []core.Operand{symbolOperand("x86.xlatb")}, nil, nil,
			core.EffectReadMemory|core.EffectUnknown, ins.address)
	case x86asm.LEAVE:
		l.leave(ins)
	case x86asm.XCHG:
		l.xchg(ins)
	case x86asm.CDQ:
		l.signExtendAccumulator(ins, "eax", "edx", 32)
	case x86asm.CQO:
		l.signExtendAccumulator(ins, l.profile.Accumulator, l.profile.AccumulatorHigh, 64)
	case x86asm.CWD:
		l.signExtendAccumulator(ins, "ax", "dx", 16)
	case x86asm.CDQE:
		l.extendFixed(ins, "eax", 32, l.profile.Accumulator, 64, core.OpSignExtend)
	case x86asm.CWDE:
		l.extendFixed(ins, "ax", 16, "eax", 32, core.OpSignExtend)
	case x86asm.JMP:
		l.jump(ins)
	case x86asm.CALL:
		l.call(ins)
	case x86asm.RET:
		l.emit(core.OpReturn, nil, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
	case x86asm.SYSCALL, x86asm.SYSENTER:
		l.emit(core.OpIntrinsic, []core.Operand{symbolOperand("x86." + strings.ToLower(ins.inst.Op.String()))}, nil, nil, core.EffectCall|core.EffectMayTrap|core.EffectUnknown, ins.address)
	case x86asm.INT:
		// Every INT is a trap; vector 3 is the breakpoint, others are system
		// calls or BIOS entries and carry the vector in the symbol.
		trap := "int3"
		if immediate, ok := ins.inst.Args[0].(x86asm.Imm); ok && immediate != 3 {
			trap = fmt.Sprintf("int %#x", uint64(immediate))
		}
		l.trap(ins, trap)
	case x86asm.UD2, x86asm.HLT:
		l.trap(ins, strings.ToLower(ins.inst.Op.String()))
	case x86asm.IRET, x86asm.IRETD, x86asm.IRETQ:
		l.emit(core.OpReturn, nil, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
	default:
		name := ins.inst.Op.String()
		switch {
		case isX87Opcode(ins.inst):
			l.x87Operation(ins)
		case isVectorOpcode(ins.inst):
			l.vectorOperation(ins)
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

func (l *lifter) move(ins instruction) {
	args := args(ins)
	if len(args) == 2 && (isSegmentRegister(args[0]) || isSegmentRegister(args[1])) {
		l.segmentMove(ins, args)
		return
	}
	if len(args) != 2 || !isMovable(args[0]) || !isSource(args[1]) {
		l.unsupported(ins)
		return
	}
	destinationBits := l.argBits(ins, args[0], l.profile.Bits)
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

func (l *lifter) extend(ins instruction, kind core.Opcode) {
	args := args(ins)
	if len(args) != 2 || !isGPRDestination(args[0]) || !isSource(args[1]) {
		l.unsupported(ins)
		return
	}
	destinationBits := l.argBits(ins, args[0], l.profile.Bits)
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

func (l *lifter) lea(ins instruction) {
	args := args(ins)
	if len(args) != 2 || !isGPRDestination(args[0]) {
		l.unsupported(ins)
		return
	}
	memory, ok := args[1].(x86asm.Mem)
	if !ok {
		l.unsupported(ins)
		return
	}
	destinationBits := l.argBits(ins, args[0], l.profile.Bits)
	destination, ok := l.place(ins, args[0], destinationBits)
	if !ok {
		l.unsupported(ins)
		return
	}
	address := l.address(ins, memory)
	if destinationBits != l.profile.Bits {
		address = l.operation(ins, core.OpTruncate, []core.Operand{l.value(address)}, destinationBits, core.EffectNone)
	}
	l.writePlace(ins, destination, address)
}

func (l *lifter) binary(ins instruction, kind core.Opcode, flag string) {
	args := args(ins)
	if len(args) != 2 || !isGPRDestination(args[0]) || !isSource(args[1]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], l.profile.Bits)
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

func (l *lifter) vectorBinary(ins instruction) {
	args := args(ins)
	if len(args) != 2 || !isMovable(args[0]) || !isSource(args[1]) {
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

func (l *lifter) compare(ins instruction, flag string) {
	args := args(ins)
	if len(args) != 2 || !isGPRDestination(args[0]) || !isSource(args[1]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], l.profile.Bits)
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

func (l *lifter) bitTest(ins instruction) {
	args := args(ins)
	if len(args) != 2 || !isSource(args[0]) || !isSource(args[1]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], l.profile.Bits)
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

func (l *lifter) multiply(ins instruction) {
	args := args(ins)
	if len(args) != 1 || !isSource(args[0]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], l.profile.Bits)
	accumulator, high := l.profile.accumulatorNames(bits)
	left := l.readRegister(ins.address, accumulator, bits)
	right, ok := l.readSource(ins, args[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	low := l.operation(ins, core.OpIntrinsic, []core.Operand{symbolOperand("x86.mul.lo"), l.value(left), l.value(right)}, bits, core.EffectNone)
	upper := l.operation(ins, core.OpIntrinsic, []core.Operand{symbolOperand("x86.mul.hi"), l.value(left), l.value(right)}, bits, core.EffectNone)
	l.writeRegister(ins.address, accumulator, bits, low)
	l.writeRegister(ins.address, high, bits, upper)
	l.setFlags(ins, "mul", low, upper)
}

func (l *lifter) divide(ins instruction, name string) {
	args := args(ins)
	if len(args) != 1 || !isSource(args[0]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], l.profile.Bits)
	quotient, remainder := l.profile.accumulatorNames(bits)
	low := l.readRegister(ins.address, quotient, bits)
	high := l.readRegister(ins.address, remainder, bits)
	divisor, ok := l.readSource(ins, args[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	operands := []core.Operand{l.value(high), l.value(low), l.value(divisor)}
	quot := l.operation(ins, core.OpIntrinsic, append([]core.Operand{symbolOperand("x86." + name + ".quot")}, operands...), bits, core.EffectMayTrap)
	rem := l.operation(ins, core.OpIntrinsic, append([]core.Operand{symbolOperand("x86." + name + ".rem")}, operands...), bits, core.EffectMayTrap)
	l.writeRegister(ins.address, quotient, bits, quot)
	l.writeRegister(ins.address, remainder, bits, rem)
}

func (l *lifter) vectorIntrinsic(ins instruction, symbol string) {
	args := args(ins)
	if len(args) != 2 || !isMovable(args[0]) || !isSource(args[1]) {
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
	result := l.operation(ins, core.OpIntrinsic, []core.Operand{symbolOperand(symbol), l.value(current), l.value(source)}, 128, core.EffectNone)
	l.writePlace(ins, destination, result)
}

func (l *lifter) increment(ins instruction, kind core.Opcode, flag string) {
	args := args(ins)
	if len(args) != 1 || !isGPRDestination(args[0]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], l.profile.Bits)
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

func (l *lifter) unary(ins instruction, kind core.Opcode, flag string) {
	args := args(ins)
	if len(args) != 1 || !isGPRDestination(args[0]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], l.profile.Bits)
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

func (l *lifter) imul(ins instruction) {
	args := args(ins)
	if len(args) == 1 {
		// One-operand IMUL multiplies the accumulator and writes the full
		// product to the accumulator pair, exactly like MUL but signed.
		if !isSource(args[0]) {
			l.unsupported(ins)
			return
		}
		bits := l.argBits(ins, args[0], l.profile.Bits)
		accumulator, high := l.profile.accumulatorNames(bits)
		left := l.readRegister(ins.address, accumulator, bits)
		right, ok := l.readSource(ins, args[0], bits)
		if !ok {
			l.unsupported(ins)
			return
		}
		low := l.operation(ins, core.OpMul, []core.Operand{l.value(left), l.value(right)}, bits, core.EffectNone)
		upper := l.operation(ins, core.OpIntrinsic, []core.Operand{
			symbolOperand("x86.imul.hi"), l.value(left), l.value(right),
		}, bits, core.EffectNone)
		l.writeRegister(ins.address, accumulator, bits, low)
		l.writeRegister(ins.address, high, bits, upper)
		l.setFlags(ins, "imul", low, upper)
		return
	}
	if len(args) != 2 && len(args) != 3 {
		l.unsupported(ins)
		return
	}
	if !isGPRDestination(args[0]) || !isSource(args[1]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], l.profile.Bits)
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

func (l *lifter) shift(ins instruction, kind core.Opcode, flag string) {
	args := args(ins)
	if len(args) != 2 || !isGPRDestination(args[0]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], l.profile.Bits)
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

func (l *lifter) push(ins instruction) {
	args := args(ins)
	if len(args) != 1 || !isSource(args[0]) {
		l.unsupported(ins)
		return
	}
	value, ok := l.readSource(ins, args[0], l.profile.Bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	l.pushValue(ins, value)
}

func (l *lifter) pushValue(ins instruction, value core.ValueID) {
	bits := l.profile.Bits
	slot := l.profile.stackSlot()
	rsp := l.readRegister(ins.address, l.profile.StackPointer, bits)
	distance := l.constant(ins.address, slot, bits)
	next := l.operation(ins, core.OpSub, []core.Operand{l.value(rsp), l.value(distance)}, bits, core.EffectNone)
	l.writeRegister(ins.address, l.profile.StackPointer, bits, next)
	l.store(ins.address, l.pointerFrom(ins.address, next), value, bits)
}

func (l *lifter) pop(ins instruction) {
	args := args(ins)
	if len(args) != 1 || (!isGPRDestination(args[0]) && !isMemory(args[0])) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], l.profile.Bits)
	destination, ok := l.place(ins, args[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	pointerBits := l.profile.Bits
	rsp := l.readRegister(ins.address, l.profile.StackPointer, pointerBits)
	value := l.load(ins.address, l.pointerFrom(ins.address, rsp), bits)
	l.writePlace(ins, destination, value)
	if destination.memory || destination.reg != l.profile.StackPointer {
		distance := l.constant(ins.address, l.profile.stackSlot(), pointerBits)
		next := l.operation(ins, core.OpAdd, []core.Operand{l.value(rsp), l.value(distance)}, pointerBits, core.EffectNone)
		l.writeRegister(ins.address, l.profile.StackPointer, pointerBits, next)
	}
}

func (l *lifter) popIntoRegister(ins instruction, name string, bits uint16) {
	pointerBits := l.profile.Bits
	rsp := l.readRegister(ins.address, l.profile.StackPointer, pointerBits)
	value := l.load(ins.address, l.pointerFrom(ins.address, rsp), bits)
	l.writeRegister(ins.address, name, bits, value)
	distance := l.constant(ins.address, l.profile.stackSlot(), pointerBits)
	next := l.operation(ins, core.OpAdd, []core.Operand{l.value(rsp), l.value(distance)}, pointerBits, core.EffectNone)
	l.writeRegister(ins.address, l.profile.StackPointer, pointerBits, next)
}

func (l *lifter) leave(ins instruction) {
	bits := l.profile.Bits
	frame := l.readRegister(ins.address, l.profile.StackFrame, bits)
	l.writeRegister(ins.address, l.profile.StackPointer, bits, frame)
	l.popIntoRegister(ins, l.profile.StackFrame, bits)
}

func (l *lifter) xchg(ins instruction) {
	args := args(ins)
	if len(args) != 2 || (!isGPRDestination(args[0]) && !isMemory(args[0])) || (!isGPRDestination(args[1]) && !isMemory(args[1])) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], l.profile.Bits)
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

func (l *lifter) signExtendAccumulator(ins instruction, source, destination string, bits uint16) {
	value := l.readRegister(ins.address, source, bits)
	shift := l.constant(ins.address, uint64(bits-1), bits)
	sign := l.operation(ins, core.OpAShr, []core.Operand{l.value(value), l.value(shift)}, bits, core.EffectNone)
	l.writeRegister(ins.address, destination, bits, sign)
}

func (l *lifter) extendFixed(ins instruction, source string, sourceBits uint16, destination string, destinationBits uint16, kind core.Opcode) {
	value := l.readRegister(ins.address, source, sourceBits)
	extended := l.operation(ins, kind, []core.Operand{l.value(value)}, destinationBits, core.EffectNone)
	l.writeRegister(ins.address, destination, destinationBits, extended)
}

func (l *lifter) jump(ins instruction) {
	if target, ok := directTarget(ins); ok {
		if block, ok := l.blockIDs[target]; ok {
			l.emit(core.OpJump, nil, nil, []core.BlockID{block}, core.EffectNone, ins.address)
			return
		}
		l.emit(core.OpJump, []core.Operand{l.pointerOperand(target)}, nil, nil, core.EffectUnknown, ins.address)
		return
	}
	args := args(ins)
	if len(args) != 1 {
		l.unsupported(ins)
		return
	}
	value, ok := l.readSource(ins, args[0], l.profile.Bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	value = l.pointerFrom(ins.address, value)
	l.emit(core.OpJump, []core.Operand{l.value(value)}, nil, nil, core.EffectUnknown, ins.address)
}

func (l *lifter) branch(ins instruction) {
	condition := l.condition(ins)
	if condition == 0 {
		return
	}
	operands := []core.Operand{l.value(condition)}
	var targets []core.BlockID
	if target, ok := directTarget(ins); ok {
		if block, ok := l.blockIDs[target]; ok {
			targets = append(targets, block)
		} else {
			operands = append(operands, l.pointerOperand(target))
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

func (l *lifter) call(ins instruction) {
	if target, ok := directTarget(ins); ok {
		instruction := core.Instruction{
			Kind:     core.OpCall,
			Operands: []core.Operand{l.pointerOperand(target)},
			Effects:  core.EffectCall | core.EffectMayTrap | core.EffectUnknown,
			Variadic: l.variadic,
			Address:  ins.address,
		}
		l.variadic = false
		l.instructions = append(l.instructions, instruction)
		return
	}
	args := args(ins)
	if len(args) != 1 {
		l.unsupported(ins)
		return
	}
	value, ok := l.readSource(ins, args[0], l.profile.Bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	value = l.pointerFrom(ins.address, value)
	instruction := core.Instruction{
		Kind:     core.OpCall,
		Operands: []core.Operand{l.value(value)},
		Effects:  core.EffectCall | core.EffectMayTrap | core.EffectUnknown,
		Variadic: l.variadic,
		Address:  ins.address,
	}
	l.variadic = false
	l.instructions = append(l.instructions, instruction)
}

func (l *lifter) setCondition(ins instruction) {
	condition := l.condition(ins)
	if condition == 0 {
		return
	}
	args := args(ins)
	if len(args) != 1 || (!isGPRDestination(args[0]) && !isMemory(args[0])) {
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

func (l *lifter) conditionalMove(ins instruction) {
	condition := l.condition(ins)
	if condition == 0 {
		return
	}
	args := args(ins)
	if len(args) != 2 || !isGPRDestination(args[0]) || !isSource(args[1]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], l.profile.Bits)
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

func (l *lifter) condition(ins instruction) core.ValueID {
	code := conditionName(ins.inst.Op)
	if code == "" {
		return 0
	}
	if code == "rcxz" || code == "ecxz" || code == "cxz" {
		name := l.profile.CountRegister
		bits := l.profile.Bits
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
	flags := l.readRegister(ins.address, l.profile.Flags, l.profile.Bits)
	return l.operation(ins, core.OpIntrinsic, []core.Operand{symbolOperand("x86.cond." + code), l.value(flags)}, 1, core.EffectReadRegister)
}

func (l *lifter) setFlags(ins instruction, name string, values ...core.ValueID) {
	operands := make([]core.Operand, 0, len(values)+1)
	operands = append(operands, symbolOperand("x86.flags."+name))
	for _, value := range values {
		operands = append(operands, l.value(value))
	}
	flags := l.operation(ins, core.OpIntrinsic, operands, l.profile.Bits, core.EffectWriteRegister)
	l.writeRegister(ins.address, l.profile.Flags, l.profile.Bits, flags)
}

func (l *lifter) trap(ins instruction, name string) {
	l.emit(core.OpTrap, []core.Operand{symbolOperand("x86." + name)}, nil, nil, core.EffectMayTrap|core.EffectUnknown, ins.address)
}

func (l *lifter) unsupported(ins instruction) {
	operands := make([]core.Operand, 0, 4)
	for _, arg := range args(ins) {
		if arg == nil {
			continue
		}
		operand, ok := l.rawOperand(ins, arg)
		if ok {
			operands = append(operands, operand)
		}
	}
	l.emit(core.OpIntrinsic, append([]core.Operand{symbolOperand("x86." + strings.ToLower(ins.inst.Op.String()))}, operands...), nil, nil, core.EffectUnknown, ins.address)
	l.diagnostics.add("unsupported-instruction", fmt.Sprintf("unsupported x86 instruction %s at 0x%x", ins.inst.Op, ins.address))
}

func (l *lifter) rawOperand(ins instruction, arg x86asm.Arg) (core.Operand, bool) {
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
		return l.pointerOperand(ins.address + uint64(ins.size) + uint64(int64(value))), true
	default:
		return core.Operand{}, false
	}
}

func (l *lifter) place(ins instruction, arg x86asm.Arg, bits uint16) (place, bool) {
	switch value := arg.(type) {
	case x86asm.Reg:
		name, registerBits, ok := l.profile.register(value)
		if !ok {
			return place{}, false
		}
		if bits == 0 {
			bits = registerBits
		}
		return place{reg: name, bits: bits}, true
	case x86asm.Mem:
		if bits == 0 {
			bits = l.argBits(ins, value, l.profile.Bits)
		}
		return place{memory: true, bits: bits, address: l.address(ins, value)}, true
	default:
		return place{}, false
	}
}

func (l *lifter) readPlace(ins instruction, place place) (core.ValueID, bool) {
	if place.memory {
		return l.load(ins.address, place.address, place.bits), true
	}
	return l.readRegister(ins.address, place.reg, place.bits), true
}

func (l *lifter) writePlace(ins instruction, place place, value core.ValueID) {
	if place.memory {
		l.store(ins.address, place.address, value, place.bits)
		return
	}
	l.writeRegister(ins.address, place.reg, place.bits, value)
}

func (l *lifter) readSource(ins instruction, arg x86asm.Arg, bits uint16) (core.ValueID, bool) {
	switch value := arg.(type) {
	case x86asm.Imm:
		return l.constant(ins.address, uint64(int64(value)), bits), true
	case x86asm.Reg:
		return l.readRegisterOperand(ins.address, value)
	case x86asm.Mem:
		if bits == 0 {
			bits = l.argBits(ins, value, l.profile.Bits)
		}
		return l.load(ins.address, l.address(ins, value), bits), true
	case x86asm.Rel:
		return l.constant(ins.address, ins.address+uint64(ins.size)+uint64(int64(value)), 64), true
	default:
		return 0, false
	}
}

// RIP-relative displacements are relative to the end of the instruction.
func (l *lifter) address(ins instruction, memory x86asm.Mem) core.ValueID {
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
		result = l.operation(ins, core.OpIntrinsic, []core.Operand{symbolOperand("x86.segment." + segment), l.value(result)}, 64, core.EffectReadRegister|core.EffectUnknown)
	}
	return result
}

func (l *lifter) addAddress(ins instruction, left, right core.ValueID) core.ValueID {
	if left == 0 {
		return l.pointerFrom(ins.address, right)
	}
	result := l.newValue(core.TypePointer, l.profile.Bits)
	l.emit(core.OpAdd, []core.Operand{l.value(left), l.value(right)}, []core.Value{result}, nil, core.EffectNone, ins.address)
	return result.ID
}

func (l *lifter) readAddressRegister(ins instruction, reg x86asm.Reg) (core.ValueID, bool) {
	name, bits, ok := l.profile.register(reg)
	if !ok {
		return 0, false
	}
	value := l.readRegister(ins.address, name, bits)
	if bits < l.profile.Bits {
		value = l.coerce(ins, value, bits, l.profile.Bits, false)
	}
	return value, true
}

func (l *lifter) pointerFrom(address uint64, value core.ValueID) core.ValueID {
	if l.values[value].Kind == core.TypePointer {
		return value
	}
	result := l.newValue(core.TypePointer, l.profile.Bits)
	l.emit(core.OpCopy, []core.Operand{l.value(value)}, []core.Value{result}, nil, core.EffectNone, address)
	return result.ID
}

func (l *lifter) readRegisterOperand(address uint64, reg x86asm.Reg) (core.ValueID, bool) {
	name, bits, ok := l.profile.register(reg)
	if !ok {
		return 0, false
	}
	return l.readRegister(address, name, bits), true
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

func (l *lifter) argBits(ins instruction, arg x86asm.Arg, fallback uint16) uint16 {
	switch value := arg.(type) {
	case x86asm.Reg:
		if _, bits, ok := l.profile.register(value); ok {
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

func (l *lifter) load(address uint64, pointer core.ValueID, bits uint16) core.ValueID {
	result := l.newValue(core.TypeInteger, bits)
	l.emit(core.OpLoad, []core.Operand{l.value(pointer)}, []core.Value{result}, nil, core.EffectReadMemory, address)
	return result.ID
}

func (l *lifter) store(address uint64, pointer, value core.ValueID, bits uint16) {
	l.emit(core.OpStore, []core.Operand{l.value(pointer), l.value(value)}, nil, nil, core.EffectWriteMemory, address)
}

func (l *lifter) operation(ins instruction, kind core.Opcode, operands []core.Operand, bits uint16, effects core.Effect) core.ValueID {
	result := l.newValue(core.TypeInteger, bits)
	l.emit(kind, operands, []core.Value{result}, nil, effects, ins.address)
	return result.ID
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

func args(ins instruction) []x86asm.Arg {
	args := make([]x86asm.Arg, 0, 4)
	for _, arg := range ins.inst.Args {
		if arg == nil {
			break
		}
		args = append(args, arg)
	}
	return args
}

func isMovable(arg x86asm.Arg) bool {
	switch value := arg.(type) {
	case x86asm.Reg:
		return isGPR(value) || (value >= x86asm.X0 && value <= x86asm.X15)
	case x86asm.Mem:
		return true
	default:
		return false
	}
}

func isSource(arg x86asm.Arg) bool {
	switch value := arg.(type) {
	case x86asm.Imm:
		return true
	case x86asm.Reg:
		return isGPR(value) || (value >= x86asm.X0 && value <= x86asm.X15)
	case x86asm.Mem:
		return true
	default:
		return false
	}
}

func isGPRDestination(arg x86asm.Arg) bool {
	switch value := arg.(type) {
	case x86asm.Reg:
		return isGPR(value)
	case x86asm.Mem:
		return true
	default:
		return false
	}
}

func isMemory(arg x86asm.Arg) bool {
	_, ok := arg.(x86asm.Mem)
	return ok
}

func registerOperand(name string, bits uint16) core.Operand {
	return core.Operand{Kind: core.OperandRegister, Type: core.ValueType{Kind: core.TypeInteger, Bits: bits}, Name: name}
}

func (l *lifter) value(id core.ValueID) core.Operand {
	typ, ok := l.values[id]
	if !ok {
		typ = core.ValueType{Kind: core.TypeInteger, Bits: 64}
	}
	return core.Operand{Kind: core.OperandValue, Type: typ, Value: id}
}

func (l *lifter) pointerOperand(value uint64) core.Operand {
	return core.Operand{
		Kind:    core.OperandConstant,
		Type:    core.ValueType{Kind: core.TypePointer, Bits: l.profile.Bits},
		Literal: literal(value, l.profile.Bits),
	}
}

func accumulatorNames(bits uint16) (string, string) {
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

func symbolOperand(name string) core.Operand {
	return core.Operand{Kind: core.OperandSymbol, Name: name}
}

func literal(value uint64, bits uint16) []byte {
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
