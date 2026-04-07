// Package lowering converts selected SSA functions into PIR form.
//
// The lowerer walks each SSA basic block, translating SSA instructions into
// the compact PIR opcode set. Values are mapped to PIR virtual registers
// via an incrementing allocator. Calls that cannot be resolved as internal
// PIR calls become host-calls that bridge back to the native runtime via
// InvokeContext.
package lowering

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/callframe"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/pir"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/region"
)

// LowerRegion lowers a set of region candidates to a PIR Region.
func LowerRegion(candidates []region.Candidate) (*pir.Region, error) {
	var pirFuncs []*pir.Function
	hostSyms := make([]string, 0)
	hostSymIndex := make(map[string]int)
	internalCallIndex := make(map[string]int, len(candidates))
	for index, candidate := range candidates {
		internalCallIndex[candidate.Name] = index
	}

	for _, c := range candidates {
		pf, _, err := lowerFunctionWithContext(c.Func, internalCallIndex, hostSymIndex, &hostSyms)
		if err != nil {
			return nil, fmt.Errorf("lowering %s: %w", c.Name, err)
		}
		pirFuncs = append(pirFuncs, pf)
	}

	return &pir.Region{
		Functions:   pirFuncs,
		HostSymbols: dedup(hostSyms),
	}, nil
}

// LowerFunction lowers a single SSA function to PIR.
// Returns the PIR function and a list of host-call symbols referenced.
func LowerFunction(fn *ssa.Function) (*pir.Function, []string, error) {
	l := newLowerer(fn, nil, make(map[string]int), nil)
	if err := l.lower(); err != nil {
		return nil, nil, err
	}
	return l.pirFunc, l.hostSymbols, nil
}

func lowerFunctionWithContext(
	fn *ssa.Function,
	internalCallIndex map[string]int,
	hostSymbolIndex map[string]int,
	sharedHostSymbols *[]string,
) (*pir.Function, []string, error) {
	l := newLowerer(fn, internalCallIndex, hostSymbolIndex, sharedHostSymbols)
	if err := l.lower(); err != nil {
		return nil, nil, err
	}
	return l.pirFunc, l.hostSymbols, nil
}

// ---------------------------------------------------------------------------
// lowerer – per-function lowering state
// ---------------------------------------------------------------------------

type lowerer struct {
	fn      *ssa.Function
	pirFunc *pir.Function

	// SSA value ID → PIR register index
	regMap  map[int64]int
	nextReg int

	// SSA block ID → PIR block index
	blockMap map[int64]int

	hostSymbols       []string
	internalCallIndex map[string]int
	hostSymbolIndex   map[string]int
	sharedHostSymbols *[]string
}

func newLowerer(
	fn *ssa.Function,
	internalCallIndex map[string]int,
	hostSymbolIndex map[string]int,
	sharedHostSymbols *[]string,
) *lowerer {
	return &lowerer{
		fn:                fn,
		regMap:            make(map[int64]int),
		blockMap:          make(map[int64]int),
		internalCallIndex: internalCallIndex,
		hostSymbolIndex:   hostSymbolIndex,
		sharedHostSymbols: sharedHostSymbols,
	}
}

func (l *lowerer) allocReg(ssaID int64) int {
	if r, ok := l.regMap[ssaID]; ok {
		return r
	}
	r := l.nextReg
	l.nextReg++
	l.regMap[ssaID] = r
	return r
}

func (l *lowerer) reg(ssaID int64) int {
	if r, ok := l.regMap[ssaID]; ok {
		return r
	}
	return l.allocReg(ssaID)
}

func (l *lowerer) blockIdx(ssaBlockID int64) int {
	if idx, ok := l.blockMap[ssaBlockID]; ok {
		return idx
	}
	// Not found - this shouldn't happen if blocks are pre-mapped
	return -1
}

