// Package lowir2ssa converts Reify LowIR into a YakSSA program.
//
// Pure values become typed SSA values. General-purpose registers become typed
// variables: a read uses the current value, a write redefines it, and no Phi is
// inserted. Memory accesses, calls, intrinsics and unsupported operations stay
// explicit as calls to reserved "reify." pseudo-functions.
package lowir2ssa

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// Result is one converted YakSSA program.
type Result struct {
	Program     *ssa.Program
	Diagnostics []core.Diagnostic
}

// Options controls conversion metadata and passes.
type Options struct {
	ProgramName     string
	KeepStackCanary bool
}

// Option customizes conversion.
type Option func(*Options)

// WithProgramName sets the generated YakSSA program name.
func WithProgramName(name string) Option {
	return func(options *Options) {
		options.ProgramName = name
	}
}

// WithKeepStackCanary keeps recognized stack-protector code. The default
// removes it, so the generated program only shows application logic.
func WithKeepStackCanary() Option {
	return func(options *Options) {
		options.KeepStackCanary = true
	}
}

// WithStackCanaryRemoval enables or disables the stack-canary pass.
func WithStackCanaryRemoval(remove bool) Option {
	return func(options *Options) {
		options.KeepStackCanary = !remove
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
		if !settings.KeepStackCanary {
			RemoveStackCanary(&function)
		}
		if err := converter.convertFunction(function); err != nil {
			return nil, err
		}
	}
	return &Result{Program: program, Diagnostics: converter.diagnostics}, nil
}

// Builder adapts Convert to the core.SSAIRBuilder contract.
type Builder struct{}

// BuildSSAIR converts LowIR to YakSSA.
func (Builder) BuildSSAIR(ctx context.Context, ir *core.LowIR) (*ssa.Program, error) {
	result, err := Convert(ctx, ir)
	if err != nil {
		return nil, err
	}
	return result.Program, nil
}

// Text renders the converted program with YakSSA's disassembler. Conversion
// scaffolding lives in functions named with the reserved "reify." prefix; Text
// omits them.
func Text(program *ssa.Program) string {
	if program == nil {
		return ""
	}
	var text strings.Builder
	program.Funcs.ForEach(func(_ string, function *ssa.Function) bool {
		if function == nil || isPseudoFunction(function) {
			return true
		}
		text.WriteString(function.DisAsm(ssa.DisAsmDefault))
		return true
	})
	return text.String()
}

func isPseudoFunction(function *ssa.Function) bool {
	return strings.HasPrefix(function.GetName(), "reify.")
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
	variables   map[string]*ssa.Variable
	types       map[core.ValueType]ssa.Type
	pseudo      map[string]ssa.Value
	undefined   map[string]ssa.Value
}

func (c *converter) convertFunction(function core.Function) error {
	name := functionName(function)
	ssaFunction := c.program.NewFunction(name)
	builder := ssaFunction.GetOrCreateBuilder()
	if builder == nil {
		return fmt.Errorf("lowir2ssa: cannot create builder for %s", name)
	}
	c.function = ssaFunction
	c.builder = builder
	c.entry = builder.CurrentBlock
	c.blocks = make(map[core.BlockID]*ssa.BasicBlock, len(function.Blocks))
	c.values = make(map[core.ValueID]ssa.Value)
	c.variables = make(map[string]*ssa.Variable)
	c.types = make(map[core.ValueType]ssa.Type)
	c.pseudo = make(map[string]ssa.Value)
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
		if err := c.convertBlock(block); err != nil {
			return err
		}
	}
	ssaFunction.Finish()
	return nil
}

func (c *converter) convertBlock(block core.Block) error {
	ssaBlock, ok := c.blocks[block.ID]
	if !ok {
		return fmt.Errorf("lowir2ssa: block b%d is missing", block.ID)
	}
	c.builder.CurrentBlock = ssaBlock
	for _, instruction := range block.Instructions {
		if err := c.ctx.Err(); err != nil {
			return err
		}
		c.convertInstruction(instruction)
	}
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
			c.setResult(instruction, c.call("reify.copy", nil))
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
		c.setResult(instruction, c.call("reify.select", c.operandValues(instruction.Operands)))
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
		c.setResult(instruction, c.call("reify.load", c.operandValues(instruction.Operands)))
	case core.OpStore:
		c.call("reify.store", c.operandValues(instruction.Operands))
	case core.OpCall:
		c.emitCall(instruction)
	case core.OpIntrinsic:
		c.emitIntrinsic(instruction)
	case core.OpZeroExtend, core.OpSignExtend, core.OpTruncate:
		c.setResult(instruction, c.call("reify."+string(instruction.Kind), c.operandValues(instruction.Operands)))
	case core.OpJump:
		c.emitJump(instruction)
	case core.OpBranch:
		c.emitBranch(instruction)
	case core.OpReturn:
		c.builder.EmitReturn(c.operandValues(instruction.Operands))
	case core.OpTrap:
		c.call("reify.trap", c.operandValues(instruction.Operands))
		c.builder.EmitReturn(nil)
	case core.OpNop:
		c.call("reify.nop", nil)
	default:
		c.addDiagnostic("lowir2ssa-unsupported-op", fmt.Sprintf("operation %q was preserved as an opaque call", instruction.Kind))
		c.setResult(instruction, c.call("reify.op_"+string(instruction.Kind), c.operandValues(instruction.Operands)))
	}
}

