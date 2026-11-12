package x86

import (
	"fmt"

	"golang.org/x/arch/x86/x86asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// carryBinary lifts ADC and SBB. Both read the carry flag, so the carry input
// stays an explicit operand instead of being folded away.
func (l *lifter) carryBinary(ins instruction, kind core.Opcode, name string) {
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
	carry := l.carry(ins, bits)
	operands := []core.Operand{l.value(left), l.value(right), l.value(carry)}
	result := l.operation(ins, kind, operands, bits, core.EffectNone)
	// The carry and overflow outputs depend on the carry input, so the flag
	// update takes all three values.
	l.setFlags(ins, name, left, right, result)
	l.writePlace(ins, destination, result)
}

// carry reads the carry flag as a zero-extended bit.
func (l *lifter) carry(ins instruction, bits uint16) core.ValueID {
	flags := l.readRegister(ins.address, l.profile.Flags, l.profile.Bits)
	return l.operation(ins, core.OpIntrinsic, []core.Operand{symbolOperand("x86.flag.cf"), l.value(flags)}, 1, core.EffectReadRegister)
}

// rotate lifts ROL, ROR, RCL and RCR.
func (l *lifter) rotate(ins instruction, name string) {
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
	operands := []core.Operand{symbolOperand("x86." + name), l.value(value), l.value(count)}
	result := l.operation(ins, core.OpIntrinsic, operands, bits, core.EffectNone)
	l.writePlace(ins, destination, result)
	l.setFlags(ins, name, value, count, result)
}

// bitScan lifts BSF, BSR and BSWAP as one intrinsic each. The result depends on
// the bit layout of the source, so no shift sequence replaces it.
func (l *lifter) bitScan(ins instruction, name string) {
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
	source, ok := l.readSource(ins, args[1], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	result := l.operation(ins, core.OpIntrinsic, []core.Operand{symbolOperand("x86." + name), l.value(source)}, bits, core.EffectNone)
	l.writePlace(ins, destination, result)
	if name != "bswap" {
		l.setFlags(ins, name, source, result)
	}
}

// readModifyWrite lifts XADD and CMPXCHG. Both atomically combine a register
// with memory, so the operation keeps both sides and reports the atomic effect.
func (l *lifter) readModifyWrite(ins instruction, name string) {
	args := args(ins)
	if len(args) != 2 {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, args[0], l.profile.Bits)
	destination, ok := l.place(ins, args[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	source, ok := l.readSource(ins, args[1], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	current, ok := l.readPlace(ins, destination)
	if !ok {
		l.unsupported(ins)
		return
	}
	effects := core.EffectAtomic | core.EffectReadMemory | core.EffectWriteMemory
	if !destination.memory {
		effects = core.EffectAtomic
	}
	result := l.operation(ins, core.OpIntrinsic, []core.Operand{
		symbolOperand("x86." + name), l.value(current), l.value(source),
	}, bits, effects)
	l.writePlace(ins, destination, result)
	l.setFlags(ins, name, current, source, result)
}

// stringOperation lifts the REP-prefixed string moves and comparisons. The
// source and destination pointers stay implicit in the machine model, so the
// whole operation is one intrinsic with a memory effect.
func (l *lifter) stringOperation(ins instruction) {
	name := "x86." + lowerOp(ins.inst.Op)
	operands := []core.Operand{symbolOperand(name)}
	if hasPrefix(ins, x86asm.PrefixREP) {
		operands = append(operands, symbolOperand("x86.rep"))
	}
	effects := core.EffectReadMemory | core.EffectWriteMemory
	switch ins.inst.Op {
	case x86asm.MOVSB, x86asm.MOVSW, x86asm.MOVSD, x86asm.MOVSQ,
		x86asm.STOSB, x86asm.STOSW, x86asm.STOSD, x86asm.STOSQ:
	case x86asm.SCASB, x86asm.SCASW, x86asm.SCASD, x86asm.SCASQ,
		x86asm.CMPSB, x86asm.CMPSW, x86asm.CMPSD, x86asm.CMPSQ:
		effects |= core.EffectWriteRegister
	default:
		effects = core.EffectReadMemory
	}
	l.emit(core.OpIntrinsic, operands, nil, nil, effects|core.EffectUnknown, ins.address)
}

func lowerOp(op x86asm.Op) string {
	return x86OpName(op)
}

// x86OpName renders an opcode in lower case. x86asm spells string instructions
// with an operand suffix (MOVSD, MOVSQ), which would otherwise collide with the
// SSE MOVSD; the suffix is kept so both stay distinguishable.
func x86OpName(op x86asm.Op) string {
	name := op.String()
	result := make([]byte, 0, len(name))
	for index := 0; index < len(name); index++ {
		current := name[index]
		if current >= 'A' && current <= 'Z' {
			current += 'a' - 'A'
		}
		result = append(result, current)
	}
	return string(result)
}

var _ = fmt.Sprintf

// vectorRegisterPrefixes lists the register families that belong to the SSE and
// MMX files. x86asm names them X0..X15 and M0..M7, and the parser renders them
// as xmm0..xmm15 and mm0..mm7 so LowIR text matches the other architectures.
var vectorRegisterPrefixes = []string{"x", "m"}

// isVectorOpcode reports whether any operand is a vector register.
func isVectorOpcode(inst x86asm.Inst) bool {
	for _, arg := range inst.Args {
		if arg == nil {
			break
		}
		switch value := arg.(type) {
		case x86asm.Reg:
			if _, _, ok := vectorRegister(value); ok {
				return true
			}
		case x86asm.Mem:
			if value.Base != 0 {
				if _, _, ok := vectorRegister(value.Base); ok {
					return true
				}
			}
		}
	}
	return false
}

// vectorRegister returns the LowIR name and width of one SSE or MMX register.
// x86asm only knows the 128-bit XMM file and the 64-bit MMX file, so the width
// follows the file, not the mnemonic.
func vectorRegister(reg x86asm.Reg) (string, uint16, bool) {
	switch {
	case reg >= x86asm.X0 && reg <= x86asm.X15:
		return fmt.Sprintf("xmm%d", int(reg-x86asm.X0)), 128, true
	case reg >= x86asm.M0 && reg <= x86asm.M7:
		return fmt.Sprintf("mm%d", int(reg-x86asm.M0)), 64, true
	default:
		return "", 0, false
	}
}

func x86OpName2(reg x86asm.Reg) string {
	name := reg.String()
	result := make([]byte, 0, len(name))
	for index := 0; index < len(name); index++ {
		current := name[index]
		if current >= 'A' && current <= 'Z' {
			current += 'a' - 'A'
		}
		result = append(result, current)
	}
	return string(result)
}

// vectorOperation lifts one SSE or AVX instruction as an opaque register
// operation: the destination register receives a vector value, the sources are
// read, and the lane semantics stay unmodeled.
func (l *lifter) vectorOperation(ins instruction) {
	operands := []core.Operand{symbolOperand("x86." + lowerOp(ins.inst.Op))}
	arguments := args(ins)
	var results []core.Value
	effects := core.EffectNone
	for index, arg := range arguments {
		name, bits, ok := l.vectorRegisterOperand(arg)
		if ok {
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
			continue
		}
		if operand, ok := l.rawOperand(ins, arg); ok {
			operands = append(operands, operand)
			if arg != nil {
				effects |= core.EffectReadMemory
			}
		}
	}
	l.emit(core.OpIntrinsic, operands, results, nil, effects|core.EffectUnknown, ins.address)
}

// vectorRegisterOperand returns the name and width of a vector register operand.
func (l *lifter) vectorRegisterOperand(arg x86asm.Arg) (string, uint16, bool) {
	reg, ok := arg.(x86asm.Reg)
	if !ok {
		return "", 0, false
	}
	return vectorRegister(reg)
}

// bitModify lifts BTS, BTR and BTC: they test one bit and set, reset or
// complement it.
func (l *lifter) bitModify(ins instruction, name string) {
	arguments := args(ins)
	if len(arguments) != 2 || !isSource(arguments[0]) || !isSource(arguments[1]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, arguments[0], l.profile.Bits)
	value, ok := l.readSource(ins, arguments[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	index, ok := l.readSource(ins, arguments[1], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	result := l.operation(ins, core.OpIntrinsic, []core.Operand{
		symbolOperand("x86." + name), l.value(value), l.value(index),
	}, bits, core.EffectNone)
	l.setFlags(ins, name, value, index, result)
	if destination, ok := l.place(ins, arguments[0], bits); ok {
		l.writePlace(ins, destination, result)
	}
}

// timestamp lifts RDTSC, RDTSCP, RDPMC and CPUID. They read architectural
// state into registers; the values are not modeled, so the operation stays an
// intrinsic that writes the ABI-defined outputs.
func (l *lifter) timestamp(ins instruction) {
	name := "x86." + x86OpName(ins.inst.Op)
	low := "eax"
	high := "edx"
	if ins.inst.Op == x86asm.CPUID {
		low, high = "eax", "ebx"
	}
	l.emit(core.OpIntrinsic, []core.Operand{symbolOperand(name)}, nil, nil, core.EffectUnknown|core.EffectReadRegister, ins.address)
	for _, register := range []string{low, high} {
		value := l.newValue(core.TypeInteger, 32)
		l.emit(core.OpIntrinsic, []core.Operand{symbolOperand(name + "." + register)}, []core.Value{value}, nil, core.EffectWriteRegister|core.EffectUnknown, ins.address)
		l.writeRegister(ins.address, register, 32, value.ID)
	}
}

// bitCount lifts POPCNT, LZCNT and TZCNT. The count depends on the bit layout of
// the source, so one intrinsic carries it.
func (l *lifter) bitCount(ins instruction) {
	arguments := args(ins)
	if len(arguments) != 2 || !isGPRDestination(arguments[0]) || !isSource(arguments[1]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, arguments[0], l.profile.Bits)
	destination, ok := l.place(ins, arguments[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	source, ok := l.readSource(ins, arguments[1], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	result := l.operation(ins, core.OpIntrinsic, []core.Operand{
		symbolOperand("x86." + x86OpName(ins.inst.Op)), l.value(source),
	}, bits, core.EffectNone)
	l.writePlace(ins, destination, result)
	l.setFlags(ins, x86OpName(ins.inst.Op), source, result)
}

// byteSwap lifts BSWAP, which takes one register operand.
func (l *lifter) byteSwap(ins instruction) {
	arguments := args(ins)
	if len(arguments) != 1 || !isGPRDestination(arguments[0]) {
		l.unsupported(ins)
		return
	}
	bits := l.argBits(ins, arguments[0], l.profile.Bits)
	destination, ok := l.place(ins, arguments[0], bits)
	if !ok {
		l.unsupported(ins)
		return
	}
	value, ok := l.readPlace(ins, destination)
	if !ok {
		l.unsupported(ins)
		return
	}
	result := l.operation(ins, core.OpIntrinsic, []core.Operand{symbolOperand("x86.bswap"), l.value(value)}, bits, core.EffectNone)
	l.writePlace(ins, destination, result)
}

// x87Opcodes lists the floating-point unit mnemonics. The FPU stack is a
// separate register file, so the operations stay opaque instead of guessing a
// data flow.
var x87Opcodes = []string{
	"FLD", "FST", "FSTP", "FILD", "FIST", "FISTP", "FISTTP", "FLDZ", "FLD1",
	"FLDCW", "FNSTCW", "FSTCW", "FNSTSW", "FSTSW", "FCLEX", "FNCLEX", "FINIT",
	"FNINIT", "FCOMI", "FCOMIP", "FUCOMI", "FUCOMIP", "FCOM", "FCOMP", "FCOMPP",
	"FUCOM", "FUCOMP", "FUCOMPP", "FADD", "FADDP", "FSUB", "FSUBP", "FSUBR",
	"FSUBRP", "FMUL", "FMULP", "FDIV", "FDIVP", "FDIVR", "FDIVRP", "FSQRT",
	"FABS", "FCHS", "FSIN", "FCOS", "FPTAN", "FPATAN", "FSCALE", "FXCH",
	"FFREE", "FFREEP", "FNOP", "FDECSTP", "FINCSTP", "FPREM", "FPREM1",
	"FRNDINT", "FXAM", "FTST", "FLDL2E", "FLDL2T", "FLDLG2", "FLDLN2",
	"FLDPI", "FYL2X", "FYL2XP1", "F2XM1", "FWAIT", "FSTENV", "FNSTENV",
	"FLDENV", "FSAVE", "FNSAVE", "FRSTOR", "FBLD", "FBSTP", "FCMOVB",
	"FCMOVE", "FCMOVBE", "FCMOVU", "FCMOVNB", "FCMOVNE", "FCMOVNBE",
	"FCMOVNU",
}

// isX87Opcode reports whether the mnemonic belongs to the FPU.
func isX87Opcode(inst x86asm.Inst) bool {
	name := inst.Op.String()
	for _, candidate := range x87Opcodes {
		if name == candidate {
			return true
		}
	}
	for _, arg := range inst.Args {
		if arg == nil {
			break
		}
		switch value := arg.(type) {
		case x86asm.Reg:
			if value >= x86asm.F0 && value <= x86asm.F7 {
				return true
			}
		case x86asm.Mem:
			if value.Base != 0 && value.Base >= x86asm.F0 && value.Base <= x86asm.F7 {
				return true
			}
		}
	}
	return false
}

// x87Operation lifts one FPU instruction as an opaque operation: the register
// file is separate, so the operation keeps its mnemonic and its memory operand
// but no lane semantics.
func (l *lifter) x87Operation(ins instruction) {
	operands := []core.Operand{symbolOperand("x86." + x86OpName(ins.inst.Op))}
	effects := core.EffectReadRegister | core.EffectWriteRegister | core.EffectUnknown
	var results []core.Value
	for index, arg := range ins.inst.Args {
		if arg == nil {
			break
		}
		switch value := arg.(type) {
		case x86asm.Reg:
			name, bits, ok := l.profile.register(value)
			if !ok {
				continue
			}
			if index == 0 {
				result := l.newValue(core.TypeFloat, 80)
				results = append(results, result)
				operands = append(operands, core.Operand{Kind: core.OperandRegister, Type: result.Type, Name: name})
				continue
			}
			operands = append(operands, core.Operand{
				Kind: core.OperandRegister,
				Type: core.ValueType{Kind: core.TypeFloat, Bits: bits},
				Name: name,
			})
		case x86asm.Mem:
			address := l.address(ins, value)
			operands = append(operands, l.value(address))
			effects |= core.EffectReadMemory | core.EffectWriteMemory
		case x86asm.Imm:
			operands = append(operands, l.value(l.constant(ins.address, uint64(int64(value)), l.profile.Bits)))
		}
	}
	if len(results) == 0 {
		// Most FPU instructions leave their result on the stack, which has no
		// register name in LowIR.
		results = append(results, l.newValue(core.TypeFloat, 80))
	}
	l.emit(core.OpIntrinsic, operands, results, nil, effects, ins.address)
}

// architectureRegisterNames lists the control, debug, performance and
// model-specific registers the architectural instructions touch.
var architectureRegisterNames = []string{"eax", "edx", "ecx", "ebx"}

// architectural lifts privileged and architectural-register instructions. They
// do not contribute to the integer data flow, so only the mnemonic and the
// ABI-defined outputs are kept.
func (l *lifter) architectural(ins instruction) {
	operands := []core.Operand{symbolOperand("x86." + x86OpName(ins.inst.Op))}
	for _, arg := range ins.inst.Args {
		if arg == nil {
			break
		}
		if operand, ok := l.rawOperand(ins, arg); ok {
			operands = append(operands, operand)
		}
	}
	effects := core.EffectReadRegister | core.EffectWriteRegister | core.EffectUnknown
	switch ins.inst.Op {
	case x86asm.OUTSB, x86asm.OUTSW, x86asm.OUTSD:
		// String output reads memory and writes the port.
		effects |= core.EffectReadMemory
	case x86asm.INSB, x86asm.INSW, x86asm.INSD:
		// String input reads the port and writes memory.
		effects |= core.EffectWriteMemory
	case x86asm.OUT, x86asm.IN:
		// Port access does not touch memory in the register forms.
	}
	l.emit(core.OpIntrinsic, operands, nil, nil, effects, ins.address)
	switch ins.inst.Op {
	case x86asm.XGETBV, x86asm.RDRAND, x86asm.SMSW:
		for _, register := range architectureRegisterNames {
			value := l.newValue(core.TypeInteger, 32)
			l.emit(core.OpIntrinsic, []core.Operand{symbolOperand("x86." + x86OpName(ins.inst.Op) + "." + register)}, []core.Value{value}, nil, core.EffectWriteRegister|core.EffectUnknown, ins.address)
			l.writeRegister(ins.address, register, 32, value.ID)
		}
	}
}

// pushAll lifts PUSHAD and PUSHAL: the eight general registers are pushed in
// order, so the stack pointer moves by 32 bytes.
func (l *lifter) pushAll(ins instruction) {
	registers := []string{"eax", "ecx", "edx", "ebx", "esp", "ebp", "esi", "edi"}
	if l.profile.Bits == 64 {
		registers = []string{"rax", "rcx", "rdx", "rbx", "rsp", "rbp", "rsi", "rdi"}
	}
	width := l.profile.Bits
	sp := l.readRegister(ins.address, l.profile.StackPointer, width)
	count := l.constant(ins.address, 4*uint64(len(registers)), width)
	pointer := l.operation(ins, core.OpSub, []core.Operand{l.value(sp), l.value(count)}, width, core.EffectNone)
	l.writeRegister(ins.address, l.profile.StackPointer, width, pointer)
	cursor := pointer
	for index, register := range registers {
		if index > 0 {
			offset := l.constant(ins.address, 4, width)
			cursor = l.addAddress(ins, cursor, offset)
		}
		value := l.readRegister(ins.address, register, width)
		l.store(ins.address, cursor, value, width)
	}
}

// popAll lifts POPAD and POPAL.
func (l *lifter) popAll(ins instruction) {
	registers := []string{"edi", "esi", "ebp", "esp", "ebx", "edx", "ecx", "eax"}
	if l.profile.Bits == 64 {
		registers = []string{"rdi", "rsi", "rbp", "rsp", "rbx", "rdx", "rcx", "rax"}
	}
	width := l.profile.Bits
	cursor := l.readRegister(ins.address, l.profile.StackPointer, width)
	for index, register := range registers {
		if index > 0 {
			offset := l.constant(ins.address, 4, width)
			cursor = l.addAddress(ins, cursor, offset)
		}
		value := l.load(ins.address, cursor, width)
		l.writeRegister(ins.address, register, width, value)
	}
	sp := l.readRegister(ins.address, l.profile.StackPointer, width)
	count := l.constant(ins.address, 4*uint64(len(registers)), width)
	next := l.operation(ins, core.OpAdd, []core.Operand{l.value(sp), l.value(count)}, width, core.EffectNone)
	l.writeRegister(ins.address, l.profile.StackPointer, width, next)
}

// compareExchange lifts CMPXCHG8B and CMPXCHG16B. The operation compares a
// register pair with memory and swaps on equality, so it stays one atomic
// intrinsic with an explicit comparison result.
func (l *lifter) compareExchange(ins instruction) {
	arguments := args(ins)
	if len(arguments) != 1 || !isMemory(arguments[0]) {
		l.unsupported(ins)
		return
	}
	memory, ok := arguments[0].(x86asm.Mem)
	if !ok {
		l.unsupported(ins)
		return
	}
	address := l.address(ins, memory)
	width := uint16(64)
	if ins.inst.Op == x86asm.CMPXCHG16B {
		width = 128
	}
	result := l.operation(ins, core.OpIntrinsic, []core.Operand{
		symbolOperand("x86." + x86OpName(ins.inst.Op)),
		l.value(address),
	}, width, core.EffectAtomic|core.EffectReadMemory|core.EffectWriteMemory|core.EffectUnknown)
	l.setFlags(ins, x86OpName(ins.inst.Op), result)
}

// isStringMove reports whether a MOVS-family opcode has the two memory operands
// of the string form. In 32-bit mode the string move and the SSE move share the
// MOVSD mnemonic, so the operand shape is the only reliable test.
func isStringMove(ins instruction) bool {
	if ins.inst.Op != x86asm.MOVSD && ins.inst.Op != x86asm.MOVSQ {
		return false
	}
	for _, arg := range ins.inst.Args[:2] {
		if arg == nil {
			return false
		}
		if _, ok := arg.(x86asm.Mem); !ok {
			return false
		}
	}
	return true
}

// isSegmentRegister reports whether the operand is one of the six x86 segment
// registers.
func isSegmentRegister(arg x86asm.Arg) bool {
	reg, ok := arg.(x86asm.Reg)
	if !ok {
		return false
	}
	return reg >= x86asm.ES && reg <= x86asm.GS
}

// segmentMove lifts MOV to or from a segment register. The selector value is
// kept as a register or memory read; the segment base itself is not modeled.
func (l *lifter) segmentMove(ins instruction, arguments []x86asm.Arg) {
	segments := map[x86asm.Reg]string{
		x86asm.ES: "es", x86asm.CS: "cs", x86asm.SS: "ss",
		x86asm.DS: "ds", x86asm.FS: "fs", x86asm.GS: "gs",
	}
	effects := core.EffectReadRegister | core.EffectWriteRegister | core.EffectUnknown
	var operands []core.Operand
	for _, arg := range arguments {
		if reg, ok := arg.(x86asm.Reg); ok && reg >= x86asm.ES && reg <= x86asm.GS {
			operands = append(operands, registerOperand(segments[reg], 16))
			continue
		}
		if operand, ok := l.rawOperand(ins, arg); ok {
			operands = append(operands, operand)
		}
	}
	operands = append([]core.Operand{symbolOperand("x86.segment.move")}, operands...)
	l.emit(core.OpIntrinsic, operands, nil, nil, effects, ins.address)
}
