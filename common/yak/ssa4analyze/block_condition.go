package ssa4analyze

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
)

const BCTag ssa.ErrorTag = "BlockCondition"

// block condition
type BlockCondition struct {
	edge   Edge
	Finish map[*ssa.BasicBlock]struct{}
}

var _ Analyzer = (*BlockCondition)(nil)

func NewBlockCondition(config) Analyzer {
	return &BlockCondition{
		edge:   make(Edge),
		Finish: make(map[*ssa.BasicBlock]struct{}),
	}
}

// map to -> from -> condition
type Edge map[*ssa.BasicBlock]map[*ssa.BasicBlock]ssa.Value

func (s *BlockCondition) Run(prog *ssa.Program) {
	for _, pkg := range prog.Packages {
		for _, fun := range pkg.Funcs {
			s.RunOnFunction(fun)
		}
	}
}

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
		from := i.GetBlock()
		if cond := i.Cond; cond.GetOpcode() == ssa.OpConstInst {
			cond.NewError(ssa.Warn, BCTag, ConditionIsConst("if"))
		}
		newEdge(i.True, from, i.Cond)
		newEdge(i.False, from, newUnOp(ssa.OpNot, i.Cond, i.GetBlock()))
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
			canReach := newBinOp(cond.Op, x, y, cond.GetBlock())
			can, ok := canReach.(*ssa.ConstInst)
			if ok && can.IsBoolean() {
				return can.Boolean()
			}
			return true
		}
		from := l.GetBlock()
		if !canReach() {
			newEdge(l.Body, from, ssa.NewConst(false))
			newEdge(l.Exit, from, ssa.NewConst(true))
		} else {
			newEdge(l.Body, from, l.Cond)
			newEdge(l.Exit, from, newUnOp(ssa.OpNot, l.Cond, l.GetBlock()))
		}
	}

	handleSwitchEdge := func(sw *ssa.Switch) {
		from := sw.GetBlock()
		if cond := sw.Cond; cond.GetOpcode() == ssa.OpConstInst {
			cond.NewError(ssa.Warn, BCTag, ConditionIsConst("switch"))
		}
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

	fixupBlockPos := func(b *ssa.BasicBlock) *ssa.Position {
		var start *ssa.Position
		for _, inst := range b.Insts {
			// inst.GetPosition == nil, this inst is edge
			if ssa.IsControlInstruction(inst) && inst.GetPosition() != nil {
				continue
			}
			start = inst.GetPosition()
			break
		}

		if start == nil {
			return nil
		}

		var end *ssa.Position
		for i := len(b.Insts) - 1; i >= 0; i-- {
			inst := b.Insts[i]
			if ssa.IsControlInstruction(inst) && inst.GetPosition() != nil {
				continue
			}
			end = inst.GetPosition()
			break
		}
		if end == nil {
			end = start
		}
		pos := &ssa.Position{}
		pos.StartLine = start.StartLine
		pos.StartColumn = start.StartColumn
		pos.EndColumn = end.EndColumn
		pos.EndLine = end.EndLine
		return pos
	}

	deleteInst := make([]ssa.Instruction, 0)
	// handler instruction
	for _, b := range fun.Blocks {
		// fix block position
		b.SetPosition(fixupBlockPos(b))

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
			case *ssa.BinOp:
				if v := ssa.HandlerBinOp(inst); v != inst {
					ssa.ReplaceAllValue(inst, v)
					deleteInst = append(deleteInst, inst)
				}
			case *ssa.UnOp:
				if v := ssa.HandlerUnOp(inst); v != inst {
					ssa.ReplaceAllValue(inst, v)
					deleteInst = append(deleteInst, inst)
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

	for _, inst := range deleteInst {
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
			bb.NewError(ssa.Warn, BCTag, BlockUnreachable())
		}

		// dfs
		for _, succ := range bb.Succs {
			handlerBlock(succ)
		}
	}

	for _, bb := range fun.Blocks {
		handlerBlock(bb)
	}
	fun.EnterBlock.Skip = false
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
		if b := block.GetBlockById(ssa.LoopLatch); b != nil {
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
	b := ssa.NewBinOp(op, x, y)
	if b, ok := ssa.ToBinOp(b); ok {
		block.EmitInst(b)
	}
	return b
}

func newUnOp(op ssa.UnaryOpcode, x ssa.Value, block *ssa.BasicBlock) ssa.Value {
	u := ssa.NewUnOp(op, x)
	if u, ok := ssa.ToUnOp(u); ok {
		block.EmitInst(u)
	}
	return u
}
