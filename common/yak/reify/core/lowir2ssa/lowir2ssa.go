// Package lowir2ssa converts Reify LowIR into a YakSSA program.
//
// Pure values become typed SSA values. General-purpose registers and memory
// slots become typed variables: a read uses the current value, a write
// redefines it, and no Phi is inserted. Only a LowIR call instruction produces
// an SSA Call. Intrinsics and unknown values become opaque typed values.
package lowir2ssa

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowir2ssa/pass"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssairtext"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// Result is one converted YakSSA program.
type Result struct {
	Program     *ssa.Program
	Diagnostics []core.Diagnostic
}

// Options controls conversion and LowIR normalization.
type Options struct {
	ProgramName string
	LowIRPasses []string
}

// Option customizes conversion.
type Option func(*Options)

// WithLowIRPasses selects registered LowIR passes, in order.
func WithLowIRPasses(names ...string) Option {
	return func(options *Options) {
		options.LowIRPasses = append([]string(nil), names...)
	}
}

// WithProgramName sets the generated YakSSA program name.
func WithProgramName(name string) Option {
	return func(options *Options) {
		options.ProgramName = name
	}
}

// Convert converts LowIR into a YakSSA program. It does not execute the input.
func Convert(ctx context.Context, ir *core.LowIR, options ...Option) (*Result, error) {
	if ir == nil {
		return nil, fmt.Errorf("lowir2ssa: nil LowIR")
	}
	if len(ir.Functions) == 0 {
		return nil, fmt.Errorf("lowir2ssa: LowIR has no functions")
	}
	settings := Options{ProgramName: "reify-lowir"}
	for _, option := range options {
		option(&settings)
	}
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(settings.ProgramName))
	if err != nil {
		return nil, fmt.Errorf("lowir2ssa: invalid program config: %w", err)
	}
	program := ssa.NewProgram(cfg, ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	converter := &converter{
		ctx:         ctx,
		program:     program,
		diagnostics: make([]core.Diagnostic, 0),
	}
	for index := range ir.Functions {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		function := ir.Functions[index]
		names := settings.LowIRPasses
		if len(names) == 0 {
			names = pass.DefaultNames()
		}
		if err := pass.Apply(&function, names...); err != nil {
			return nil, err
		}
		function.FrameCleanup = containsPass(names, "remove-stackframe")
		function.FrameKept = !function.FrameCleanup
		if err := converter.convertFunction(function); err != nil {
			return nil, err
		}
	}
	return &Result{Program: program, Diagnostics: converter.diagnostics}, nil
}

// Text renders the converted program with YakSSA's disassembler. Extern call
// targets and conversion scaffolding are omitted.
func Text(program *ssa.Program) string {
	if program == nil {
		return ""
	}
	return ssairtext.FormatProgram(program)
}

type converter struct {
	ctx         context.Context
	program     *ssa.Program
	diagnostics []core.Diagnostic
	function    *ssa.Function
	builder     *ssa.FunctionBuilder
	entry       *ssa.BasicBlock
	blocks      map[core.BlockID]*ssa.BasicBlock
	values      map[core.ValueID]ssa.Value
	defs        map[core.ValueID]core.Instruction
	variables   map[string]*ssa.Variable
	memory      map[string]*ssa.Variable
	memoryNames map[string]string
	memoryIndex int
	functions   map[string]*ssa.Function
	flags       map[core.ValueID]flagInfo
	arguments   map[string]bool
	loopHeaders map[core.BlockID]bool
	loopPhis    map[core.BlockID]map[string]*ssa.Phi
	loopPhiDone map[int64]bool
	headerStore map[core.BlockID]map[string]bool
	loopCarried map[core.BlockID]map[string]bool
	current     core.BlockID
	types       map[core.ValueType]ssa.Type
	undefined   map[string]ssa.Value
}

func (c *converter) convertFunction(function core.Function) error {
	name := functionName(function)
	ssaFunction := c.program.NewFunction(name)
	metadata := ""
	if function.Boundary != "" {
		metadata = "boundary: " + string(function.Boundary)
	}
	if function.FrameRemoved || function.FrameCleanup {
		if metadata != "" {
			metadata += ";"
		}
		metadata += "frame-cleanup"
	}
	if function.FrameKept {
		if metadata != "" {
			metadata += ";"
		}
		metadata += "frame-kept"
	}
	if metadata != "" {
		ssaFunction.SetVerboseName(metadata)
	}
	builder := ssaFunction.GetOrCreateBuilder()
	if builder == nil {
		return fmt.Errorf("lowir2ssa: cannot create builder for %s", name)
	}
	c.function = ssaFunction
	c.builder = builder
	c.entry = builder.CurrentBlock
	c.blocks = make(map[core.BlockID]*ssa.BasicBlock, len(function.Blocks))
	c.values = make(map[core.ValueID]ssa.Value)
	c.defs = make(map[core.ValueID]core.Instruction)
	c.variables = make(map[string]*ssa.Variable)
	c.memory = make(map[string]*ssa.Variable)
	c.memoryNames = make(map[string]string)
	c.functions = make(map[string]*ssa.Function)
	c.flags = make(map[core.ValueID]flagInfo)
	c.arguments = make(map[string]bool)
	c.loopHeaders = make(map[core.BlockID]bool)
	c.loopPhis = make(map[core.BlockID]map[string]*ssa.Phi)
	c.loopPhiDone = make(map[int64]bool)
	c.headerStore = make(map[core.BlockID]map[string]bool)
	c.loopCarried = make(map[core.BlockID]map[string]bool)
	c.types = make(map[core.ValueType]ssa.Type)
	c.undefined = make(map[string]ssa.Value)

	for index, block := range function.Blocks {
		var ssaBlock *ssa.BasicBlock
		if index == 0 {
			ssaBlock = c.entry
		} else {
			ssaBlock = ssaFunction.NewBasicBlock(fmt.Sprintf("b%d", block.ID))
			ssaBlock.SetScope(ssa.NewScope(ssaFunction, c.program.GetProgramName()))
		}
		if ssaBlock == nil {
			return fmt.Errorf("lowir2ssa: cannot create block b%d", block.ID)
		}
		c.blocks[block.ID] = ssaBlock
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instructions {
			for _, result := range instruction.Results {
				c.defs[result.ID] = instruction
			}
		}
	}
	c.markLoopHeaders(function)
	c.collectLoopCarried(function)
	c.bindParameters()
	for _, block := range function.Blocks {
		if err := c.convertBlock(block); err != nil {
			return err
		}
	}
	ssaFunction.Finish()
	return nil
}