// lower walks the function and produces PIR.
func (l *lowerer) lower() error {
	fn := l.fn
	name := fn.GetName()

	// Pre-allocate block map
	for i, blockID := range fn.Blocks {
		l.blockMap[blockID] = i
	}

	// Pre-allocate argument registers using the real callable frame layout
	// (params + parameter members + freevalues), so protected wrappers can
	// reuse the existing InvokeContext ABI without re-packing arguments.
	frameInputs := callframe.OrderedCallFrameInputs(fn)
	numArgs := len(frameInputs)
	for _, input := range frameInputs {
		if input.Value == nil {
			continue
		}
		l.allocReg(input.Value.GetId())
	}

	pirFunc := &pir.Function{
		Name:       name,
		SSAID:      fn.GetId(),
		NumArgs:    numArgs,
		EntryBlock: 0,
		Blocks:     make([]pir.Block, len(fn.Blocks)),
	}
	l.pirFunc = pirFunc

	// Create blocks with predecessor/successor info
	for i, blockID := range fn.Blocks {
		blockVal, ok := fn.GetValueById(blockID)
		if !ok {
			return fmt.Errorf("block %d not found", blockID)
		}
		block, ok := blockVal.(*ssa.BasicBlock)
		if !ok {
			return fmt.Errorf("value %d is not a BasicBlock", blockID)
		}

		pirBlock := &pirFunc.Blocks[i]
		pirBlock.Index = i

		// Map predecessors and successors
		for _, p := range block.Preds {
			if idx, ok := l.blockMap[p]; ok {
				pirBlock.Preds = append(pirBlock.Preds, idx)
			}
		}
		for _, s := range block.Succs {
			if idx, ok := l.blockMap[s]; ok {
				pirBlock.Succs = append(pirBlock.Succs, idx)
			}
		}

		// Emit argument loads in the entry block
		if blockID == fn.EnterBlock {
			for argIdx, input := range frameInputs {
				if input.Value == nil {
					continue
				}
				dst := l.reg(input.Value.GetId())
				pirBlock.Insts = append(pirBlock.Insts, pir.Inst{
					Op:  pir.OpArg,
					Dst: dst,
					Imm: int64(argIdx),
				})
			}
		}

		// Lower phi nodes
		for _, phiID := range block.Phis {
			phiVal, ok := fn.GetValueById(phiID)
			if !ok {
				continue
			}
			phi, ok := phiVal.(*ssa.Phi)
			if !ok {
				continue
			}
			if err := l.lowerPhi(pirBlock, phi); err != nil {
				return err
			}
		}

		// Lower instructions
		for _, instID := range block.Insts {
			instObj, ok := fn.GetInstructionById(instID)
			if !ok || instObj == nil {
				continue
			}
			if instObj.IsLazy() {
				instObj = instObj.Self()
			}
			if instObj == nil {
				continue
			}
			if err := l.lowerInstruction(pirBlock, instObj); err != nil {
				return err
			}
		}
	}

	pirFunc.NumRegs = l.nextReg
	return nil
}

// ---------------------------------------------------------------------------
// Instruction lowering
// ---------------------------------------------------------------------------

func (l *lowerer) lowerInstruction(block *pir.Block, inst ssa.Instruction) error {
	switch op := inst.(type) {
	case *ssa.BinOp:
		return l.lowerBinOp(block, op)
	case *ssa.ConstInst:
		return l.lowerConst(block, op)
	case *ssa.Call:
		return l.lowerCall(block, op)
	case *ssa.Return:
		return l.lowerReturn(block, op)
	case *ssa.Jump:
		return l.lowerJump(block, op)
	case *ssa.If:
		return l.lowerIf(block, op)
	case *ssa.Loop:
		return l.lowerLoop(block, op)
	case *ssa.SideEffect:
		return l.lowerSideEffect(block, op)
	default:
		// Unsupported instructions are skipped with a nop.
		return nil
	}
}

func (l *lowerer) lowerBinOp(block *pir.Block, op *ssa.BinOp) error {
	dst := l.allocReg(op.GetId())
	lhs := l.reg(op.X)
	rhs := l.reg(op.Y)

	pirOp, ok := ssaBinOpToPIR[op.Op]
	if !ok {
		return fmt.Errorf("unsupported BinOp %s", op.Op)
	}

	block.Insts = append(block.Insts, pir.Inst{
		Op:  pirOp,
		Dst: dst,
		Src: [2]int{lhs, rhs},
	})
	return nil
}

