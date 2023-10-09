package pass

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
)

const BCTag ssa.ErrorTag = "BlockCondition"

func init() {
	RegisterFunctionPass(&BlockCondition{})
}

// block condition
type BlockCondition struct {
	edge   Edge
	Finish map[*ssa.BasicBlock]struct{}
}

// map to -> from -> condition
type Edge map[*ssa.BasicBlock]map[*ssa.BasicBlock]ssa.Value

func (s *BlockCondition) RunOnFunction(fun *ssa.Function) {
	s.edge = make(Edge)
	s.Finish = make(map[*ssa.BasicBlock]struct{})
	newEdge := func(to, from *ssa.BasicBlock, condition ssa.Value) {
		fromTable, ok := s.edge[to]
		if !ok {
			fromTable = make(map[*ssa.BasicBlock]ssa.Value)
		}
		fromTable[from] = condition
		s.edge[to] = fromTable
	}

	handleIfEdge := func(i *ssa.If) {
		from := i.Block
		newEdge(i.True, from, i.Cond)
		newEdge(i.False, from, newUnOp(ssa.OpNot, i.Cond, i.Block))
	}
	handleLoopEdge := func(l *ssa.Loop) {
		canReach := func() bool {
			if l.Key == nil || l.Init == nil || l.Cond == nil {
				return true
			}
			cond, ok := l.Cond.(*ssa.BinOp)
			if !ok {
				return true
			}
			var x, y ssa.Value

			if l.Key == cond.X {
				x = l.Init
				y = cond.Y
			} else {
				x = cond.X
				y = l.Init
			}
			canReach := newBinOp(cond.Op, x, y, cond.Block)
			can, ok := canReach.(*ssa.Const)
			if ok && can.IsBoolean() {
				return can.Boolean()
			}
			return true
		}
		from := l.Block
		if !canReach() {
			newEdge(l.Body, from, ssa.NewConst(false))
			newEdge(l.Exit, from, ssa.NewConst(true))
		} else {
			newEdge(l.Body, from, l.Cond)
			newEdge(l.Exit, from, newUnOp(ssa.OpNot, l.Cond, l.Block))
		}
	}

	handleSwitchEdge := func(sw *ssa.Switch) {
		from := sw.Block
		var defaultCond ssa.Value
		for _, lab := range sw.Label {
			cond := newBinOp(ssa.OpEq, sw.Cond, lab.Value, lab.Dest)
			newEdge(lab.Dest, from, cond)
			// lab.Dest.Condition = cond
			if defaultCond == nil {
				defaultCond = newUnOp(ssa.OpNot, cond, sw.DefaultBlock)
			} else {
				defaultCond = newBinOp(ssa.OpLogicOr, defaultCond, newUnOp(ssa.OpNot, cond, sw.DefaultBlock), sw.DefaultBlock)
			}
		}
		newEdge(sw.DefaultBlock, from, defaultCond)
	}

	deleteStmt := make([]ssa.Instruction, 0)
	// handler instruction
	for _, b := range fun.Blocks {
		for _, inst := range b.Insts {
			switch inst := inst.(type) {
			// call function
			case *ssa.Call:
				// TODO: config can set function return is a const
				// !! medium: need a good interface for user config this

			case *ssa.Field:
				if !inst.OutCapture {
					// TODO: handler field if this field not OutCaptured
					// ! easy: just replace value
				}

			// collect control flow
			case *ssa.If:
				handleIfEdge(inst)
			case *ssa.Loop:
				handleLoopEdge(inst)
			case *ssa.Switch:
				handleSwitchEdge(inst)

			}
		}
	}

	for _, inst := range deleteStmt {
		ssa.DeleteInst(inst)
	}

	// handler
	fun.EnterBlock.Condition = ssa.NewConst(true)
	fun.EnterBlock.Skip = true
	// deep first search
	var handlerBlock func(*ssa.BasicBlock)
	handlerBlock = func(bb *ssa.BasicBlock) {
		// skip finish block
		if _, ok := s.Finish[bb]; ok {
			return
		}
		// get condition
		cond := s.calcCondition(bb)
		if cond == nil {
			return
		}

		// set finish
		s.Finish[bb] = struct{}{}
		bb.Condition = cond

		if bb.Reachable() == -1 {
			bb.NewError(ssa.Warn, BCTag, "this block unreachable!")
		}

		// dfs
		for _, succ := range bb.Succs {
			handlerBlock(succ)
		}
	}

	for _, bb := range fun.Blocks {
		handlerBlock(bb)
	}
}

func (s *BlockCondition) calcCondition(block *ssa.BasicBlock) ssa.Value {
	getCondition := func(to, from *ssa.BasicBlock) ssa.Value {
		var edgeCond ssa.Value
		if fromTable, ok := s.edge[to]; ok {
			if value, ok := fromTable[from]; ok {
				edgeCond = value
			}
		}
		if edgeCond == nil {
			return from.Condition
		} else {
			return newBinOp(ssa.OpLogicAnd, from.Condition, edgeCond, from)
		}
	}

	skipBlock := make(map[*ssa.BasicBlock]struct{})
	if block.IsBlock(ssa.LoopHeader) {
		// loop.header get prev, but skip latch
		if b := block.GetBlock(ssa.LoopLatch); b != nil {
			skipBlock[b] = struct{}{}
		}
	}

	// handler normal
	var cond ssa.Value
	for _, pre := range block.Preds {
		// skip
		if _, ok := skipBlock[pre]; ok {
			continue
		}
		// check
		if pre.Condition == nil {
			// panic(fmt.Sprintf("this cond is null: %s", pre.Name))
			return nil
		}
		// calc
		if cond == nil {
			cond = getCondition(block, pre)
		} else {
			cond = newBinOp(ssa.OpLogicOr, cond, getCondition(block, pre), pre)
		}
	}
	return cond
}

func newBinOp(op ssa.BinaryOpcode, x ssa.Value, y ssa.Value, block *ssa.BasicBlock) ssa.Value {
	b := ssa.NewBinOp(op, x, y, block)
	if inst, ok := b.(ssa.Instruction); ok {
		ssa.EmitInst(inst)
	}
	return b
}

func newUnOp(op ssa.UnaryOpcode, x ssa.Value, block *ssa.BasicBlock) ssa.Value {
	u := ssa.NewUnOp(op, x, block)
	if inst, ok := u.(ssa.Instruction); ok {
		ssa.EmitInst(inst)
	}
	return u
}