func (c *converter) markLoopHeaders(function core.Function) {
	order := make(map[core.BlockID]int, len(function.Blocks))
	for index, block := range function.Blocks {
		order[block.ID] = index
	}
	for index, block := range function.Blocks {
		for _, instruction := range block.Instructions {
			if instruction.Kind != core.OpJump && instruction.Kind != core.OpBranch {
				continue
			}
			for _, target := range instruction.Targets {
				if position, ok := order[target]; ok && position <= index {
					c.loopHeaders[target] = true
				}
			}
		}
	}
}

func (c *converter) collectLoopCarried(function core.Function) {
	order := make(map[core.BlockID]int, len(function.Blocks))
	for index, block := range function.Blocks {
		order[block.ID] = index
	}
	for header, position := range order {
		if !c.loopHeaders[header] {
			continue
		}
		last := position
		for index, block := range function.Blocks {
			if index <= position {
				continue
			}
			for _, instruction := range block.Instructions {
				for _, target := range instruction.Targets {
					if target == header && index > last {
						last = index
					}
				}
			}
		}
		if last <= position {
			continue
		}
		loaded := make(map[string]bool)
		stored := make(map[string]bool)
		for index := position; index <= last; index++ {
			for _, instruction := range function.Blocks[index].Instructions {
				switch instruction.Kind {
				case core.OpLoad:
					if len(instruction.Operands) > 0 {
						loaded[valueKey(c.defs, instruction.Operands[0].Value)] = true
					}
				case core.OpStore:
					if len(instruction.Operands) > 0 {
						stored[valueKey(c.defs, instruction.Operands[0].Value)] = true
					}
				}
			}
		}
		keys := make(map[string]bool)
		for key := range loaded {
			if stored[key] {
				keys[key] = true
			}
		}
		if len(keys) > 0 {
			c.loopCarried[header] = keys
		}
	}
}

func (c *converter) prepareLoopPhis(blockID core.BlockID) {
	for key := range c.loopCarried[blockID] {
		c.loopPhi("mem:"+key, c.memoryVariable(c.memoryName(key)))
	}
}

func (c *converter) bindParameters() {
	c.builder.CurrentBlock = c.entry
	for _, name := range []string{"rsp", "rbp"} {
		parameter := ssa.NewParam("entry_"+name, false, c.builder)
		if parameter == nil {
			continue
		}
		parameter.SetType(c.typeFor(integerType(64)))
		c.assignRegister(name, parameter)
	}
	for index, name := range []string{"rdi", "rsi", "rdx", "rcx", "r8", "r9"} {
		parameter := ssa.NewParam(fmt.Sprintf("arg%d", index), false, c.builder)
		if parameter == nil {
			continue
		}
		parameter.FormalParameterIndex = index
		c.function.Params = append(c.function.Params, parameter.GetId())
		c.assignRegister(name, parameter)
	}
}

func (c *converter) convertBlock(block core.Block) error {
	ssaBlock, ok := c.blocks[block.ID]
	if !ok {
		return fmt.Errorf("lowir2ssa: block b%d is missing", block.ID)
	}
	c.current = block.ID
	c.builder.CurrentBlock = ssaBlock
	c.prepareLoopPhis(block.ID)
	for _, instruction := range block.Instructions {
		if err := c.ctx.Err(); err != nil {
			return err
		}
		c.convertInstruction(instruction)
	}
	c.fillLoopCarried(block)
	// A malformed or truncated LowIR block may have no terminator. Preserve a
	// complete CFG by closing it with an empty return; terminated blocks ignore
	// this call.
	c.builder.EmitReturn(nil)
	return nil
}