var ssaBinOpToPIR = map[ssa.BinaryOpcode]pir.Opcode{
	ssa.OpAdd:      pir.OpAdd,
	ssa.OpSub:      pir.OpSub,
	ssa.OpMul:      pir.OpMul,
	ssa.OpDiv:      pir.OpDiv,
	ssa.OpMod:      pir.OpMod,
	ssa.OpAnd:      pir.OpAnd,
	ssa.OpOr:       pir.OpOr,
	ssa.OpXor:      pir.OpXor,
	ssa.OpShl:      pir.OpShl,
	ssa.OpShr:      pir.OpShr,
	ssa.OpAndNot:   pir.OpAndNot,
	ssa.OpEq:       pir.OpEq,
	ssa.OpNotEq:    pir.OpNeq,
	ssa.OpLt:       pir.OpLt,
	ssa.OpLtEq:     pir.OpLe,
	ssa.OpGt:       pir.OpGt,
	ssa.OpGtEq:     pir.OpGe,
	ssa.OpLogicAnd: pir.OpLogicAnd,
	ssa.OpLogicOr:  pir.OpLogicOr,
}

func (l *lowerer) lowerConst(block *pir.Block, inst *ssa.ConstInst) error {
	dst := l.allocReg(inst.GetId())
	var imm int64
	if inst.GetRawValue() == nil {
		imm = 0
	} else if inst.IsNumber() {
		imm = int64(inst.Number())
	} else if inst.IsBoolean() {
		if inst.Boolean() {
			imm = 1
		}
	} else {
		// Strings and other types become 0 in PIR MVP (host-call for string ops).
		imm = 0
	}
	block.Insts = append(block.Insts, pir.Inst{
		Op:  pir.OpConst,
		Dst: dst,
		Imm: imm,
	})
	return nil
}

func (l *lowerer) lowerCall(block *pir.Block, inst *ssa.Call) error {
	dst := l.allocReg(inst.GetId())

	// Resolve callee
	var calleeName string
	if program := l.fn.GetProgram(); program != nil {
		if direct, ok := callframe.ResolveDirectCallee(program, l.fn, inst); ok && direct != nil {
			calleeName = direct.GetName()
		}
	}
	if methodVal, ok := l.fn.GetValueById(inst.Method); ok {
		if fn, ok := ssa.ToFunction(methodVal); ok {
			calleeName = fn.GetName()
		}
		if calleeName == "" {
			calleeName = resolveProtectedCalleeName(l.fn, methodVal)
		}
	}

	// Collect argument registers
	args := make([]int, 0, len(inst.Args))
	for _, argID := range inst.Args {
		args = append(args, l.reg(argID))
	}

	calleeReg := l.reg(inst.Method)
	if calleeName == "" {
		return fmt.Errorf("unsupported dynamic call %d in protected lowering", inst.GetId())
	}

	if index, ok := l.internalCallIndex[calleeName]; ok {
		block.Insts = append(block.Insts, pir.Inst{
			Op:  pir.OpConst,
			Dst: calleeReg,
			Imm: int64(index),
		})
		block.Insts = append(block.Insts, pir.Inst{
			Op:       pir.OpCall,
			Dst:      dst,
			Src:      [2]int{calleeReg, 0},
			CallArgs: args,
		})
		return nil
	}

	hostIndex := l.hostSymbolSlot(calleeName)
	block.Insts = append(block.Insts, pir.Inst{
		Op:  pir.OpConst,
		Dst: calleeReg,
		Imm: int64(hostIndex),
	})
	block.Insts = append(block.Insts, pir.Inst{
		Op:       pir.OpHostCall,
		Dst:      dst,
		Src:      [2]int{calleeReg, 0},
		CallArgs: args,
	})
	return nil
}

func (l *lowerer) hostSymbolSlot(name string) int {
	if idx, ok := l.hostSymbolIndex[name]; ok {
		return idx
	}
	idx := len(l.hostSymbolIndex)
	l.hostSymbolIndex[name] = idx
	if l.sharedHostSymbols != nil {
		*l.sharedHostSymbols = append(*l.sharedHostSymbols, name)
	}
	l.hostSymbols = append(l.hostSymbols, name)
	return idx
}

func resolveProtectedCalleeName(fn *ssa.Function, value ssa.Value) string {
	if value == nil {
		return ""
	}
	if mc, ok := value.(ssa.MemberCall); ok && mc.IsMember() {
		objName := resolveProtectedValueName(fn, mc.GetObject())
		keyName := resolveProtectedValueName(fn, mc.GetKey())
		switch {
		case objName != "" && keyName != "":
			return objName + "." + keyName
		case keyName != "":
			return keyName
		}
	}
	return resolveProtectedValueName(fn, value)
}

