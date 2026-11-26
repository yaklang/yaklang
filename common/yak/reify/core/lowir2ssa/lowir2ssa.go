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
	"os"
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
	// CacheKind selects the program cache. The default keeps the program in
	// memory; a producer that writes ssadb asks for ProgramCacheDBWrite.
	CacheKind ssa.ProgramCacheKind
	// Language is recorded on the program and the ir_programs row.
	Language ssaconfig.Language
	// PointerAt resolves a pointer-sized value stored at an image address. A
	// target reaches a global through a table of addresses, so a load from that
	// table has to become the address it holds; otherwise every access stays
	// symbolic and the object it names cannot be recovered.
	PointerAt func(address uint64, size uint16) (uint64, bool)
}

// WithProgramCache selects the program cache kind. ProgramCacheDBWrite makes
// every emitted instruction reach ssadb at SaveToDatabase time.
func WithProgramCache(kind ssa.ProgramCacheKind) Option {
	return func(options *Options) { options.CacheKind = kind }
}

// WithLanguage records the program language. A lifted binary has no source
// language of its own, so the caller names the language family it models.
func WithLanguage(language ssaconfig.Language) Option {
	return func(options *Options) { options.Language = language }
}

// WithPointerReader supplies the table lookup that turns a load from a table of
// addresses into the address it holds.
func WithPointerReader(read func(address uint64, size uint16) (uint64, bool)) Option {
	return func(options *Options) { options.PointerAt = read }
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
	settings := Options{ProgramName: "reify-lowir", CacheKind: ssa.ProgramCacheMemory, Language: ssaconfig.General}
	for _, option := range options {
		option(&settings)
	}
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(settings.ProgramName))
	if err != nil {
		return nil, fmt.Errorf("lowir2ssa: invalid program config: %w", err)
	}
	program := ssa.NewProgram(cfg, settings.CacheKind, ssa.Application, nil, "", 0)
	if settings.Language != "" {
		program.Language = settings.Language
	}
	converter := &converter{
		ctx:          ctx,
		program:      program,
		facts:        ir.Facts(),
		pointerAt:    settings.PointerAt,
		frameValues:  make(map[ssa.Value]bool),
		spDerived:    make(map[ssa.Value]bool),
		frameDerived: make(map[ssa.Value]bool),
		diagnostics:  make([]core.Diagnostic, 0),
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
		facts := ir.FactsFor(function)
		if err := pass.Apply(&function, facts, names...); err != nil {
			return nil, err
		}
		converter.facts = facts
		function.FrameCleanup = containsPass(names, "remove-stackframe")
		function.FrameKept = !function.FrameCleanup
		if err := convertFunctionSafely(converter, function); err != nil {
			return nil, err
		}
	}
	return &Result{Program: program, Diagnostics: converter.diagnostics}, nil
}

// convertFunctionSafely turns a conversion panic into an error that names the
// function. A malformed instruction stream must not abort the process, and the
// caller can report the diagnostic with the rest of the run.
func convertFunctionSafely(converter *converter, function core.Function) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("lowir2ssa: function %s at 0x%x: %v", functionName(function), function.Address, recovered)
		}
	}()
	return converter.convertFunction(function)
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
	// mergePhis holds the join phis of a block keyed by the register key, and
	// blockOut holds the value a block leaves in each register.
	mergePhis   map[core.BlockID]map[string]*ssa.Phi
	blockOut    map[core.BlockID]map[string]ssa.Value
	lowirBySSA  map[int64]core.BlockID
	idom        map[core.BlockID]core.BlockID
	predsOf     map[core.BlockID][]core.BlockID
	current     core.BlockID
	types       map[core.ValueType]ssa.Type
	signedTypes map[uint16]ssa.Type
	undefined   map[string]ssa.Value
	facts       core.ArchFacts
	// memoryValid records whether a memory variable still holds the value its
	// last store wrote. A store through a different address expression that may
	// alias it clears the flag, so a later load keeps the reload.
	memoryValid map[string]bool
	memoryKind  map[string]memorySlotKind
	// stackStores collects the values a cdecl caller stores for its arguments.
	stackStores []ssa.Value
	// frameValues marks the values read from a frame register, which a prologue
	// saves but never passes as an argument.
	frameValues map[ssa.Value]bool
	// pointerAt resolves the pointer stored at an image address.
	pointerAt func(address uint64, size uint16) (uint64, bool)
	// spDerived marks the values computed from the stack pointer. A cdecl
	// argument area is the only thing a caller stores through that chain.
	spDerived map[ssa.Value]bool
	// frameDerived marks the values computed from the frame pointer. A local
	// variable slot is addressed through either pointer, and both are inside the
	// current frame.
	frameDerived map[ssa.Value]bool
	// sourceFunction is the LowIR function being converted. A few entry values are
	// derived from its address.
	sourceFunction core.Function
}