func (c *converter) convertInstruction(instruction core.Instruction) {
	switch instruction.Kind {
	case core.OpConstant:
		c.setResult(instruction, c.constValue(firstOperand(instruction.Operands)))
	case core.OpCopy:
		value := c.operandValue(firstOperand(instruction.Operands))
		if value == nil {
			c.setResult(instruction, c.opaqueValue("copy", instruction))
			return
		}
		c.setResult(instruction, c.coerceType(value, instruction.Results))
	case core.OpPhi:
		c.addDiagnostic("lowir2ssa-unexpected-phi", "LowIR Phi was folded into variable assignment")
		values := c.operandValues(instruction.Operands)
		if len(values) > 0 {
			c.setResult(instruction, values[0])
		}
	case core.OpSelect:
		c.setResult(instruction, c.opaqueValue("select", instruction))
	case core.OpAdd:
		c.emitBinary(instruction, ssa.OpAdd)
	case core.OpSub:
		c.emitBinary(instruction, ssa.OpSub)
	case core.OpMul:
		c.emitBinary(instruction, ssa.OpMul)
	case core.OpUDiv, core.OpSDiv:
		c.emitBinary(instruction, ssa.OpDiv)
	case core.OpAnd:
		c.emitBinary(instruction, ssa.OpAnd)
	case core.OpOr:
		c.emitBinary(instruction, ssa.OpOr)
	case core.OpXor:
		c.emitBinary(instruction, ssa.OpXor)
	case core.OpShl:
		c.emitBinary(instruction, ssa.OpShl)
	case core.OpLShr, core.OpAShr:
		c.emitBinary(instruction, ssa.OpShr)
	case core.OpEqual:
		c.emitBinary(instruction, ssa.OpEq)
	case core.OpULess, core.OpSLess:
		c.emitBinary(instruction, ssa.OpLt)
	case core.OpNeg:
		c.emitUnary(instruction, ssa.OpNeg)
	case core.OpNot:
		c.emitUnary(instruction, ssa.OpBitwiseNot)
	case core.OpReadRegister:
		c.emitRegisterRead(instruction)
	case core.OpWriteRegister:
		c.emitRegisterWrite(instruction)
	case core.OpLoad:
		c.emitLoad(instruction)
	case core.OpStore:
		c.emitStore(instruction)
	case core.OpCall:
		c.emitCall(instruction)
	case core.OpIntrinsic:
		c.emitIntrinsic(instruction)
	case core.OpZeroExtend, core.OpSignExtend, core.OpTruncate:
		c.emitWidthCast(instruction)
	case core.OpJump:
		c.emitJump(instruction)
	case core.OpBranch:
		c.emitBranch(instruction)
	case core.OpReturn:
		values := c.operandValues(instruction.Operands)
		if len(values) == 0 {
			if value := c.currentRegister("rax"); value != nil {
				values = append(values, value)
			}
		}
		c.builder.EmitReturn(values)
	case core.OpTrap:
		c.emitTrap(instruction)
	case core.OpNop:
		// NOP has no SSA effect.
	default:
		c.addDiagnostic("lowir2ssa-unsupported-op", fmt.Sprintf("operation %q was preserved as an opaque value", instruction.Kind))
		c.setResult(instruction, c.opaqueValue(string(instruction.Kind), instruction))
	}
}

func (c *converter) emitBinary(instruction core.Instruction, op ssa.BinaryOpcode) {
	values := c.operandValues(instruction.Operands)
	if len(values) < 2 {
		c.addDiagnostic("lowir2ssa-missing-operand", fmt.Sprintf("operation %q at 0x%x has %d operands", instruction.Kind, instruction.Address, len(values)))
		c.setResult(instruction, c.opaqueValue(string(instruction.Kind), instruction))
		return
	}
	c.setResult(instruction, c.builder.EmitBinOp(op, values[0], values[1]))
}

func (c *converter) emitUnary(instruction core.Instruction, op ssa.UnaryOpcode) {
	values := c.operandValues(instruction.Operands)
	if len(values) == 0 {
		c.addDiagnostic("lowir2ssa-missing-operand", fmt.Sprintf("operation %q at 0x%x has no operand", instruction.Kind, instruction.Address))
		c.setResult(instruction, c.opaqueValue(string(instruction.Kind), instruction))
		return
	}
	c.setResult(instruction, c.builder.EmitUnOp(op, values[0]))
}

func (c *converter) emitRegisterRead(instruction core.Instruction) {
	operand := firstOperand(instruction.Operands)
	view := registerViewFor(operand.Name, operand.Type.Bits)
	if view.canonical == "" {
		c.setResult(instruction, c.opaqueValue("read_register", instruction))
		return
	}
	if c.loopHeaders[c.current] {
		key := "reg:" + view.canonical
		if phi := c.loopPhi(key, c.registerVariable(view.canonical)); phi != nil {
			c.setResult(instruction, phi)
			return
		}
	}
	value := c.registerValue(view)
	if value == nil {
		c.setResult(instruction, c.opaqueValue("read_register", instruction))
		return
	}
	if view.narrow() {
		value = c.extractRegister(value, view)
	}
	c.setResult(instruction, value)
}

func (c *converter) emitRegisterWrite(instruction core.Instruction) {
	if len(instruction.Operands) < 2 {
		c.addDiagnostic("lowir2ssa-missing-operand", fmt.Sprintf("write_register at 0x%x has no value", instruction.Address))
		return
	}
	operand := instruction.Operands[0]
	value := c.operandValue(instruction.Operands[1])
	if value == nil {
		return
	}
	view := registerViewFor(operand.Name, operand.Type.Bits)
	if view.canonical == "" {
		return
	}
	value = c.widenRegister(value, view)
	if view.partial() {
		current := c.registerValue(view)
		if current == nil {
			current = c.undefinedValue(view.canonical)
		}
		mask := c.constantLiteral(view.mask())
		cleared := c.typedBinOp(ssa.OpAndNot, current, mask, integerType(64))
		inserted := value
		if view.shift > 0 {
			inserted = c.typedBinOp(ssa.OpShl, inserted, c.constantLiteral(uint64(view.shift)), integerType(64))
		}
		inserted = c.typedBinOp(ssa.OpAnd, inserted, mask, integerType(64))
		value = c.typedBinOp(ssa.OpOr, cleared, inserted, integerType(64))
	}
	if value.IsUndefined() {
		// A variable that holds a placeholder deletes it on the next real write,
		// which would leave earlier users with a dangling value.
		return
	}
	variable := c.registerVariable(view.canonical)
	if variable == nil {
		return
	}
	if err := variable.Assign(value); err != nil {
		c.addDiagnostic("lowir2ssa-assignment", fmt.Sprintf("cannot assign %s: %v", view.canonical, err))
	}
	c.markHeaderStore("reg:" + view.canonical)
	if argumentRegister(view.canonical) {
		c.arguments[view.canonical] = true
	}
}

