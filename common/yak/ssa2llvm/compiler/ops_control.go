package compiler

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/go-llvm"
)

// compileJump creates an unconditional branch to the target block.
func (c *Compiler) compileJump(inst *ssa.Jump) error {
	targetBlock, ok := c.Blocks[inst.To]
	if !ok {
		return fmt.Errorf("compileJump: target block %d not found", inst.To)
	}
	c.Builder.CreateBr(targetBlock)
	return nil
}

// compileIf creates a conditional branch.
// It handles conversion of the condition value to i1 if necessary.
func (c *Compiler) compileIf(inst *ssa.If) error {
	condVal, err := c.getValue(inst, inst.Cond)
	if err != nil {
		return err
	}

	// Check type of condition. If it's not i1, compare it with 0 to get i1.
	// We assume values are i64 for now as per Phase 1/2 assumption.
	if condVal.Type().IntTypeWidth() != 1 {
		// Create ICmp NE (Not Equal) 0
		zero := llvm.ConstInt(condVal.Type(), 0, false)
		condVal = c.Builder.CreateICmp(llvm.IntPredicate(llvm.IntNE), condVal, zero, "if_cond")
	}

	trueBlock, ok := c.Blocks[inst.True]
	if !ok {
		return fmt.Errorf("compileIf: true block %d not found", inst.True)
	}

	falseBlock, ok := c.Blocks[inst.False]
	if !ok {
		return fmt.Errorf("compileIf: false block %d not found", inst.False)
	}

	c.Builder.CreateCondBr(condVal, trueBlock, falseBlock)
	return nil
}

// compileLoop creates a conditional branch for loop headers.
// It branches to Body when the condition is true, otherwise to Exit.
func (c *Compiler) compileLoop(inst *ssa.Loop) error {
	condVal, err := c.getValue(inst, inst.Cond)
	if err != nil {
		return err
	}

	if condVal.Type().IntTypeWidth() != 1 {
		zero := llvm.ConstInt(condVal.Type(), 0, false)
		condVal = c.Builder.CreateICmp(llvm.IntPredicate(llvm.IntNE), condVal, zero, "loop_cond")
	}

	bodyBlock, ok := c.Blocks[inst.Body]
	if !ok {
		return fmt.Errorf("compileLoop: body block %d not found", inst.Body)
	}
	exitBlock, ok := c.Blocks[inst.Exit]
	if !ok {
		return fmt.Errorf("compileLoop: exit block %d not found", inst.Exit)
	}

	c.Builder.CreateCondBr(condVal, bodyBlock, exitBlock)
	return nil
}

// compilePhi creates the PHI node but DOES NOT populate incoming values.
// This is Pass 1 of Phi handling.
func (c *Compiler) compilePhi(inst *ssa.Phi) error {
	phiType := c.inferPhiType(inst)
	phiNode := c.Builder.CreatePHI(phiType, fmt.Sprintf("phi_%d", inst.GetId()))
	c.Values[inst.GetId()] = phiNode
	return nil
}

func (c *Compiler) llvmValueTypeForSSA(t ssa.Type) llvm.Type {
	if t == nil {
		return c.LLVMCtx.Int64Type()
	}

	switch t.GetTypeKind() {
	case ssa.ObjectTypeKind, ssa.SliceTypeKind, ssa.MapTypeKind, ssa.PointerKind, ssa.StringTypeKind:
		return llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	case ssa.AnyTypeKind:
		return c.LLVMCtx.Int64Type()
	case ssa.StructTypeKind:
		base := c.TypeConverter.ConvertType(t)
		return llvm.PointerType(base, 0)
	default:
		return c.LLVMCtx.Int64Type()
	}
}

func (c *Compiler) inferPhiType(inst *ssa.Phi) llvm.Type {
	fn := inst.GetFunc()
	if fn != nil {
		for _, edgeID := range inst.Edge {
			edgeVal, ok := fn.GetValueById(edgeID)
			if !ok || edgeVal == nil {
				continue
			}
			if _, isUndef := edgeVal.(*ssa.Undefined); isUndef {
				continue
			}
			if c.ssaValueIsPointer(edgeVal, fn) {
				return llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
			}
		}
	}
	return c.llvmValueTypeForSSA(inst.GetType())
}

