package ppc

import (
	"fmt"
	"strings"

	"golang.org/x/arch/ppc64/ppc64asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// readOperand reads one register or immediate operand as a value.
func (l *lifter) readOperand(ins instruction, arg ppc64asm.Arg) (core.ValueID, bool) {
	if reg, ok := arg.(ppc64asm.Reg); ok {
		name, _, ok := registerName(reg)
		if !ok {
			return 0, false
		}
		if isFloatRegister(name) || isVectorRegister(name) {
			return 0, false
		}
		return l.readRegister(ins.address, name, l.profile.Bits), true
	}
	if value, ok := signedImmediate(arg); ok {
		return l.constant(ins.address, uint64(value), l.profile.Bits), true
	}
	return 0, false
}

// arithmetic lifts the integer ALU instructions. The destination is the first
// register operand; the inputs follow.
func (l *lifter) arithmetic(ins instruction) (core.ValueID, bool) {
	name := normalize(opName(ins.inst.Op))
	arguments := args(ins)
	bits := l.profile.Bits

	switch name {
	case "li", "lis", "mr":
		if len(arguments) < 2 {
			return 0, false
		}
		destination, ok := l.destinationName(arguments[0])
		if !ok {
			return 0, false
		}
		value, ok := l.readOperand(ins, arguments[1])
		if !ok {
			return 0, false
		}
		if name == "lis" {
			value = l.operation(ins, core.OpShl, []core.Operand{l.value(value), l.value(l.constant(ins.address, 16, bits))}, bits, core.EffectNone)
		}
		l.writeRegister(ins.address, destination, bits, value)
		return value, true
	case "nop":
		l.emit(core.OpNop, nil, nil, nil, core.EffectNone, ins.address)
		return 0, true
	case "cmpwi", "cmpdi", "cmplwi", "cmpldi", "cmpw", "cmpd", "cmplw", "cmpld":
		return l.compare(ins, name)
	case "extsb", "extsh", "extsw", "extswsli", "cntlzw", "cntlzd", "cnttzd", "popcntd", "popcntw":
		return l.unary(ins, name)
	case "mfspr", "mtspr", "mfcr", "mtcrf", "mfctr", "mtctr", "mflr", "mtlr", "mfxer", "mtxer":
		return l.specialRegister(ins, name)
	case "rlwinm", "rlwimi", "rldicl", "rldicr", "rldic", "rldimi", "rlwnm", "rldcl", "rldcr":
		return l.rotateMask(ins, name)
	case "isel":
		return l.selectInstruction(ins)
	}

	kind, ok := binaryOpcode(name)
	if !ok {
		return 0, false
	}
	if len(arguments) < 2 {
		return 0, false
	}
	destination, ok := l.destinationName(arguments[0])
	if !ok {
		return 0, false
	}
	left, ok := l.readOperand(ins, arguments[1])
	if !ok {
		return 0, false
	}
	var right core.ValueID
	if len(arguments) >= 3 {
		right, ok = l.readOperand(ins, arguments[2])
		if !ok {
			return 0, false
		}
		if shiftedImmediate(name) {
			// The "is" forms carry the high half of a 32-bit constant: addis,
			// andis, oris and xoris all shift their immediate left by sixteen.
			right = l.operation(ins, core.OpShl, []core.Operand{l.value(right), l.value(l.constant(ins.address, 16, bits))}, bits, core.EffectNone)
		}
	} else {
		right = left
	}
	result := l.operation(ins, kind, []core.Operand{l.value(left), l.value(right)}, bits, core.EffectNone)
	l.writeRegister(ins.address, destination, bits, result)
	return result, true
}

// shiftedImmediate reports whether one mnemonic shifts its immediate left by
// sixteen bits. PowerPC spells those forms with a trailing "is".
func shiftedImmediate(name string) bool {
	switch name {
	case "addis", "andis", "oris", "xoris":
		return true
	}
	return false
}

// destinationName returns the register name of one destination operand.
func (l *lifter) destinationName(arg ppc64asm.Arg) (string, bool) {
	reg, ok := arg.(ppc64asm.Reg)
	if !ok {
		return "", false
	}
	name, _, ok := registerName(reg)
	return name, ok
}

// unary lifts the sign-extension and bit-count instructions.
func (l *lifter) unary(ins instruction, name string) (core.ValueID, bool) {
	arguments := args(ins)
	if len(arguments) != 2 {
		return 0, false
	}
	destination, ok := l.destinationName(arguments[0])
	if !ok {
		return 0, false
	}
	value, ok := l.readOperand(ins, arguments[1])
	if !ok {
		return 0, false
	}
	bits := l.profile.Bits
	var result core.ValueID
	switch name {
	case "extsb", "extsh", "extsw":
		result = l.operation(ins, core.OpSignExtend, []core.Operand{l.value(value)}, bits, core.EffectNone)
	default:
		result = l.operation(ins, core.OpIntrinsic, []core.Operand{
			symbolOperand("ppc." + name), l.value(value),
		}, bits, core.EffectNone)
	}
	l.writeRegister(ins.address, destination, bits, result)
	return result, true
}

// compare lifts the compare instructions. The result is written to the
// condition register, which the conditional branches read.
func (l *lifter) compare(ins instruction, name string) (core.ValueID, bool) {
	arguments := args(ins)
	if len(arguments) < 2 {
		return 0, false
	}
	index := 0
	fieldName := "cr0"
	if reg, ok := arguments[0].(ppc64asm.CondReg); ok {
		fieldName = "cr" + conditionIndex(reg)
		index = 1
	}
	if len(arguments) < index+2 {
		return 0, false
	}
	left, ok := l.readOperand(ins, arguments[index])
	if !ok {
		return 0, false
	}
	right, ok := l.readOperand(ins, arguments[index+1])
	if !ok {
		return 0, false
	}
	result := l.operation(ins, core.OpIntrinsic, []core.Operand{
		symbolOperand("flags." + name), l.value(left), l.value(right),
	}, 32, core.EffectWriteRegister)
	l.writeRegister(ins.address, fieldName, 32, result)
	l.flagsValue = result
	l.hasFlagsValue = true
	return result, true
}

// conditionIndex renders the CR field number of a condition operand.
func conditionIndex(reg ppc64asm.CondReg) string {
	text := reg.String()
	if len(text) == 0 {
		return "0"
	}
	return text[len(text)-1:]
}

func binaryOpcode(name string) (core.Opcode, bool) {
	switch normalize(name) {
	case "add", "addi", "addis", "addc", "adde", "addme", "addze", "addic":
		return core.OpAdd, true
	case "subf", "subfc", "subfe", "subfic", "subfme", "subfze", "neg":
		return core.OpSub, true
	case "and", "andi", "andis", "andc":
		return core.OpAnd, true
	case "or", "ori", "oris", "orc", "nor":
		return core.OpOr, true
	case "xor", "xori", "xoris", "eqv":
		return core.OpXor, true
	case "slw", "sld", "sraw", "srawi", "srad", "sradi", "sla", "slwi", "sldi", "srwi", "srdi":
		return core.OpShl, true
	case "srw", "srd":
		return core.OpLShr, true
	case "mulld", "mullw", "mulli", "mulhd", "mulhdu", "mulhw", "mulhwu":
		return core.OpMul, true
	case "divd", "divw", "divdu", "divwu":
		return core.OpSDiv, true
	case "fadd", "fadds", "fsub", "fsubs", "fmul", "fmuls", "fdiv", "fdivs":
		return core.OpIntrinsic, true
	}
	return "", false
}

// call lifts BL and its variants: the link register receives the return address.
func (l *lifter) call(ins instruction) {
	target, ok := branchTarget(ins)
	if ok {
		l.emit(core.OpCall, []core.Operand{pointerOperand(target, l.profile.Bits)}, nil, nil,
			core.EffectCall|core.EffectMayTrap|core.EffectUnknown, ins.address)
	} else {
		// bctrl and blrl call through a register; the target is the current
		// link-register value at run time, which is not modeled.
		l.emit(core.OpCall, []core.Operand{symbolOperand("ppc.link-register")}, nil, nil,
			core.EffectCall|core.EffectMayTrap|core.EffectUnknown, ins.address)
	}
	l.writeRegister(ins.address, "lr", l.profile.Bits, l.constant(ins.address, ins.address+uint64(ins.size), l.profile.Bits))
}

// branch lifts BC and the decrement-count forms: the condition register decides
// the successor.
func (l *lifter) branch(ins instruction) {
	target, ok := branchTarget(ins)
	if !ok {
		l.unsupported(ins)
		return
	}
	name := opName(ins.inst.Op)
	targets := []core.BlockID{}
	if block, ok := l.blockIDs[target]; ok {
		targets = append(targets, block)
	}
	next := ins.address + uint64(ins.size)
	if block, ok := l.blockIDs[next]; ok && !containsBlock(targets, block) {
		targets = append(targets, block)
	}
	// A branch names the condition register bit it tests. Without that bit the
	// condition reads as the whole compare result, which is what the lifted value
	// carries anyway.
	symbol := "ppc.branch." + name
	if cc, ok := branchConditionCode(ins); ok {
		symbol = core.ConditionIntrinsicPrefix + cc
	}
	// A branch that ignores the condition register has one successor. Listing the
	// instruction after it as a second successor invents a condition the program
	// never tests.
	if len(targets) == 1 || unconditionalBranch(ins, name) {
		l.emit(core.OpJump, nil, nil, targets[:1], core.EffectNone, ins.address)
		return
	}
	condition := l.newValue(core.TypeInteger, 1)
	// Name the comparison itself when this block made one. The condition register
	// read loses the comparison once the value crosses a block boundary, so it is
	// only the fallback.
	var flags core.ValueID
	if l.hasFlagsValue {
		flags = l.flagsValue
	} else {
		flags = l.readRegister(ins.address, "cr0", 32)
	}
	l.emit(core.OpIntrinsic, []core.Operand{
		symbolOperand(symbol),
		l.value(flags),
	}, []core.Value{condition}, nil, core.EffectReadRegister|core.EffectUnknown, ins.address)
	effects := core.EffectNone
	if len(targets) == 0 {
		effects = core.EffectUnknown
	}
	l.emit(core.OpBranch, []core.Operand{l.value(condition.ID)}, nil, targets, effects, ins.address)
}

// jump lifts B and BA.
func (l *lifter) jump(ins instruction) {
	target, ok := branchTarget(ins)
	if !ok {
		l.unsupported(ins)
		return
	}
	if block, ok := l.blockIDs[target]; ok {
		l.emit(core.OpJump, nil, nil, []core.BlockID{block}, core.EffectNone, ins.address)
		return
	}
	l.emit(core.OpJump, []core.Operand{pointerOperand(target, l.profile.Bits)}, nil, nil, core.EffectUnknown, ins.address)
}

func pointerOperand(value uint64, bits uint16) core.Operand {
	return core.Operand{
		Kind:    core.OperandConstant,
		Type:    core.ValueType{Kind: core.TypePointer, Bits: bits},
		Literal: literal(value, bits),
	}
}

// load lifts the integer and floating-point loads. The operand order is
// <rt>, <displacement>, <ra>.
func (l *lifter) load(ins instruction) {
	arguments := args(ins)
	if len(arguments) < 2 {
		l.unsupported(ins)
		return
	}
	destination, ok := l.destinationName(arguments[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	displacement := int64(0)
	baseIndex := 1
	if value, ok := signedImmediate(arguments[1]); ok {
		displacement = value
		baseIndex = 2
	}
	if len(arguments) <= baseIndex {
		l.unsupported(ins)
		return
	}
	baseArg := arguments[baseIndex]
	base, ok := l.destinationName(baseArg)
	if !ok {
		l.unsupported(ins)
		return
	}
	pointer := l.addressFromBase(ins, base, displacement)
	width := accessWidth(ins.inst.Op, l.profile)
	if hasUpdate(ins.inst.Op) {
		l.writeRegister(ins.address, base, l.profile.Bits, pointer)
	}
	if isFloatRegister(destination) || isVectorRegister(destination) {
		value := l.newValue(core.TypeFloat, width)
		l.emit(core.OpLoad, []core.Operand{l.value(pointer)}, []core.Value{value}, nil, core.EffectReadMemory, ins.address)
		l.emit(core.OpWriteRegister, []core.Operand{
			{Kind: core.OperandRegister, Type: value.Type, Name: destination},
			l.value(value.ID),
		}, nil, nil, core.EffectWriteRegister, ins.address)
		return
	}
	value := l.loadValue(ins.address, pointer, width)
	if width < l.profile.Bits {
		value = l.operation(ins, core.OpZeroExtend, []core.Operand{l.value(value)}, l.profile.Bits, core.EffectNone)
	}
	l.writeRegister(ins.address, destination, l.profile.Bits, value)
}

// store lifts the integer and floating-point stores.
func (l *lifter) store(ins instruction) {
	arguments := args(ins)
	if len(arguments) < 2 {
		l.unsupported(ins)
		return
	}
	source, ok := l.destinationName(arguments[0])
	if !ok {
		l.unsupported(ins)
		return
	}
	displacement := int64(0)
	baseIndex := 1
	if value, ok := signedImmediate(arguments[1]); ok {
		displacement = value
		baseIndex = 2
	}
	if len(arguments) <= baseIndex {
		l.unsupported(ins)
		return
	}
	base, ok := l.destinationName(arguments[baseIndex])
	if !ok {
		l.unsupported(ins)
		return
	}
	pointer := l.addressFromBase(ins, base, displacement)
	if hasUpdate(ins.inst.Op) {
		// A store-update form writes the computed address back into the base
		// register. Without the write-back the frame cannot be seen at all,
		// because the prologue allocates with stdu.
		l.writeRegister(ins.address, base, l.profile.Bits, pointer)
	}
	if isFloatRegister(source) || isVectorRegister(source) {
		value := l.newValue(core.TypeFloat, accessWidth(ins.inst.Op, l.profile))
		l.emit(core.OpReadRegister, []core.Operand{{
			Kind: core.OperandRegister,
			Type: value.Type,
			Name: source,
		}}, []core.Value{value}, nil, core.EffectReadRegister, ins.address)
		l.storeValue(ins.address, pointer, value.ID)
		return
	}
	value := l.readRegister(ins.address, source, l.profile.Bits)
	l.storeValue(ins.address, pointer, value)
}

// accessWidth returns the memory access width of one load or store.
func accessWidth(op ppc64asm.Op, profile Profile) uint16 {
	name := opName(op)
	switch {
	case len(name) >= 3 && name[:3] == "lbz", len(name) >= 3 && name[:3] == "stb":
		return 8
	case len(name) >= 3 && name[:3] == "lhz", len(name) >= 3 && name[:3] == "sth":
		return 16
	case len(name) >= 3 && name[:3] == "lwz", len(name) >= 3 && name[:3] == "stw":
		return 32
	case len(name) >= 2 && name[:2] == "lw", len(name) >= 2 && name[:2] == "st":
		if len(name) > 2 && name[2] == 'd' {
			return 64
		}
	}
	return profile.Bits
}

// system lifts synchronization, trap and cache-control instructions.
func (l *lifter) system(ins instruction) {
	name := opName(ins.inst.Op)
	effects := core.EffectUnknown
	switch name {
	case "sc", "trap", "tw", "td", "twi", "tdi":
		effects |= core.EffectMayTrap | core.EffectCall
	case "sync", "lwsync", "isync", "eieio":
		effects |= core.EffectReadMemory | core.EffectWriteMemory
	}
	l.emit(core.OpIntrinsic, []core.Operand{symbolOperand("ppc." + name)}, nil, nil, effects, ins.address)
}

// vector lifts one floating-point or vector instruction as an opaque register
// operation: the destination receives a value, the sources are read, and the
// lane semantics stay unmodeled.
func (l *lifter) vector(ins instruction) {
	operands := []core.Operand{symbolOperand("ppc." + opName(ins.inst.Op))}
	var results []core.Value
	effects := core.EffectNone
	for index, arg := range ins.inst.Args {
		if arg == nil {
			break
		}
		reg, ok := arg.(ppc64asm.Reg)
		if !ok {
			if operand, ok := l.rawOperand(ins, index, arg); ok {
				operands = append(operands, operand)
			}
			continue
		}
		name, _, ok := registerName(reg)
		if !ok {
			continue
		}
		if index == 0 {
			value := l.newValue(core.TypeFloat, l.profile.Bits)
			results = append(results, value)
			operands = append(operands, core.Operand{Kind: core.OperandRegister, Type: value.Type, Name: name})
			effects |= core.EffectWriteRegister
			continue
		}
		operands = append(operands, core.Operand{
			Kind: core.OperandRegister,
			Type: core.ValueType{Kind: core.TypeFloat, Bits: l.profile.Bits},
			Name: name,
		})
		effects |= core.EffectReadRegister
	}
	l.emit(core.OpIntrinsic, operands, results, nil, effects|core.EffectUnknown, ins.address)
}

// specialRegister lifts the SPR file accesses. The SPR numbers are
// architectural: 1 is XER, 8 is LR, 9 is CTR, and reads and writes are
// explicit so the data flow stays visible.
func (l *lifter) specialRegister(ins instruction, name string) (core.ValueID, bool) {
	arguments := args(ins)
	bits := l.profile.Bits
	switch name {
	case "mfspr", "mtspr":
		if len(arguments) != 2 {
			return 0, false
		}
		if name == "mfspr" {
			destination, ok := l.destinationName(arguments[0])
			if !ok {
				return 0, false
			}
			spr, ok := argumentSprNumber(arguments[1])
			if !ok {
				return 0, false
			}
			if l.hasPushedPC && sprName(spr) == "lr" {
				// The branch just before this read pushed the address of the
				// instruction after it; the link register holds that constant.
				l.hasPushedPC = false
				value := l.pointerConstant(ins.address, l.pushedPC)
				l.writeRegister(ins.address, destination, bits, value)
				return value, true
			}
			value := l.readRegister(ins.address, sprName(spr), bits)
			l.writeRegister(ins.address, destination, bits, value)
			return value, true
		}
		spr, ok := argumentSprNumber(arguments[0])
		if !ok {
			return 0, false
		}
		value, ok := l.readOperand(ins, arguments[1])
		if !ok {
			return 0, false
		}
		l.writeRegister(ins.address, sprName(spr), bits, value)
		return value, true
	case "mfcr":
		if len(arguments) != 1 {
			return 0, false
		}
		destination, ok := l.destinationName(arguments[0])
		if !ok {
			return 0, false
		}
		value := l.readRegister(ins.address, "cr", 32)
		l.writeRegister(ins.address, destination, bits, value)
		return value, true
	case "mtcrf":
		if len(arguments) < 2 {
			return 0, false
		}
		value, ok := l.readOperand(ins, arguments[1])
		if !ok {
			return 0, false
		}
		l.writeRegister(ins.address, "cr", 32, value)
		return value, true
	case "mflr":
		value := l.readRegister(ins.address, "lr", bits)
		if len(arguments) > 0 {
			if destination, ok := l.destinationName(arguments[0]); ok {
				l.writeRegister(ins.address, destination, bits, value)
			}
		}
		return value, true
	case "mtlr":
		if len(arguments) > 0 {
			if value, ok := l.readOperand(ins, arguments[0]); ok {
				l.writeRegister(ins.address, "lr", bits, value)
				return value, true
			}
		}
		return 0, false
	}
	// The remaining special registers are opaque.
	operands := []core.Operand{symbolOperand("ppc." + name)}
	for _, arg := range arguments {
		if operand, ok := l.rawOperand(ins, 0, arg); ok {
			operands = append(operands, operand)
		}
	}
	l.emit(core.OpIntrinsic, operands, nil, nil, core.EffectUnknown, ins.address)
	return 0, true
}

// argumentSprNumber extracts the SPR number from one operand.
func argumentSprNumber(arg ppc64asm.Arg) (int, bool) {
	value, ok := arg.(ppc64asm.SpReg)
	if !ok {
		return 0, false
	}
	return spprNumber(value), true
}

// spprNumber reads the SPR number from its printed form.
func spprNumber(value ppc64asm.SpReg) int {
	text := value.String()
	number := 0
	for _, current := range text {
		if current < '0' || current > '9' {
			continue
		}
		number = number*10 + int(current-'0')
	}
	return number
}

// sprName maps the architectural SPR numbers to register names.
func sprName(number int) string {
	switch number {
	case 1:
		return "xer"
	case 8:
		return "lr"
	case 9:
		return "ctr"
	default:
		return fmt.Sprintf("spr%d", number)
	}
}

// rotateMask lifts the rotate-and-mask family. The mask is a bit range, so the
// operation keeps its positions and stays an intrinsic.
func (l *lifter) rotateMask(ins instruction, name string) (core.ValueID, bool) {
	arguments := args(ins)
	if len(arguments) < 3 {
		return 0, false
	}
	destination, ok := l.destinationName(arguments[0])
	if !ok {
		return 0, false
	}
	source, ok := l.readOperand(ins, arguments[1])
	if !ok {
		return 0, false
	}
	bits := l.profile.Bits
	operands := []core.Operand{symbolOperand("ppc." + name), l.value(source)}
	for _, arg := range arguments[2:] {
		if value, ok := signedImmediate(arg); ok {
			operands = append(operands, l.value(l.constant(ins.address, uint64(value), 8)))
			continue
		}
		if value, ok := l.readOperand(ins, arg); ok {
			operands = append(operands, l.value(value))
		}
	}
	result := l.operation(ins, core.OpIntrinsic, operands, bits, core.EffectNone)
	l.writeRegister(ins.address, destination, bits, result)
	return result, true
}

// selectInstruction lifts ISEL: it chooses between two registers on a condition.
func (l *lifter) selectInstruction(ins instruction) (core.ValueID, bool) {
	arguments := args(ins)
	if len(arguments) != 4 {
		return 0, false
	}
	destination, ok := l.destinationName(arguments[0])
	if !ok {
		return 0, false
	}
	whenTrue, ok := l.readOperand(ins, arguments[1])
	if !ok {
		return 0, false
	}
	whenFalse, ok := l.readOperand(ins, arguments[2])
	if !ok {
		return 0, false
	}
	code, ok := arguments[3].(ppc64asm.CondReg)
	if !ok {
		return 0, false
	}
	condition := l.conditionValue(ins, code)
	bits := l.profile.Bits
	result := l.operation(ins, core.OpSelect, []core.Operand{
		l.value(condition), l.value(whenTrue), l.value(whenFalse),
	}, bits, core.EffectNone)
	l.writeRegister(ins.address, destination, bits, result)
	return result, true
}

// conditionValue materializes one CR field condition.
func (l *lifter) conditionValue(ins instruction, code ppc64asm.CondReg) core.ValueID {
	cr := l.readRegister(ins.address, "cr", 32)
	return l.operation(ins, core.OpIntrinsic, []core.Operand{
		symbolOperand("cond." + strings.ToLower(code.String())), l.value(cr),
	}, 1, core.EffectReadRegister)
}

// hasUpdate reports whether one load or store writes the computed address back
// into its base register. PowerPC marks the immediate form with a trailing "u".
func hasUpdate(op ppc64asm.Op) bool {
	name := opName(op)
	return len(name) > 2 && name[len(name)-1] == 'u' &&
		(strings.HasPrefix(name, "st") || strings.HasPrefix(name, "l"))
}

// branchConditionCode returns the comparison a branch tests, as the neutral
// condition code the SSA layer understands. The branch operand names the bit it
// tests and whether it branches on the bit or against it, so the mnemonic alias
// is not needed.
func branchConditionCode(ins instruction) (string, bool) {
	arguments := args(ins)
	if len(arguments) < 2 {
		return "", false
	}
	options, optionsOK := arguments[0].(ppc64asm.Imm)
	field, fieldOK := arguments[1].(ppc64asm.CondReg)
	if !optionsOK || !fieldOK {
		return "", false
	}
	// An options word without bit three takes the branch when the tested bit is
	// clear; with the bit, when it is set.
	if uint32(options)&0x10 != 0 && uint32(options)&0x08 == 0 {
		return "", false
	}
	negated := uint32(options)&0x08 == 0
	switch (int(field) - 1) % 4 {
	case 2:
		if negated {
			return "ne", true
		}
		return "eq", true
	case 0:
		if negated {
			return "ge", true
		}
		return "lt", true
	case 1:
		if negated {
			return "le", true
		}
		return "gt", true
	}
	return "", false
}

// unconditionalBranch reports whether a branch takes its target without reading a
// condition. The options word names the shapes; the plain mnemonics are the common
// spelling.
func unconditionalBranch(ins instruction, name string) bool {
	if name == "b" || name == "ba" {
		return true
	}
	arguments := args(ins)
	if len(arguments) == 0 {
		return false
	}
	options, ok := arguments[0].(ppc64asm.Imm)
	if !ok {
		return false
	}
	switch uint32(options) {
	case 18, 20:
		return true
	}
	return false
}