func (c *converter) extractRegister(value ssa.Value, view registerView) ssa.Value {
	if view.shift > 0 {
		value = c.typedBinOp(ssa.OpShr, value, c.constantLiteral(uint64(view.shift)), integerType(64))
	}
	if view.width > 0 && view.width < view.canonicalBits {
		return c.castType(value, c.typeFor(integerType(view.width)))
	}
	return value
}

// widenRegister types a 32-bit write as a 64-bit value. x86 zero-extends such a
// write, so the cast is explicit rather than a silent retype.
func (c *converter) widenRegister(value ssa.Value, view registerView) ssa.Value {
	if view.canonicalBits == 0 || view.width == 0 || view.width >= view.canonicalBits {
		return value
	}
	if view.width == 32 && view.shift == 0 {
		return c.castType(value, c.typeFor(integerType(64)))
	}
	return value
}

func (c *converter) registerVariable(name string) *ssa.Variable {
	if variable, ok := c.variables[name]; ok {
		return variable
	}
	current := c.builder.CurrentBlock
	c.builder.CurrentBlock = c.entry
	variable := c.builder.CreateVariable(name)
	c.builder.CurrentBlock = current
	if variable == nil {
		return nil
	}
	c.variables[name] = variable
	return variable
}

func (c *converter) currentRegister(name string) ssa.Value {
	variable := c.registerVariable(name)
	if variable == nil {
		return nil
	}
	return variable.GetValue()
}

func (c *converter) assignRegister(name string, value ssa.Value) {
	if value == nil || value.IsUndefined() {
		return
	}
	view := registerViewFor(name, 64)
	variable := c.registerVariable(view.canonical)
	if variable == nil {
		return
	}
	if err := variable.Assign(value); err != nil {
		c.addDiagnostic("lowir2ssa-assignment", fmt.Sprintf("cannot assign %s: %v", view.canonical, err))
	}
}

func (c *converter) registerValue(view registerView) ssa.Value {
	variable := c.registerVariable(view.canonical)
	if variable == nil {
		return nil
	}
	if value := variable.GetValue(); value != nil {
		return value
	}
	// Do not assign the placeholder to the variable. YakSSA deletes a value when
	// a variable replaces an undefined value, and earlier reads may still refer
	// to that placeholder.
	return c.undefinedValue(view.canonical)
}

func (c *converter) undefinedValue(name string) ssa.Value {
	if value, ok := c.undefined[name]; ok {
		return value
	}
	current := c.builder.CurrentBlock
	c.builder.CurrentBlock = c.entry
	value := c.builder.EmitUndefined(name)
	c.builder.CurrentBlock = current
	if value == nil {
		c.addDiagnostic("lowir2ssa-missing-target", "could not create an undefined value for "+name)
		return nil
	}
	c.undefined[name] = value
	return value
}

func (c *converter) typedBinOp(op ssa.BinaryOpcode, left, right ssa.Value, valueType core.ValueType) ssa.Value {
	value := c.builder.EmitBinOp(op, left, right)
	if value != nil {
		value.SetType(c.typeFor(valueType))
	}
	return value
}

func (c *converter) castType(value ssa.Value, typ ssa.Type) ssa.Value {
	if value == nil || typ == nil {
		return value
	}
	if value.GetType() != nil && value.GetType().String() == typ.String() {
		return value
	}
	cast := c.builder.EmitTypeCast(value, typ)
	if cast == nil {
		return value
	}
	return cast
}

func (c *converter) constantLiteral(value uint64) ssa.Value {
	if value <= math.MaxInt64 {
		return c.newConstant(int64(value), c.typeFor(integerType(64)))
	}
	return c.newConstant(value, c.typeFor(integerType(64)))
}

func (c *converter) newConstant(value any, typ ssa.Type) *ssa.ConstInst {
	constant := c.builder.EmitConstInst(value)
	if typ != nil {
		constant.SetType(typ)
	}
	return constant
}

func (c *converter) emitCall(instruction core.Instruction) {
	if len(instruction.Operands) == 0 {
		c.addDiagnostic("lowir2ssa-missing-operand", fmt.Sprintf("call at 0x%x has no target", instruction.Address))
		return
	}
	var target ssa.Value
	operand := instruction.Operands[0]
	switch operand.Kind {
	case core.OperandConstant:
		target = c.externFunction(fmt.Sprintf("sub_0x%x", literalUint64(operand.Literal)))
	case core.OperandSymbol:
		target = c.externFunction(operand.Name)
	default:
		target = c.operandValue(operand)
	}
	if target == nil {
		return
	}
	args := c.operandValues(instruction.Operands[1:])
	args = append(args, c.captureArguments()...)
	call := c.builder.EmitCall(c.builder.NewCall(target, args))
	if instruction.Variadic && call != nil {
		call.SetVerboseName("variadic")
	}
	c.setResult(instruction, call)
	// System V AMD64 returns integer and pointer values in rax.
	c.assignRegister("rax", call)
}

// captureArguments reads the System V AMD64 integer argument registers that
// were written since the previous call. Registers above the highest written
// slot are omitted; missing lower slots become undefined arguments.
func (c *converter) captureArguments() []ssa.Value {
	names := []string{"rdi", "rsi", "rdx", "rcx", "r8", "r9"}
	last := -1
	for index, name := range names {
		if c.arguments[name] {
			last = index
		}
	}
	if last < 0 {
		return nil
	}
	args := make([]ssa.Value, 0, last+1)
	for index := 0; index <= last; index++ {
		name := names[index]
		args = append(args, c.registerValue(registerViewFor(name, 64)))
		c.arguments[name] = false
	}
	return args
}