func (c *converter) emitBinary(instruction core.Instruction, op ssa.BinaryOpcode) {
	values := c.operandValues(instruction.Operands)
	if len(values) < 2 {
		c.addDiagnostic("lowir2ssa-missing-operand", fmt.Sprintf("operation %q at 0x%x has %d operands", instruction.Kind, instruction.Address, len(values)))
		c.setResult(instruction, c.call("reify."+string(instruction.Kind), values))
		return
	}
	c.setResult(instruction, c.builder.EmitBinOp(op, values[0], values[1]))
}

func (c *converter) emitUnary(instruction core.Instruction, op ssa.UnaryOpcode) {
	values := c.operandValues(instruction.Operands)
	if len(values) == 0 {
		c.addDiagnostic("lowir2ssa-missing-operand", fmt.Sprintf("operation %q at 0x%x has no operand", instruction.Kind, instruction.Address))
		c.setResult(instruction, c.call("reify."+string(instruction.Kind), nil))
		return
	}
	c.setResult(instruction, c.builder.EmitUnOp(op, values[0]))
}

func (c *converter) emitRegisterRead(instruction core.Instruction) {
	operand := firstOperand(instruction.Operands)
	view := registerViewFor(operand.Name, operand.Type.Bits)
	if view.canonical == "" {
		c.setResult(instruction, c.call("reify.read-register", nil))
		return
	}
	value := c.registerValue(view)
	if value == nil {
		c.setResult(instruction, c.call("reify.read-register", nil))
		return
	}
	if view.narrow() {
		value = c.extractRegister(value, view)
	}
	c.setResult(instruction, value)
}

func (c *converter) emitRegisterWrite(instruction core.Instruction) {
	if len(instruction.Operands) < 2 {
		c.call("reify.write-register", c.operandValues(instruction.Operands))
		return
	}
	operand := instruction.Operands[0]
	value := c.operandValue(instruction.Operands[1])
	if value == nil {
		return
	}
	view := registerViewFor(operand.Name, operand.Type.Bits)
	if view.canonical == "" {
		c.call("reify.write-register", c.operandValues(instruction.Operands))
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
		return c.builder.EmitConstInst(int64(value))
	}
	return c.builder.EmitConstInst(value)
}

func (c *converter) emitCall(instruction core.Instruction) {
	if len(instruction.Operands) == 0 {
		c.setResult(instruction, c.call("reify.call", nil))
		return
	}
	operand := instruction.Operands[0]
	switch operand.Kind {
	case core.OperandConstant:
		c.setResult(instruction, c.call(fmt.Sprintf("reify.call_0x%x", literalUint64(operand.Literal)), nil))
	case core.OperandSymbol:
		c.setResult(instruction, c.call("reify.call_"+operand.Name, nil))
	default:
		c.setResult(instruction, c.call("reify.call", []ssa.Value{c.operandValue(operand)}))
	}
}

func (c *converter) emitIntrinsic(instruction core.Instruction) {
	if len(instruction.Operands) == 0 {
		c.setResult(instruction, c.call("reify.intrinsic", nil))
		return
	}
	name := "reify.intrinsic"
	operands := instruction.Operands
	if operands[0].Kind == core.OperandSymbol {
		name = "reify.intrinsic." + operands[0].Name
		operands = operands[1:]
	}
	c.setResult(instruction, c.call(name, c.operandValues(operands)))
}

func (c *converter) emitJump(instruction core.Instruction) {
	if len(instruction.Targets) == 1 {
		if target, ok := c.blocks[instruction.Targets[0]]; ok && target != nil {
			c.builder.EmitJump(target)
			return
		}
	}
	// A jump with an operand is indirect. It has no static successor, so the
	// target value is preserved as a pseudo-call instead of a CFG edge.
	if len(instruction.Operands) > 0 {
		c.call("reify.jump", c.operandValues(instruction.Operands))
		c.builder.EmitReturn(nil)
		return
	}
	c.addDiagnostic("lowir2ssa-incomplete-jump", fmt.Sprintf("jump at 0x%x has no resolved target", instruction.Address))
	c.call("reify.jump", nil)
	c.builder.EmitReturn(nil)
}

func (c *converter) emitBranch(instruction core.Instruction) {
	condition := c.operandValue(firstOperand(instruction.Operands))
	targets := c.targetBlocks(instruction.Targets)
	if condition == nil {
		c.addDiagnostic("lowir2ssa-missing-condition", fmt.Sprintf("branch at 0x%x has no condition", instruction.Address))
		c.call("reify.branch", nil)
		c.builder.EmitReturn(nil)
		return
	}
	switch len(targets) {
	case 0:
		c.addDiagnostic("lowir2ssa-incomplete-branch", fmt.Sprintf("branch at 0x%x has no resolved targets", instruction.Address))
		c.call("reify.branch", []ssa.Value{condition})
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
	constant := c.constantLiteral(value)
	if constant != nil {
		if typ := c.typeFor(operand.Type); typ != nil {
			constant.SetType(typ)
		}
	}
	return constant
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

func (c *converter) call(name string, args []ssa.Value) ssa.Value {
	target := c.pseudoFunction(name)
	call := c.builder.NewCall(target, args)
	return c.builder.EmitCall(call)
}

func (c *converter) pseudoFunction(name string) ssa.Value {
	if target, ok := c.pseudo[name]; ok {
		return target
	}
	target := c.program.NewFunction(name)
	c.pseudo[name] = target
	return target
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