func (c *Compiler) ssaValueIsPointer(val ssa.Value, fn *ssa.Function) bool {
	if val == nil {
		return false
	}

	if t := val.GetType(); t != nil {
		switch t.GetTypeKind() {
		case ssa.ObjectTypeKind, ssa.SliceTypeKind, ssa.MapTypeKind, ssa.PointerKind, ssa.StringTypeKind:
			return true
		case ssa.StructTypeKind:
			return true
		}
	}

	switch v := val.(type) {
	case *ssa.ConstInst:
		return v.IsString()
	case *ssa.Make:
		return true
	case *ssa.Call:
		return c.callReturnsPointer(v, fn)
	}

	return false
}

func (c *Compiler) callReturnsPointer(call *ssa.Call, fn *ssa.Function) bool {
	if call == nil {
		return false
	}

	calleeName := ""
	if fn != nil {
		if calleeVal, ok := fn.GetValueById(call.Method); ok && calleeVal != nil {
			calleeName = calleeVal.GetName()
			if ssaFn, ok := ssa.ToFunction(calleeVal); ok {
				calleeName = ssaFn.GetName()
			}
		}
	}
	if calleeName == "" {
		calleeName = fmt.Sprintf("func_%d", call.Method)
	}

	switch calleeName {
	case "getObject":
		return true
	default:
		return false
	}
}

// resolvePhi populates the incoming values for a PHI node.
// This is Pass 2 of Phi handling, called after all blocks are generated.
func (c *Compiler) resolvePhi(inst *ssa.Phi) error {
	phiVal, ok := c.Values[inst.GetId()]
	if !ok {
		return fmt.Errorf("resolvePhi: phi value %d not found", inst.GetId())
	}

	block := inst.GetBlock()
	if block == nil {
		return fmt.Errorf("resolvePhi: phi %d has no block", inst.GetId())
	}
	blockID := block.GetId()

	fn := inst.GetFunc()
	if fn == nil {
		return fmt.Errorf("resolvePhi: function for phi %d is nil", inst.GetId())
	}

	blockVal, ok := fn.GetValueById(blockID)
	if !ok {
		return fmt.Errorf("resolvePhi: block %d not found in function", blockID)
	}

	bbSsa, ok := blockVal.(*ssa.BasicBlock)
	if !ok {
		return fmt.Errorf("resolvePhi: value %d is not *ssa.BasicBlock", blockID)
	}

	edges := inst.Edge
	preds := bbSsa.Preds

	var incomingVals []llvm.Value
	var incomingBlocks []llvm.BasicBlock

	// Handle cases where edges don't match preds exactly
	// YakSSA may have Undefined edges that don't correspond to actual predecessors
	if len(edges) != len(preds) {
		// Try to match edges to preds by filtering out Undefined values
		for i, edgeValID := range edges {
			edgeObj, ok := fn.GetValueById(edgeValID)
			if !ok {
				continue
			}

			// Skip Undefined values
			if _, isUndef := edgeObj.(*ssa.Undefined); isUndef {
				continue
			}

			val, err := c.getValue(inst, edgeValID)
			if err != nil {
				return err
			}

			// Use the corresponding predecessor if available
			if i < len(preds) {
				predBlockID := preds[i]
				blk, ok := c.Blocks[predBlockID]
				if ok {
					incomingVals = append(incomingVals, val)
					incomingBlocks = append(incomingBlocks, blk)
				}
			}
		}

		// If we couldn't match any edges, use a simple strategy:
		// replicate the first valid edge value for all predecessors
		if len(incomingVals) == 0 {
			for _, edgeValID := range edges {
				val, err := c.getValue(inst, edgeValID)
				if err == nil {
					// Use this value for all predecessors
					for _, predBlockID := range preds {
						blk, ok := c.Blocks[predBlockID]
						if ok {
							incomingVals = append(incomingVals, val)
							incomingBlocks = append(incomingBlocks, blk)
						}
					}
					break
				}
			}
		}
	} else {
		// Normal case: edges match preds
		for i, edgeValID := range edges {
			val, err := c.getValue(inst, edgeValID)
			if err != nil {
				return err
			}

			predBlockID := preds[i]
			blk, ok := c.Blocks[predBlockID]
			if !ok {
				return fmt.Errorf("resolvePhi: incoming block %d not found", predBlockID)
			}

			incomingVals = append(incomingVals, val)
			incomingBlocks = append(incomingBlocks, blk)
		}
	}

	if len(incomingVals) > 0 {
		phiVal.AddIncoming(incomingVals, incomingBlocks)
	}

	return nil
}