func argumentRegister(name string) bool {
	switch name {
	case "rdi", "rsi", "rdx", "rcx", "r8", "r9":
		return true
	default:
		return false
	}
}

// externFunction supplies a target for a direct call. The stub has no body and
// is skipped by text renderers; the call instruction itself is a real Call.
func (c *converter) externFunction(name string) ssa.Value {
	if name == "" {
		name = "unknown_call"
	}
	if function, ok := c.functions[name]; ok {
		return function
	}
	if existing, ok := c.program.Funcs.Get(name); ok {
		c.functions[name] = existing
		return existing
	}
	function := c.program.NewFunction(name)
	function.SetExtern(true)
	c.functions[name] = function
	return function
}

type flagInfo struct {
	name     string
	operands []core.ValueID
}

func (c *converter) emitIntrinsic(instruction core.Instruction) {
	if len(instruction.Results) == 0 {
		return
	}
	name := "intrinsic"
	if len(instruction.Operands) > 0 && instruction.Operands[0].Kind == core.OperandSymbol {
		name = instruction.Operands[0].Name
	}
	if strings.HasPrefix(name, "x86.flags.") {
		info := flagInfo{name: strings.TrimPrefix(name, "x86.flags.")}
		for _, operand := range instruction.Operands[1:] {
			if operand.Kind == core.OperandValue {
				info.operands = append(info.operands, operand.Value)
			}
		}
		c.flags[instruction.Results[0].ID] = info
		marker := c.opaqueValue(name, instruction)
		c.setResult(instruction, marker)
		if marker != nil {
			c.flags[core.ValueID(marker.GetId())] = info
		}
		return
	}
	if strings.HasPrefix(name, "x86.cond.") {
		if condition := c.conditionValue(name, instruction); condition != nil {
			c.setResult(instruction, condition)
			return
		}
	}
	c.setResult(instruction, c.opaqueValue(name, instruction))
}

func (c *converter) conditionValue(symbol string, instruction core.Instruction) ssa.Value {
	cc := strings.TrimPrefix(symbol, "x86.cond.")
	if cc == "" || len(instruction.Operands) < 2 || instruction.Operands[1].Kind != core.OperandValue {
		return nil
	}
	flags := c.operandValue(instruction.Operands[1])
	if flags == nil {
		return nil
	}
	info, ok := c.flags[core.ValueID(flags.GetId())]
	if !ok {
		return nil
	}
	switch info.name {
	case "cmp":
		if len(info.operands) < 2 {
			return nil
		}
		return c.compareCondition(cc, info.operands[0], info.operands[1])
	case "test":
		if len(info.operands) < 2 {
			return nil
		}
		left := c.values[info.operands[0]]
		right := c.values[info.operands[1]]
		if left == nil || right == nil {
			return nil
		}
		subject := c.builder.EmitBinOp(ssa.OpAnd, left, right)
		return c.zeroCondition(cc, subject)
	default:
		if len(info.operands) == 0 {
			return nil
		}
		subject := c.values[info.operands[len(info.operands)-1]]
		if subject == nil {
			return nil
		}
		return c.zeroCondition(cc, subject)
	}
}

func (c *converter) compareCondition(cc string, leftID, rightID core.ValueID) ssa.Value {
	left := c.values[leftID]
	right := c.values[rightID]
	if left == nil || right == nil {
		return nil
	}
	var op ssa.BinaryOpcode
	switch cc {
	case "e", "z":
		op = ssa.OpEq
	case "ne", "nz":
		op = ssa.OpNotEq
	case "l", "b", "c":
		op = ssa.OpLt
	case "le", "be":
		op = ssa.OpLtEq
	case "g", "a":
		op = ssa.OpGt
	case "ge", "ae":
		op = ssa.OpGtEq
	default:
		return nil
	}
	return c.builder.EmitBinOp(op, left, right)
}

func (c *converter) zeroCondition(cc string, subject ssa.Value) ssa.Value {
	if subject == nil {
		return nil
	}
	zero := c.newConstant(int64(0), c.typeFor(integerType(64)))
	switch cc {
	case "e", "z":
		return c.builder.EmitBinOp(ssa.OpEq, subject, zero)
	case "ne", "nz":
		return c.builder.EmitBinOp(ssa.OpNotEq, subject, zero)
	case "l", "b", "c":
		return c.builder.EmitBinOp(ssa.OpLt, subject, zero)
	case "le", "be":
		return c.builder.EmitBinOp(ssa.OpLtEq, subject, zero)
	case "g", "a":
		return c.builder.EmitBinOp(ssa.OpGt, subject, zero)
	case "ge", "ae":
		return c.builder.EmitBinOp(ssa.OpGtEq, subject, zero)
	case "s":
		return c.builder.EmitBinOp(ssa.OpLt, subject, zero)
	case "ns":
		less := c.builder.EmitBinOp(ssa.OpLt, subject, zero)
		return c.builder.EmitUnOp(ssa.OpNot, less)
	default:
		return nil
	}
}

// opaqueValue represents a value whose exact semantics are not modeled. It is a
// tagged constant cast to the result type, not a call, so the marker stays
// visible in both YakSSA and source text.
func (c *converter) opaqueValue(name string, instruction core.Instruction) ssa.Value {
	if name == "" {
		name = "opaque"
	}
	marker := c.newConstant(name, ssa.CreateStringType())
	if len(instruction.Results) > 0 {
		return c.castType(marker, c.typeFor(instruction.Results[0].Type))
	}
	return marker
}

func (c *converter) emitWidthCast(instruction core.Instruction) {
	value := c.operandValue(firstOperand(instruction.Operands))
	if value == nil || len(instruction.Results) == 0 {
		return
	}
	c.setResult(instruction, c.castType(value, c.typeFor(instruction.Results[0].Type)))
}