func (c *converter) convertFunction(function core.Function) error {
	c.sourceFunction = function
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
	c.mergePhis = make(map[core.BlockID]map[string]*ssa.Phi)
	c.blockOut = make(map[core.BlockID]map[string]ssa.Value)
	c.lowirBySSA = make(map[int64]core.BlockID, len(function.Blocks))
	c.types = make(map[core.ValueType]ssa.Type)
	c.signedTypes = make(map[uint16]ssa.Type)
	c.undefined = make(map[string]ssa.Value)
	c.memoryValid = make(map[string]bool)
	c.memoryKind = make(map[string]memorySlotKind)
	c.frameDerived = make(map[ssa.Value]bool)

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
	// The block ids are stable only after every block exists, so the reverse map is
	// built here rather than inside the creation loop.
	for _, block := range function.Blocks {
		if ssaBlock := c.blocks[block.ID]; ssaBlock != nil {
			c.lowirBySSA[ssaBlock.GetId()] = block.ID
		}
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
	c.planMergePhis(function)
	for _, block := range function.Blocks {
		if err := c.convertBlock(block); err != nil {
			return err
		}
	}
	c.fillMergePhis()
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
	// The entry parameters are the values the first block leaves in those
	// registers when it does not write them again.
	if len(c.sourceFunction.Blocks) > 0 {
		c.current = c.sourceFunction.Blocks[0].ID
	}
	c.builder.CurrentBlock = c.entry
	stackPointer := ""
	if sp := c.facts.Frame.StackPointer; sp != "" {
		stackPointer = c.registerView(sp, 0).canonical
	}
	for _, name := range c.entryRegisters() {
		parameterName := "entry_" + name
		if stackPointer != "" && c.registerView(name, 0).canonical == stackPointer {
			// The stack pointer is not a parameter of the recovered function. Naming
			// it frame keeps the ABI register out of the source.
			parameterName = "frame"
		}
		parameter := ssa.NewParam(parameterName, false, c.builder)
		if parameter == nil {
			continue
		}
		parameter.SetType(c.typeFor(integerType(64)))
		c.assignRegister(name, parameter)
	}
	// A function may know its own address at entry. Binding it lets a PowerPC
	// callee compute its TOC pointer, which is what turns a TOC-relative access
	// into a named literal.
	if name := c.facts.Registers.EntryAddress; name != "" {
		if value := c.builder.EmitConstInst(int64(c.sourceFunction.Address)); value != nil {
			c.assignRegister(name, value)
		}
	}
	// A global pointer register carries a base address the loader resolved. Binding
	// it turns an indirect access into an address the renderer can name.
	if name := c.facts.Registers.GlobalPointer; name != "" && c.facts.Registers.GlobalPointerAddress != 0 {
		if value := c.builder.EmitConstInst(int64(c.facts.Registers.GlobalPointerAddress)); value != nil {
			value.SetType(c.typeFor(core.ValueType{Kind: core.TypePointer, Bits: c.facts.Registers.Width}))
			c.assignRegister(name, value)
		}
	}
	for index, name := range c.argumentRegisters() {
		parameter := ssa.NewParam(fmt.Sprintf("arg%d", index), false, c.builder)
		if parameter == nil {
			continue
		}
		parameter.FormalParameterIndex = index
		c.function.Params = append(c.function.Params, parameter.GetId())
		c.assignRegister(name, parameter)
	}
}

// entryRegisters lists the frame registers that start with ABI-defined values.
// They come from the frame model, so a new architecture needs no change here.
func (c *converter) entryRegisters() []string {
	return c.facts.Frame.EntryRegisters()
}

// argumentRegisters lists the integer argument registers of the target ABI. An
// empty list means the ABI passes every argument on the stack.
func (c *converter) argumentRegisters() []string {
	return c.facts.Registers.Arguments
}

// returnRegister names the register that carries a scalar result.
func (c *converter) returnRegister() string {
	return c.facts.Registers.Return
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
		// A terminator does not necessarily finish the block: a jump with an
		// unresolved target falls back to a return, and a malformed input can
		// place more instructions after it. They are unreachable, and emitting
		// them would build SSA on a finished block, where every operation
		// returns nil.
		switch instruction.Kind {
		case core.OpJump, core.OpBranch, core.OpReturn, core.OpTrap:
			c.builder.EmitReturn(nil)
			c.fillLoopCarried(block)
			return nil
		}
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
		// A copy of a constant is that constant. A cast would produce a new value
		// that no longer folds, and a target builds an address chain out of copies:
		// the base register is copied, the displacement is added, and the sum has
		// to collapse into one address.
		if _, ok := ssa.ToConstInst(value); ok {
			if typ := c.typeFor(instruction.Results[0].Type); typ != nil {
				value.SetType(typ)
			}
			c.setResult(instruction, value)
			return
		}
		c.setResult(instruction, c.coerceType(value, instruction.Results))
	case core.OpPhi:
		c.emitPhi(instruction)
	case core.OpSelect:
		// The split-select pass rewrites a conditional move into control flow, so a
		// select that reaches the conversion was not split.
		c.addDiagnostic("lowir2ssa-unexpected-select", fmt.Sprintf("select at 0x%x was not split into control flow", instruction.Address))
		c.setResult(instruction, c.opaqueValue("select", instruction))
	case core.OpAdd:
		c.emitBinary(instruction, ssa.OpAdd)
	case core.OpSub:
		c.emitBinary(instruction, ssa.OpSub)
	case core.OpMul:
		c.emitBinary(instruction, ssa.OpMul)
	case core.OpUDiv:
		c.emitOrderedBinary(instruction, ssa.OpDiv, false)
	case core.OpSDiv:
		c.emitOrderedBinary(instruction, ssa.OpDiv, true)
	case core.OpAnd:
		c.emitBinary(instruction, ssa.OpAnd)
	case core.OpOr:
		c.emitBinary(instruction, ssa.OpOr)
	case core.OpXor:
		c.emitBinary(instruction, ssa.OpXor)
	case core.OpShl:
		c.emitBinary(instruction, ssa.OpShl)
	case core.OpLShr:
		c.emitOrderedBinary(instruction, ssa.OpShr, false)
	case core.OpAShr:
		c.emitOrderedBinary(instruction, ssa.OpShr, true)
	case core.OpEqual:
		c.emitBinary(instruction, ssa.OpEq)
	case core.OpULess:
		c.emitOrderedCompare(instruction, ssa.OpLt, false)
	case core.OpSLess:
		c.emitOrderedCompare(instruction, ssa.OpLt, true)
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
		c.rememberStackStore(instruction)
		c.emitStore(instruction)
	case core.OpCall:
		c.emitCall(instruction)
	case core.OpIntrinsic:
		c.emitIntrinsic(instruction)
	case core.OpZeroExtend, core.OpSignExtend, core.OpTruncate:
		c.emitWidthCast(instruction)
	case core.OpJump:
		c.stackStores = c.stackStores[:0]
		c.emitJump(instruction)
	case core.OpBranch:
		c.emitBranch(instruction)
	case core.OpReturn:
		values := c.operandValues(instruction.Operands)
		if len(values) == 0 {
			// The instruction names the architecture return register in the
			// architecture spelling (eax on 32-bit x86). Register writes are
			// stored under the canonical view, so the fallback has to normalize
			// the name first or it reads a variable that was never written.
			if name := c.returnRegister(); name != "" {
				view := c.registerView(name, c.facts.Registers.Width)
				if value := c.registerValue(view); value != nil {
					if view.narrow() {
						value = c.extractRegister(value, view)
					}
					values = append(values, value)
				}
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
	// Fold at the width the result declares: a 32-bit addition wraps at 32 bits,
	// so a negative addend must not grow into the upper half.
	bits := uint16(0)
	if len(instruction.Results) == 1 {
		bits = instruction.Results[0].Type.Bits
	}
	c.setResult(instruction, c.emitBinOpWidth(op, values[0], values[1], bits))
}

// emitBinOpWidth folds one operation and truncates the result to the width the
// instruction declares.
func (c *converter) emitBinOpWidth(op ssa.BinaryOpcode, left, right ssa.Value, bits uint16) ssa.Value {
	if value, ok := c.foldBinOpWidth(op, left, right, bits); ok {
		return value
	}
	return c.builder.EmitBinOp(op, left, right)
}

// foldBinOpWidth computes one operation on two constants and truncates it.
func (c *converter) foldBinOpWidth(op ssa.BinaryOpcode, left, right ssa.Value, bits uint16) (ssa.Value, bool) {
	value, ok := c.foldBinOp(op, left, right)
	if !ok {
		return nil, false
	}
	if bits == 0 || bits >= 64 {
		return value, true
	}
	raw, ok := ssa.ToConstInst(value)
	if !ok {
		return value, true
	}
	number, ok := constInteger(raw)
	if !ok {
		return value, true
	}
	mask := int64(1)<<bits - 1
	sign := int64(1) << (bits - 1)
	number &= mask
	if number&sign != 0 {
		number -= int64(1) << bits
	}
	folded := c.builder.EmitConstInst(number)
	if folded == nil {
		return value, true
	}
	folded.SetType(value.GetType())
	return folded, true
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
	view := c.registerView(operand.Name, operand.Type.Bits)
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
	if c.isFrameRegister(view.canonical) {
		// Saving the frame pointer or the return address is not an argument, so
		// a later cdecl call must not treat the store as one.
		c.frameValues[value] = true
	}
	if view.canonical == c.registerView(c.facts.Frame.StackPointer, 0).canonical {
		c.spDerived[value] = true
	}
	if framePointer := c.facts.Frame.FramePointer; framePointer != "" && view.canonical == c.registerView(framePointer, 0).canonical {
		c.frameDerived[value] = true
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
	view := c.registerView(operand.Name, operand.Type.Bits)
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
		if cleared == nil {
			c.addDiagnostic("lowir2ssa-partial-write", fmt.Sprintf("cannot clear %s at 0x%x", view.canonical, instruction.Address))
			return
		}
		inserted := value
		if view.shift > 0 {
			inserted = c.typedBinOp(ssa.OpShl, inserted, c.constantLiteral(uint64(view.shift)), integerType(64))
		}
		inserted = c.typedBinOp(ssa.OpAnd, inserted, mask, integerType(64))
		value = c.typedBinOp(ssa.OpOr, cleared, inserted, integerType(64))
	}
	if value == nil {
		return
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
	c.recordRegisterValue(view.canonical, value)
	c.markHeaderStore("reg:" + view.canonical)
	if c.isArgumentRegister(view.canonical) {
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

// registerView returns the register view for the current architecture. The rule
// comes from the register model, because the same spelling means different things
// on different targets: r8 is an x86 extended register and an ARM register, and
// x2 is a RISC-V register, not the second half of an AArch64 pair.
func (c *converter) registerView(name string, bits uint16) registerView {
	return registerViewFor(name, bits, c.facts.Registers)
}

func isARMRegister(name string) bool {
	lower := strings.ToLower(name)
	if !strings.HasPrefix(lower, "r") {
		return false
	}
	index := 0
	for _, digit := range lower[1:] {
		if digit < '0' || digit > '9' {
			return false
		}
		index = index*10 + int(digit-'0')
	}
	return index >= 0 && index <= 15
}

func (c *converter) assignRegister(name string, value ssa.Value) {
	if value == nil || value.IsUndefined() {
		return
	}
	view := c.registerView(name, 0)
	variable := c.registerVariable(view.canonical)
	if variable == nil {
		return
	}
	if err := variable.Assign(value); err != nil {
		c.addDiagnostic("lowir2ssa-assignment", fmt.Sprintf("cannot assign %s: %v", view.canonical, err))
	}
	c.recordRegisterValue(view.canonical, value)
}

func (c *converter) registerValue(view registerView) ssa.Value {
	// Resolve the value along the dominator chain. The variable holds whichever
	// write the conversion reached last, and blocks are converted in reverse
	// postorder, so a block converted earlier can leave a value that does not
	// dominate this read.
	if value := c.outgoingRegisterValue(c.current, "reg:"+view.canonical); value != nil {
		return value
	}
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
	value := c.emitBinOp(op, left, right)
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
	if os.Getenv("REIFY_DEBUG_CALL") != "" {
		fmt.Fprintf(os.Stderr, "call@%x opargs=%d arguments=%v names=%v\n", instruction.Address, len(args), c.arguments, c.argumentRegisters())
	}
	args = append(args, c.captureArguments()...)
	if c.facts.Registers.StackArguments {
		// The arguments are the values the caller stored just before the call,
		// last store first.
		args = append(args, c.stackArguments()...)
		c.stackStores = c.stackStores[:0]
	}
	call := c.builder.EmitCall(c.builder.NewCall(target, args))
	if call == nil {
		// The builder refuses a call in some malformed states. Keep the
		// instruction visible through the diagnostic instead of assigning a
		// nil value to the return register.
		c.addDiagnostic("lowir2ssa-call", "builder dropped a call instruction")
		return
	}
	if instruction.Variadic {
		call.SetVerboseName("variadic")
	}
	c.setResult(instruction, call)
	// AMD64 returns in rax, 386 in eax, ARM64 in x0 and ARM in r0.
	c.assignRegister(c.returnRegister(), call)
}

// captureArguments reads the System V AMD64 integer argument registers that
// were written since the previous call. Registers above the highest written
// slot are omitted; missing lower slots become undefined arguments.
// stackArguments returns the values of the stack stores that precede a call, in
// argument order. A caller that pushes b then a passes a as the first argument,
// so the last store comes first.
func (c *converter) stackArguments() []ssa.Value {
	if len(c.stackStores) == 0 {
		return nil
	}
	limit := len(c.stackStores)
	if limit > 6 {
		limit = 6
	}
	args := make([]ssa.Value, 0, limit)
	for index := 0; index < limit; index++ {
		args = append(args, c.stackStores[len(c.stackStores)-1-index])
	}
	return args
}

func (c *converter) captureArguments() []ssa.Value {
	names := c.argumentRegisters()
	// Arguments are positional: the caller fills a prefix of the argument
	// registers. A register written past a gap was written for something else, so
	// the capture stops at the gap instead of inventing the arguments in between.
	count := 0
	for _, name := range names {
		if !c.arguments[name] {
			break
		}
		count++
	}
	if count == 0 {
		return nil
	}
	args := make([]ssa.Value, 0, count)
	for index := 0; index < count; index++ {
		name := names[index]
		args = append(args, c.registerValue(c.registerView(name, 0)))
		c.arguments[name] = false
	}
	for _, name := range names[count:] {
		c.arguments[name] = false
	}
	return args
}

// isArgumentRegister reports whether one register carries a call argument. The
// list comes from the ABI facts, so a target whose argument registers are not
// r0..r3 and rdi..r9 still captures them.
func (c *converter) isArgumentRegister(name string) bool {
	if name == "" {
		return false
	}
	for _, candidate := range c.argumentRegisters() {
		if c.registerView(candidate, 0).canonical == name {
			return true
		}
	}
	return false
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
	if flags, ok := flagSymbolName(name); ok {
		info := flagInfo{name: flags}
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
	if conditionCode, ok := conditionSymbolName(name); ok {
		if condition := c.conditionValue(conditionCode, instruction); condition != nil {
			c.setResult(instruction, condition)
			return
		}
	}
	c.setResult(instruction, c.opaqueValue(name, instruction))
}

// flagSymbolName returns the comparison name of a flag update, for example
// flags.cmp. Every lifter writes this one namespace, so a consumer does not have
// to enumerate architecture spellings.
func flagSymbolName(symbol string) (string, bool) {
	if strings.HasPrefix(symbol, core.FlagsIntrinsicPrefix) {
		return strings.TrimPrefix(symbol, core.FlagsIntrinsicPrefix), true
	}
	return "", false
}

// conditionSymbolName returns the condition code of an architecture condition
// read, for example cond.e.
func conditionSymbolName(symbol string) (string, bool) {
	if strings.HasPrefix(symbol, core.ConditionIntrinsicPrefix) {
		return strings.TrimPrefix(symbol, core.ConditionIntrinsicPrefix), true
	}
	return "", false
}

func (c *converter) conditionValue(cc string, instruction core.Instruction) ssa.Value {
	if cc == "" || len(instruction.Operands) < 2 || instruction.Operands[1].Kind != core.OperandValue {
		return nil
	}
	flags := c.operandValue(instruction.Operands[1])
	if flags == nil {
		return nil
	}
	info, ok := c.flagInfoOf(flags)
	if !ok {
		return nil
	}
	switch info.name {
	case "cmp", "cmn", "cmpwi", "cmpdi", "cmplwi", "cmpldi",
		"cmpw", "cmpd", "cmplw", "cmpld":
		if len(info.operands) < 2 {
			return nil
		}
		return c.compareCondition(cc, info.operands[0], info.operands[1])
	case "test", "tst":
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

// emitBinOp emits one binary operation, folding it when both operands are
// already constants. A target writes an address in pieces, and some of those
// pieces only become constant after the entry registers are bound; folding here
// is what collapses the chain into one address the renderer can name.
func (c *converter) emitBinOp(op ssa.BinaryOpcode, left, right ssa.Value) ssa.Value {
	if value, ok := c.foldBinOp(op, left, right); ok {
		return value
	}
	return c.builder.EmitBinOp(op, left, right)
}

// foldBinOp computes one operation on two constant operands.
func (c *converter) foldBinOp(op ssa.BinaryOpcode, left, right ssa.Value) (ssa.Value, bool) {
	leftConstant, ok := ssa.ToConstInst(left)
	if !ok {
		return nil, false
	}
	rightConstant, ok := ssa.ToConstInst(right)
	if !ok {
		return nil, false
	}
	a, ok := constInteger(leftConstant)
	if !ok {
		return nil, false
	}
	b, ok := constInteger(rightConstant)
	if !ok {
		return nil, false
	}
	var result int64
	switch op {
	case ssa.OpAdd:
		result = a + b
	case ssa.OpSub:
		result = a - b
	case ssa.OpMul:
		result = a * b
	case ssa.OpAnd:
		result = a & b
	case ssa.OpOr:
		result = a | b
	case ssa.OpXor:
		result = a ^ b
	case ssa.OpShl:
		if b < 0 || b >= 64 {
			return nil, false
		}
		result = a << uint(b)
	case ssa.OpShr:
		if b < 0 || b >= 64 {
			return nil, false
		}
		result = int64(uint64(a) >> uint(b))
	default:
		return nil, false
	}
	value := c.builder.EmitConstInst(result)
	if value == nil {
		return nil, false
	}
	if left != nil {
		value.SetType(left.GetType())
	}
	return value, true
}

// constInteger reads the integer payload of one constant.
func constInteger(constant *ssa.ConstInst) (int64, bool) {
	switch value := constant.GetRawValue().(type) {
	case int64:
		return value, true
	case int:
		return int64(value), true
	case uint64:
		return int64(value), true
	}
	return 0, false
}

func (c *converter) compareCondition(cc string, leftID, rightID core.ValueID) ssa.Value {
	left := c.values[leftID]
	right := c.values[rightID]
	if left == nil || right == nil {
		return nil
	}
	switch cc {
	case "e", "eq", "z":
		return c.builder.EmitBinOp(ssa.OpEq, left, right)
	case "ne", "nz":
		return c.builder.EmitBinOp(ssa.OpNotEq, left, right)
	case "l", "lt":
		return c.orderedCompareSource(ssa.OpLt, left, right, true)
	case "b", "c", "cc", "lo":
		return c.orderedCompareSource(ssa.OpLt, left, right, false)
	case "le":
		return c.orderedCompareSource(ssa.OpLtEq, left, right, true)
	case "be", "ls":
		return c.orderedCompareSource(ssa.OpLtEq, left, right, false)
	case "g", "gt":
		return c.orderedCompareSource(ssa.OpGt, left, right, true)
	case "a", "hi":
		return c.orderedCompareSource(ssa.OpGt, left, right, false)
	case "ge":
		return c.orderedCompareSource(ssa.OpGtEq, left, right, true)
	case "ae", "cs", "hs":
		return c.orderedCompareSource(ssa.OpGtEq, left, right, false)
	default:
		return nil
	}
}

func (c *converter) zeroCondition(cc string, subject ssa.Value) ssa.Value {
	if subject == nil {
		return nil
	}
	zero := c.newConstant(int64(0), c.typeFor(integerType(64)))
	switch cc {
	case "e", "eq", "z":
		return c.builder.EmitBinOp(ssa.OpEq, subject, zero)
	case "ne", "nz":
		return c.builder.EmitBinOp(ssa.OpNotEq, subject, zero)
	case "l", "lt", "b", "c", "cc", "lo":
		return c.builder.EmitBinOp(ssa.OpLt, subject, zero)
	case "le", "be", "ls":
		return c.builder.EmitBinOp(ssa.OpLtEq, subject, zero)
	case "g", "gt", "a", "hi":
		return c.builder.EmitBinOp(ssa.OpGt, subject, zero)
	case "ge", "ae", "cs", "hs":
		return c.builder.EmitBinOp(ssa.OpGtEq, subject, zero)
	case "s", "mi":
		return c.builder.EmitBinOp(ssa.OpLt, subject, zero)
	case "ns", "pl":
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
	destination := instruction.Results[0].Type.Bits
	if destination == 0 {
		destination = widthOfSSAType(c.typeFor(instruction.Results[0].Type))
	}
	source := c.operandWidth(firstOperand(instruction.Operands), 0)
	if source == 0 {
		source = widthOfSSAType(value.GetType())
	}
	switch instruction.Kind {
	case core.OpSignExtend:
		if constant, ok := ssa.ToConstInst(value); ok {
			if number, ok := signedConstant(constant, source); ok {
				folded := c.builder.EmitConstInst(number)
				if folded != nil {
					c.setResult(instruction, folded)
					folded.SetType(c.signedTypeFor(destination))
				}
				return
			}
		}
		// The intermediate cast is what makes the target C expression sign-extend:
		// (int64_t)(int32_t)x keeps the sign bit, (int64_t)x on a uint32_t does not.
		value = c.castType(value, c.signedTypeFor(source))
		value = c.castType(value, c.signedTypeFor(destination))
		c.setResult(instruction, value)
		value.SetType(c.signedTypeFor(destination))
	case core.OpZeroExtend:
		if constant, ok := ssa.ToConstInst(value); ok {
			if number, ok := unsignedConstant(constant, source); ok {
				folded := c.builder.EmitConstInst(number)
				if folded != nil {
					c.setResult(instruction, folded)
					folded.SetType(c.typeFor(instruction.Results[0].Type))
				}
				return
			}
		}
		value = c.castType(value, c.typeFor(integerType(source)))
		value = c.castType(value, c.typeFor(integerType(destination)))
		c.setResult(instruction, value)
	case core.OpTruncate:
		if constant, ok := ssa.ToConstInst(value); ok {
			if number, ok := unsignedConstant(constant, destination); ok {
				folded := c.builder.EmitConstInst(number)
				if folded != nil {
					c.setResult(instruction, folded)
					folded.SetType(c.typeFor(instruction.Results[0].Type))
				}
				return
			}
		}
		value = c.castType(value, c.typeFor(instruction.Results[0].Type))
		c.setResult(instruction, value)
	default:
		c.setResult(instruction, c.castType(value, c.typeFor(instruction.Results[0].Type)))
	}
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
	// A load from a table of addresses is the address it holds. The target stores
	// its globals that way, so resolving the table entry is what turns a symbolic
	// chain into a nameable object.
	if c.pointerAt != nil && len(instruction.Results) == 1 {
		if address, ok := c.constantAddress(instruction.Operands[0].Value); ok {
			bits := instruction.Results[0].Type.Bits
			if bits == 0 {
				bits = c.facts.Registers.Width
			}
			if value, ok := c.pointerAt(address, bits); ok {
				if constant := c.builder.EmitConstInst(int64(value)); constant != nil {
					constant.SetType(c.typeFor(instruction.Results[0].Type))
					c.setResult(instruction, constant)
					return
				}
			}
		}
	}
	key := valueKey(c.defs, instruction.Operands[0].Value)
	name := c.memoryName(key)
	variable := c.memoryVariable(name)
	if variable == nil {
		return
	}
	// A store through another address that may alias this slot invalidates the
	// cached value, so the load stays a reload instead of folding to a constant.
	address := instruction.Operands[0]
	markerPrefix := "load:"
	if c.classifyMemorySlot(key, address) != memorySlotFrame {
		// A dereference through a pointer or a global address reads memory the
		// caller can observe, and SSA preparation must keep it as a real value.
		markerPrefix = "reload:"
	}
	if !c.memoryValidAt(key) {
		c.setResult(instruction, c.loadMarker(instruction, address.Value, markerPrefix))
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
	c.setResult(instruction, c.loadMarker(instruction, address.Value, markerPrefix))
}

func (c *converter) emitStore(instruction core.Instruction) {
	if len(instruction.Operands) < 2 {
		return
	}
	address := instruction.Operands[0]
	key := valueKey(c.defs, address.Value)
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
	c.recordMemoryStore(key, address)
	if c.classifyMemorySlot(key, address) != memorySlotFrame {
		c.emitStoreMarker(instruction, address, value)
	}
	c.markHeaderStore("mem:" + key)
}

func (c *converter) loadMarker(instruction core.Instruction, addressID core.ValueID, prefix string) ssa.Value {
	address := c.values[addressID]
	markerText := prefix + "unknown"
	if address != nil {
		markerText = fmt.Sprintf("%s%d", prefix, address.GetId())
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
	if merge := c.mergePhis[c.current]; merge != nil {
		if phi := merge[key]; phi != nil {
			return phi
		}
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
		view := c.registerView(operand.Name, operand.Type.Bits)
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
	// Mark the values that derive from the stack pointer, so a call can tell a
	// pushed argument from an ordinary spill.
	if c.isStackDerived(instruction) {
		c.spDerived[value] = true
	}
	if c.isFramePointerDerived(instruction) {
		c.frameDerived[value] = true
	}
	// A copy of a frame register is still that register: the prologue saves the
	// frame pointer through a copy, and that value is not an argument.
	if instruction.Kind == core.OpCopy && len(instruction.Operands) > 0 {
		if source, ok := c.values[instruction.Operands[0].Value]; ok && c.frameValues[source] {
			c.frameValues[value] = true
		}
	}
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
	// A write narrower than 32 bits merges with the value already in the
	// canonical variable, because x86 leaves the upper bits untouched. A
	// 32-bit write zero-extends instead, and on a 16-bit target the 16-bit
	// register is the whole variable, so neither is partial. A view with no
	// canonical width (the flags pseudo register) is also a whole variable.
	if v.canonicalBits == 0 {
		return false
	}
	limit := uint16(32)
	if v.canonicalBits < limit {
		limit = v.canonicalBits
	}
	return v.width > 0 && v.width < limit
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
//
// The rule follows the register model, not a table of every name the parsers
// might emit. x86 splits one register into 8, 16 and 32-bit views; AArch64 shares
// a variable between Wn and Xn; every other target treats a name as one register.
func registerViewFor(name string, bits uint16, model core.RegisterModel) registerView {
	lower := strings.ToLower(name)
	if lower == "" {
		return registerView{}
	}
	if model.Flags != "" && lower == model.Flags {
		// A flag word is not an integer register, so its width is declared rather
		// than derived from the machine width. x86 keeps one variable for the
		// flags word across the 32-bit and 64-bit spellings.
		//
		// A flag write replaces the whole word even when the word is narrower than
		// a general register (s390x cc is two bits). Reporting a bit width as a
		// canonical width would turn every flag write into a partial write that
		// merges with the previous value, which erases the comparison the flags
		// value carries.
		width := model.FlagsWidth
		if width == 0 {
			width = model.Width
		}
		canonical := lower
		view := registerView{canonical: canonical, width: width}
		if model.Naming == core.RegisterNamingX86 {
			// The x86 flags word is one 64-bit variable in every mode, so the
			// test that pins the name also pins the canonical width.
			view.canonical = "rflags"
			view.canonicalBits = width
		}
		return view
	}
	switch model.Naming {
	case core.RegisterNamingX86:
		// The canonical variable is as wide as the target's registers. A 32-bit
		// target keeps eax and ax in one 32-bit variable, so a read must not
		// extract from a 64-bit one and lose the constant.
		return x86RegisterView(lower, bits, model.Width)
	case core.RegisterNamingAArch64:
		return aarch64RegisterView(lower, bits, model.Width)
	default:
		width := bits
		if width == 0 {
			width = model.Width
		}
		return registerView{canonical: lower, width: width, canonicalBits: width}
	}
}

// x86RegisterView maps one x86 register name, including the partial views, to the
// 64-bit variable that holds it.
func x86RegisterView(lower string, bits, canonicalBits uint16) registerView {
	if canonicalBits == 0 {
		canonicalBits = 64
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
			canonicalBits: canonicalBits,
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
		// The flags pseudo register is one opaque word regardless of the
		// machine width, so a 32-bit write must not become a partial write.
		return registerView{canonical: "rflags", width: 64, canonicalBits: 64}
	default:
		return registerView{canonical: lower, width: bits}
	}
}

// aarch64RegisterView shares one variable between a W register and its X
// counterpart, and between SP and WSP.
func aarch64RegisterView(lower string, bits, defaultBits uint16) registerView {
	switch lower {
	case "wsp", "sp":
		return registerView{canonical: "sp", width: 64, canonicalBits: 64}
	case "wzr", "xzr":
		return registerView{canonical: "xzr", width: 64, canonicalBits: 64}
	case "fp":
		return registerView{canonical: "x29", width: 64, canonicalBits: 64}
	case "lr":
		return registerView{canonical: "x30", width: 64, canonicalBits: 64}
	}
	digits, ok := registerDigits(lower[1:])
	if !ok || len(lower) < 2 || (lower[0] != 'w' && lower[0] != 'x') {
		width := bits
		if width == 0 {
			width = defaultBits
		}
		return registerView{canonical: lower, width: width, canonicalBits: width}
	}
	width := uint16(64)
	if lower[0] == 'w' {
		width = 32
	}
	return registerView{canonical: "x" + digits, width: width, canonicalBits: 64}
}

func generalRegisterFamily(name string) (string, bool) {
	// AArch64: Wn shares variable Xn; SP and WSP are one variable.
	if len(name) >= 2 && (name[0] == 'w' || name[0] == 'x') {
		if digits, ok := registerDigits(name[1:]); ok {
			return "x" + digits, true
		}
	}
	switch name {
	case "wsp", "sp":
		return "sp", true
	case "wzr", "xzr":
		return "xzr", true
	case "fp":
		return "x29", true
	case "lr":
		return "x30", true
	}
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

func registerDigits(text string) (string, bool) {
	if text == "" {
		return "", false
	}
	for _, digit := range text {
		if digit < '0' || digit > '9' {
			return "", false
		}
	}
	return text, true
}

func generalRegisterWidth(name string) uint16 {
	switch name {
	case "eax", "ebx", "ecx", "edx", "esi", "edi", "esp", "ebp":
		return 32
	case "ax", "bx", "cx", "dx", "si", "di", "sp", "bp":
		return 16
	case "al", "bl", "cl", "dl", "ah", "bh", "ch", "dh":
		return 8
	}
	// AArch64 W registers are the 32-bit view of an X register.
	if len(name) >= 2 && name[0] == 'w' && name != "wsp" {
		if _, ok := registerDigits(name[1:]); ok {
			return 32
		}
	}
	if name == "wsp" {
		return 32
	}
	if name == "xzr" || name == "sp" || name[0] == 'x' {
		if name == "xzr" || name == "sp" {
			return 64
		}
		if _, ok := registerDigits(name[1:]); ok {
			return 64
		}
	}
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

// rememberStackStore records the value of one store so a following cdecl call can
// pass it as an argument. Only the nearest run matters, so a branch or a jump
// forgets it.
func (c *converter) rememberStackStore(instruction core.Instruction) {
	if !c.facts.Registers.StackArguments || len(instruction.Operands) < 2 {
		return
	}
	value := c.operandValue(instruction.Operands[1])
	if value == nil {
		return
	}
	c.stackStores = append(c.stackStores, value)
}

// isFrameRegister reports whether one register belongs to the call frame.
func (c *converter) isFrameRegister(name string) bool {
	if name == "" {
		return false
	}
	frame := c.facts.Frame
	for _, candidate := range []string{frame.StackPointer, frame.FramePointer, frame.ReturnAddress} {
		if candidate != "" && c.registerView(candidate, 0).canonical == name {
			return true
		}
	}
	return false
}

// isStackDerived reports whether one binary operation combines a stack-derived
// value with a constant, which is how a push computes its address.
func (c *converter) isStackDerived(instruction core.Instruction) bool {
	for _, operand := range instruction.Operands {
		if operand.Kind != core.OperandValue {
			continue
		}
		if value, ok := c.values[operand.Value]; ok && c.spDerived[value] {
			return true
		}
	}
	return false
}

// isFramePointerDerived reports whether one operation builds an address from the
// frame pointer in the same way. Only pointer arithmetic propagates the
// property: a value loaded from a frame slot is data, not an address.
func (c *converter) isFramePointerDerived(instruction core.Instruction) bool {
	switch instruction.Kind {
	case core.OpAdd, core.OpSub, core.OpShl, core.OpCopy, core.OpAnd:
	default:
		return false
	}
	for _, operand := range instruction.Operands {
		if operand.Kind != core.OperandValue {
			continue
		}
		if value, ok := c.values[operand.Value]; ok && (c.spDerived[value] || c.frameDerived[value]) {
			return true
		}
	}
	return false
}

// constantAddress resolves one operand to a constant image address.
func (c *converter) constantAddress(id core.ValueID) (uint64, bool) {
	value, ok := c.values[id]
	if !ok || value == nil {
		return 0, false
	}
	constant, ok := ssa.ToConstInst(value)
	if !ok {
		return 0, false
	}
	number, ok := constInteger(constant)
	if !ok || number < 0 {
		return 0, false
	}
	return uint64(number), true
}

// flagInfoOf looks up the comparison a value carries. A narrow flag register is
// read through a type cast, and the comparison is recorded under the value the
// cast reads, so the search follows the cast.
func (c *converter) flagInfoOf(value ssa.Value) (flagInfo, bool) {
	for depth := 0; depth < 4 && value != nil; depth++ {
		if info, ok := c.flags[core.ValueID(value.GetId())]; ok {
			return info, true
		}
		cast, ok := ssa.ToTypeCast(value)
		if !ok || cast == nil {
			return flagInfo{}, false
		}
		inner, ok := cast.GetValueById(cast.Value)
		if !ok || inner == nil {
			return flagInfo{}, false
		}
		value = inner
	}
	return flagInfo{}, false
}

// emitPhi builds the merge value a split conditional move left behind. The
// operands are the values of the incoming arms, in predecessor order.
func (c *converter) emitPhi(instruction core.Instruction) {
	if len(instruction.Results) == 0 || len(instruction.Operands) == 0 {
		return
	}
	block := c.builder.CurrentBlock
	if block == nil {
		return
	}
	phi := ssa.NewPhi(block, fmt.Sprintf("phi_%d", instruction.Results[0].ID))
	if phi == nil {
		return
	}
	for _, operand := range instruction.Operands {
		value := c.operandValue(operand)
		if value == nil {
			continue
		}
		phi.Edge = append(phi.Edge, value.GetId())
		value.AddUser(phi)
	}
	if typ := c.typeFor(instruction.Results[0].Type); typ != nil {
		phi.SetType(typ)
	}
	block.Phis = append(block.Phis, phi.GetId())
	c.setResult(instruction, phi)
}