func resolveProtectedValueName(fn *ssa.Function, value ssa.Value) string {
	if value == nil {
		return ""
	}
	if inst, ok := value.(ssa.Instruction); ok && inst.IsLazy() {
		if self, ok := inst.Self().(ssa.Value); ok && self != nil {
			value = self
		}
	}
	if fnValue, ok := ssa.ToFunction(value); ok && fnValue != nil {
		return strings.Trim(strings.TrimSpace(fnValue.GetName()), "\"")
	}
	if cinst, ok := ssa.ToConstInst(value); ok && cinst != nil {
		return strings.Trim(cinst.String(), "\"")
	}
	return strings.Trim(strings.TrimSpace(value.GetName()), "\"")
}

func (l *lowerer) lowerReturn(block *pir.Block, inst *ssa.Return) error {
	src := -1
	if len(inst.Results) > 0 {
		src = l.reg(inst.Results[0])
	}
	block.Insts = append(block.Insts, pir.Inst{
		Op:  pir.OpReturn,
		Dst: -1,
		Src: [2]int{src, 0},
	})
	return nil
}

func (l *lowerer) lowerJump(block *pir.Block, inst *ssa.Jump) error {
	target := l.blockIdx(inst.To)
	block.Insts = append(block.Insts, pir.Inst{
		Op:    pir.OpJump,
		Dst:   -1,
		Block: target,
	})
	return nil
}

func (l *lowerer) lowerIf(block *pir.Block, inst *ssa.If) error {
	cond := l.reg(inst.Cond)
	trueBlk := l.blockIdx(inst.True)
	falseBlk := l.blockIdx(inst.False)
	block.Insts = append(block.Insts, pir.Inst{
		Op:       pir.OpBranch,
		Dst:      -1,
		Src:      [2]int{cond, 0},
		Block:    trueBlk,
		AuxBlock: falseBlk,
	})
	return nil
}

func (l *lowerer) lowerLoop(block *pir.Block, inst *ssa.Loop) error {
	cond := l.reg(inst.Cond)
	bodyBlk := l.blockIdx(inst.Body)
	exitBlk := l.blockIdx(inst.Exit)
	block.Insts = append(block.Insts, pir.Inst{
		Op:       pir.OpBranch,
		Dst:      -1,
		Src:      [2]int{cond, 0},
		Block:    bodyBlk,
		AuxBlock: exitBlk,
	})
	return nil
}

func (l *lowerer) lowerSideEffect(block *pir.Block, inst *ssa.SideEffect) error {
	// SideEffect in SSA represents a value modification from a call.
	// In PIR, we just alias the source register.
	dst := l.allocReg(inst.GetId())
	src := l.reg(inst.Value)
	// Emit a nop move: const 0 then add with src to copy value.
	// Simpler: just alias by mapping dst to src's register.
	l.regMap[inst.GetId()] = src
	_ = dst
	return nil
}

func (l *lowerer) lowerPhi(block *pir.Block, phi *ssa.Phi) error {
	dst := l.allocReg(phi.GetId())

	// Build phi edges from the SSA phi.
	// phi.Edge contains value IDs, one per predecessor.
	// phi.CFGEntryBasicBlock is the block this phi belongs to.
	// The predecessor order matches block.Preds.
	var edges []pir.PhiEdge
	parentBlockVal, ok := l.fn.GetValueById(phi.CFGEntryBasicBlock)
	if !ok {
		return fmt.Errorf("phi parent block %d not found", phi.CFGEntryBasicBlock)
	}
	parentBlock, ok := parentBlockVal.(*ssa.BasicBlock)
	if !ok {
		return fmt.Errorf("phi parent %d is not a BasicBlock", phi.CFGEntryBasicBlock)
	}

	for i, edgeValID := range phi.Edge {
		var predBlockIdx int
		if i < len(parentBlock.Preds) {
			predBlockIdx = l.blockIdx(parentBlock.Preds[i])
		} else {
			predBlockIdx = -1
		}
		edges = append(edges, pir.PhiEdge{
			Block: predBlockIdx,
			Reg:   l.reg(edgeValID),
		})
	}

	block.Insts = append(block.Insts, pir.Inst{
		Op:    pir.OpPhi,
		Dst:   dst,
		Edges: edges,
	})
	return nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func dedup(ss []string) []string {
	seen := make(map[string]struct{}, len(ss))
	var result []string
	for _, s := range ss {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			result = append(result, s)
		}
	}
	return result
}