func (c *converter) emitTrap(instruction core.Instruction) {
	message := "trap"
	if len(instruction.Operands) > 0 && instruction.Operands[0].Kind == core.OperandSymbol {
		message = instruction.Operands[0].Name
	}
	c.builder.EmitPanic(c.builder.EmitConstInst(message))
	c.builder.EmitReturn(nil)
}

func (c *converter) emitLoad(instruction core.Instruction) {
	if len(instruction.Operands) == 0 {
		return
	}
	key := valueKey(c.defs, instruction.Operands[0].Value)
	name := c.memoryName(key)
	variable := c.memoryVariable(name)
	if variable == nil {
		return
	}
	if c.loopHeaders[c.current] && c.loopCarried[c.current][key] {
		prefixed := "mem:" + key
		if phi := c.loopPhi(prefixed, variable); phi != nil {
			c.setResult(instruction, phi)
			return
		}
	}
	if value := variable.GetValue(); value != nil {
		c.setResult(instruction, value)
		return
	}
	c.setResult(instruction, c.loadMarker(instruction, instruction.Operands[0].Value))
}

func (c *converter) emitStore(instruction core.Instruction) {
	if len(instruction.Operands) < 2 {
		return
	}
	key := valueKey(c.defs, instruction.Operands[0].Value)
	variable := c.memoryVariable(c.memoryName(key))
	if variable == nil {
		return
	}
	value := c.operandValue(instruction.Operands[1])
	if value == nil || value.IsUndefined() {
		return
	}
	if err := variable.Assign(value); err != nil {
		c.addDiagnostic("lowir2ssa-assignment", fmt.Sprintf("cannot assign %s: %v", variable.GetName(), err))
	}
	c.markHeaderStore("mem:" + key)
}

func (c *converter) loadMarker(instruction core.Instruction, addressID core.ValueID) ssa.Value {
	address := c.values[addressID]
	markerText := "load:unknown"
	if address != nil {
		markerText = fmt.Sprintf("load:%d", address.GetId())
	}
	marker := c.newConstant(markerText, ssa.CreateStringType())
	if len(instruction.Results) == 0 {
		return marker
	}
	value := c.castType(marker, c.typeFor(instruction.Results[0].Type))
	if user, ok := value.(ssa.User); ok && address != nil && value != address {
		address.AddUser(user)
	}
	return value
}

func (c *converter) markHeaderStore(key string) {
	if !c.loopHeaders[c.current] {
		return
	}
	if c.headerStore[c.current] == nil {
		c.headerStore[c.current] = make(map[string]bool)
	}
	c.headerStore[c.current][key] = true
}

func (c *converter) loopPhi(key string, variable *ssa.Variable) *ssa.Phi {
	if c.headerStore[c.current] != nil && c.headerStore[c.current][key] {
		return nil
	}
	if existing := c.loopPhis[c.current]; existing != nil {
		if phi := existing[key]; phi != nil {
			return phi
		}
	}
	var current ssa.Value
	if variable != nil {
		current = variable.GetValue()
	}
	phi := ssa.NewPhi(c.builder.CurrentBlock, key)
	if current != nil {
		phi.Edge = []int64{current.GetId()}
		phi.SetType(current.GetType())
		current.AddUser(phi)
	}
	c.builder.CurrentBlock.Phis = append(c.builder.CurrentBlock.Phis, phi.GetId())
	c.program.SetVirtualRegister(phi)
	if variable != nil {
		_ = variable.Assign(phi)
	}
	if c.loopPhis[c.current] == nil {
		c.loopPhis[c.current] = make(map[string]*ssa.Phi)
	}
	c.loopPhis[c.current][key] = phi
	return phi
}

func (c *converter) fillLoopCarried(block core.Block) {
	for _, instruction := range block.Instructions {
		if instruction.Kind != core.OpJump && instruction.Kind != core.OpBranch {
			continue
		}
		for _, target := range instruction.Targets {
			if !c.loopHeaders[target] {
				continue
			}
			for key, phi := range c.loopPhis[target] {
				if c.loopPhiDone[phi.GetId()] {
					continue
				}
				value := c.variableByKey(key)
				if value == nil || value.GetId() == phi.GetId() {
					c.loopPhiDone[phi.GetId()] = true
					continue
				}
				phi.Edge = append(phi.Edge, value.GetId())
				value.AddUser(phi)
				phi.GetProgram().SetVirtualRegister(phi)
				if variable := c.variableFor(key); variable != nil {
					_ = variable.Assign(phi)
				}
				c.loopPhiDone[phi.GetId()] = true
			}
		}
	}
}

func (c *converter) variableFor(key string) *ssa.Variable {
	switch {
	case strings.HasPrefix(key, "reg:"):
		return c.variables[strings.TrimPrefix(key, "reg:")]
	case strings.HasPrefix(key, "mem:"):
		raw := strings.TrimPrefix(key, "mem:")
		if name, ok := c.memoryNames[raw]; ok {
			raw = name
		}
		return c.memory[raw]
	}
	return nil
}

func (c *converter) variableByKey(key string) ssa.Value {
	if variable := c.variableFor(key); variable != nil {
		return variable.GetValue()
	}
	return nil
}

func (c *converter) memoryName(key string) string {
	if name, ok := c.memoryNames[key]; ok {
		return name
	}
	name := ""
	if strings.HasPrefix(key, "const:") {
		name = "mem_" + strings.TrimPrefix(key, "const:")
	} else {
		c.memoryIndex++
		name = fmt.Sprintf("mem_%d", c.memoryIndex)
	}
	c.memoryNames[key] = name
	return name
}

func (c *converter) memoryVariable(name string) *ssa.Variable {
	if variable, ok := c.memory[name]; ok {
		return variable
	}
	current := c.builder.CurrentBlock
	c.builder.CurrentBlock = c.entry
	variable := c.builder.CreateVariable(name)
	c.builder.CurrentBlock = current
	if variable == nil {
		return nil
	}
	c.memory[name] = variable
	return variable
}

func (c *converter) emitJump(instruction core.Instruction) {
	if len(instruction.Targets) == 1 {
		if target, ok := c.blocks[instruction.Targets[0]]; ok && target != nil {
			c.builder.EmitJump(target)
			return
		}
	}
	// An indirect jump has no static successor. End the block without inventing
	// a call or a fake target.
	if len(instruction.Operands) > 0 {
		c.builder.EmitReturn(nil)
		return
	}
	c.addDiagnostic("lowir2ssa-incomplete-jump", fmt.Sprintf("jump at 0x%x has no resolved target", instruction.Address))
	c.builder.EmitReturn(nil)
}

func (c *converter) emitBranch(instruction core.Instruction) {
	condition := c.operandValue(firstOperand(instruction.Operands))
	targets := c.targetBlocks(instruction.Targets)
	if condition == nil {
		c.addDiagnostic("lowir2ssa-missing-condition", fmt.Sprintf("branch at 0x%x has no condition", instruction.Address))
		c.builder.EmitReturn(nil)
		return
	}
	switch len(targets) {
	case 0:
		c.addDiagnostic("lowir2ssa-incomplete-branch", fmt.Sprintf("branch at 0x%x has no resolved targets", instruction.Address))
		c.builder.EmitReturn(nil)
	case 1:
		c.addDiagnostic("lowir2ssa-incomplete-branch", fmt.Sprintf("branch at 0x%x has only one resolved target", instruction.Address))
		c.emitIf(condition, targets[0], nil)
	default:
		c.emitIf(condition, targets[0], targets[1])
	}
}

func (c *converter) emitIf(condition ssa.Value, trueBlock, falseBlock *ssa.BasicBlock) {
	ifInstruction := c.builder.EmitIf()
	if ifInstruction == nil {
		return
	}
	ifInstruction.Cond = condition.GetId()
	condition.AddUser(ifInstruction)
	ifInstruction.True = trueBlock.GetId()
	c.builder.CurrentBlock.AddSucc(trueBlock)
	if falseBlock != nil {
		ifInstruction.False = falseBlock.GetId()
		c.builder.CurrentBlock.AddSucc(falseBlock)
	}
	c.builder.CurrentBlock.SetConditionInstID(ifInstruction.GetId())
	c.builder.CurrentBlock.SetConditionFromValue(condition, "if")
}

func (c *converter) targetBlocks(ids []core.BlockID) []*ssa.BasicBlock {
	var targets []*ssa.BasicBlock
	for _, id := range ids {
		if block, ok := c.blocks[id]; ok && block != nil {
			targets = append(targets, block)
		}
	}
	return targets
}

func (c *converter) operandValues(operands []core.Operand) []ssa.Value {
	values := make([]ssa.Value, 0, len(operands))
	for _, operand := range operands {
		values = append(values, c.operandValue(operand))
	}
	return values
}

func (c *converter) operandValue(operand core.Operand) ssa.Value {
	switch operand.Kind {
	case core.OperandValue:
		if value, ok := c.values[operand.Value]; ok && value != nil {
			return value
		}
		c.addDiagnostic("lowir2ssa-missing-value", fmt.Sprintf("value v%d is not defined", operand.Value))
		return c.undefinedValue(fmt.Sprintf("missing.v%d", operand.Value))
	case core.OperandConstant:
		return c.constValue(operand)
	case core.OperandRegister:
		view := registerViewFor(operand.Name, operand.Type.Bits)
		if view.canonical == "" {
			return c.builder.EmitConstInst(operand.Name)
		}
		value := c.registerValue(view)
		if value != nil && view.narrow() {
			value = c.extractRegister(value, view)
		}
		return value
	case core.OperandSymbol:
		return c.builder.EmitConstInst(operand.Name)
	default:
		return c.builder.EmitConstInst(string(operand.Kind))
	}
}

func (c *converter) constValue(operand core.Operand) ssa.Value {
	if len(operand.Literal) > 8 {
		c.addDiagnostic("lowir2ssa-wide-constant", "a constant wider than 64 bits was preserved as hexadecimal text")
		return c.builder.EmitConstInst("0x" + hex.EncodeToString(operand.Literal))
	}
	value := literalUint64(operand.Literal)
	return c.newConstant(value, c.typeFor(operand.Type))
}

// coerceType keeps a copy typed as its LowIR result. A copy between different
// widths or kinds becomes an explicit cast; a same-width copy reuses the value.
func (c *converter) coerceType(value ssa.Value, results []core.Value) ssa.Value {
	if len(results) == 0 {
		return value
	}
	typ := c.typeFor(results[0].Type)
	if typ == nil {
		return value
	}
	return c.castType(value, typ)
}

func (c *converter) typeFor(valueType core.ValueType) ssa.Type {
	if typ, ok := c.types[valueType]; ok {
		return typ
	}
	var typ ssa.Type
	switch valueType.Kind {
	case core.TypeInteger:
		if valueType.Bits == 0 {
			typ = ssa.CreateAnyType()
		} else {
			typ = ssa.NewBasicType(ssa.NumberTypeKind, fmt.Sprintf("i%d", valueType.Bits))
		}
	case core.TypePointer:
		bits := valueType.Bits
		if bits == 0 {
			bits = 64
		}
		typ = ssa.NewBasicType(ssa.NumberTypeKind, fmt.Sprintf("ptr%d", bits))
	case core.TypeFloat:
		bits := valueType.Bits
		if bits == 0 {
			bits = 64
		}
		typ = ssa.NewBasicType(ssa.NumberTypeKind, fmt.Sprintf("f%d", bits))
	case core.TypeOpaque:
		typ = ssa.CreateAnyType()
	default:
		typ = ssa.CreateAnyType()
	}
	c.types[valueType] = typ
	return typ
}

func (c *converter) setResult(instruction core.Instruction, value ssa.Value) {
	if value == nil || len(instruction.Results) == 0 {
		return
	}
	if len(instruction.Results) > 1 {
		c.addDiagnostic("lowir2ssa-multiple-results", "only the first result of a multi-result operation is represented")
	}
	if typ := c.typeFor(instruction.Results[0].Type); typ != nil {
		if current := value.GetType(); current == nil || current.String() != typ.String() {
			value.SetType(typ)
		}
	}
	c.values[instruction.Results[0].ID] = value
}

func (c *converter) addDiagnostic(code, message string) {
	c.diagnostics = append(c.diagnostics, core.Diagnostic{Code: code, Message: message})
}

func integerType(bits uint16) core.ValueType {
	return core.ValueType{Kind: core.TypeInteger, Bits: bits}
}

type registerView struct {
	canonical     string
	width         uint16
	shift         uint16
	canonicalBits uint16
}

func (v registerView) narrow() bool {
	return v.canonicalBits > 0 && v.width > 0 && v.width < v.canonicalBits
}

func (v registerView) partial() bool {
	return v.canonicalBits > 0 && v.width > 0 && v.width < 32
}

func (v registerView) mask() uint64 {
	width := v.width
	if width == 0 || width >= 64 {
		return math.MaxUint64
	}
	return (uint64(1) << width) - 1
}

// registerViewFor maps a machine register name to one SSA variable. Partial
// registers share the canonical 64-bit variable; their width and bit offset
// decide how a read is extracted or a write is merged.
func registerViewFor(name string, bits uint16) registerView {
	lower := strings.ToLower(name)
	if lower == "" {
		return registerView{}
	}
	if family, ok := generalRegisterFamily(lower); ok {
		width := bits
		if width == 0 {
			width = generalRegisterWidth(lower)
		}
		return registerView{
			canonical:     family,
			width:         width,
			shift:         generalRegisterShift(lower),
			canonicalBits: 64,
		}
	}
	switch {
	case strings.HasPrefix(lower, "xmm"), strings.HasPrefix(lower, "ymm"), strings.HasPrefix(lower, "zmm"):
		width := bits
		if width == 0 {
			width = 128
		}
		return registerView{canonical: lower, width: width, canonicalBits: width}
	case lower == "rflags", lower == "eflags", lower == "flags":
		width := bits
		if width == 0 {
			width = 64
		}
		return registerView{canonical: "rflags", width: width, canonicalBits: 64}
	default:
		return registerView{canonical: lower, width: bits}
	}
}

func generalRegisterFamily(name string) (string, bool) {
	switch name {
	case "al", "ah", "ax", "eax", "rax":
		return "rax", true
	case "bl", "bh", "bx", "ebx", "rbx":
		return "rbx", true
	case "cl", "ch", "cx", "ecx", "rcx":
		return "rcx", true
	case "dl", "dh", "dx", "edx", "rdx":
		return "rdx", true
	case "sil", "si", "esi", "rsi":
		return "rsi", true
	case "dil", "di", "edi", "rdi":
		return "rdi", true
	case "bpl", "bp", "ebp", "rbp":
		return "rbp", true
	case "spl", "sp", "esp", "rsp":
		return "rsp", true
	case "rip":
		return "rip", true
	}
	if len(name) < 2 || name[0] != 'r' || name[1] < '1' || name[1] > '9' {
		return "", false
	}
	digits := name[1:]
	if last := digits[len(digits)-1]; len(digits) > 1 && (last == 'b' || last == 'w' || last == 'd' || last == 'l') {
		digits = digits[:len(digits)-1]
	}
	if digits == "" {
		return "", false
	}
	for _, digit := range digits {
		if digit < '0' || digit > '9' {
			return "", false
		}
	}
	return "r" + digits, true
}

func generalRegisterWidth(name string) uint16 {
	switch name {
	case "ah", "bh", "ch", "dh":
		return 8
	}
	// x86asm names the 32-bit extended registers r8l..r15l.
	if len(name) > 2 && name[0] == 'r' && name[1] >= '1' && name[1] <= '9' {
		switch name[len(name)-1] {
		case 'b':
			return 8
		case 'w':
			return 16
		case 'd', 'l':
			return 32
		}
	}
	switch {
	case strings.HasSuffix(name, "b"), strings.HasSuffix(name, "l"):
		return 8
	case strings.HasSuffix(name, "w"):
		return 16
	case strings.HasSuffix(name, "d"):
		return 32
	default:
		return 64
	}
}

func generalRegisterShift(name string) uint16 {
	switch name {
	case "ah", "bh", "ch", "dh":
		return 8
	default:
		return 0
	}
}

func functionName(function core.Function) string {
	if function.Name != "" {
		return function.Name
	}
	if function.Address != 0 {
		return fmt.Sprintf("sub_0x%x", function.Address)
	}
	return fmt.Sprintf("fn_%d", function.ID)
}

func firstOperand(operands []core.Operand) core.Operand {
	if len(operands) == 0 {
		return core.Operand{}
	}
	return operands[0]
}

func literalUint64(literal []byte) uint64 {
	var buffer [8]byte
	copy(buffer[:], literal)
	return binary.LittleEndian.Uint64(buffer[:])
}

func containsPass(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}
