package ssa4analyze

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

const BCTag ssa.ErrorTag = "BlockCondition"

// block condition
type BlockCondition struct {
	edge   Edge
	Finish map[ssa.Value]struct{}
}

var _ Analyzer = (*BlockCondition)(nil)

func NewBlockCondition(config) Analyzer {
	return &BlockCondition{
		edge:   make(Edge),
		Finish: make(map[ssa.Value]struct{}),
	}
}

// map to -> from -> condition
type Edge map[ssa.Value]map[ssa.Value]ssa.Value

func (s *BlockCondition) Run(prog *ssa.Program) {
	prog.EachFunction(func(f *ssa.Function) {
		s.RunOnFunction(f)
	})
}

func (s *BlockCondition) RunOnFunction(fun *ssa.Function) {
	s.edge = make(Edge)
	s.Finish = make(map[ssa.Value]struct{})
	newEdge := func(to, from, condition ssa.Value) {
		fromTable, ok := s.edge[to]
		if !ok {
			fromTable = make(map[ssa.Value]ssa.Value)
		}
		fromTable[from] = condition
		s.edge[to] = fromTable
	}

	handleIfEdge := func(i *ssa.If) {
		from := i.GetBlock()
		if cond := i.Cond; cond != nil && cond.GetOpcode() == ssa.SSAOpcodeConstInst {
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
		if cond := sw.Cond; cond != nil && cond.GetOpcode() == ssa.SSAOpcodeConstInst {
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

	fixupBlockPos := func(b *ssa.BasicBlock) memedit.RangeIf {
		var start memedit.PositionIf
		var end memedit.PositionIf
		for _, inst := range b.Insts {
			// inst.GetPosition == nil, this inst is edge
			if ssa.IsControlInstruction(inst) && inst.GetRange() != nil {
				continue
			}
			if pos := inst.GetRange(); pos != nil {
				start = pos.GetStart()
			}
			break
		}

		if start == nil {
			return nil
		}

		for i := len(b.Insts) - 1; i >= 0; i-- {
			inst := b.Insts[i]
			if ssa.IsControlInstruction(inst) && inst.GetRange() != nil {
				continue
			}
			if pos := inst.GetRange(); pos != nil {
				end = pos.GetStart()
			}
			break
		}
		if end == nil {
			end = start
		}

		var editor *memedit.MemEditor
		for _, inst := range b.Insts {
			if pos := inst.GetRange(); pos != nil {
				editor = pos.GetEditor()
				break
			}
		}
		if editor == nil {
			log.Warnf("BUG: block has no position (in instrs)")
		}
		r := editor.GetRangeByPosition(start, end)
		return r
	}
	_ = fixupBlockPos

	deleteInst := make([]ssa.Instruction, 0)
	// handler instruction
	for _, bRaw := range fun.Blocks {
		// fix block position
		// b.SetRange(fixupBlockPos(b))
		b, ok := ssa.ToBasicBlock(bRaw)
		if !ok {
			log.Warnf("function %s has a non-block instruction: %s", fun.GetName(), bRaw.GetName())
			continue
		}

		for _, inst := range b.Insts {
			switch inst := inst.(type) {
			// call function
			case *ssa.Call:
				// TODO: config can set function return is a const
				// !! medium: need a good interface for user config this

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
	enter, ok := ssa.ToBasicBlock(fun.EnterBlock)
	if ok {
		enter.SetReachable(true)
	} else {
		log.Warnf("BUG: function %s has a non-block instruction: %s", fun.GetName(), fun.EnterBlock.GetName())
	}

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

		if bb.Reachable() == ssa.BasicBlockUnReachable {
			bb.NewError(ssa.Warn, BCTag, BlockUnreachable())
		}

		// dfs
		for _, succRaw := range bb.Succs {
			succ, ok := succRaw.(*ssa.BasicBlock)
			if !ok {
				log.Warn("BUG: succ is not *ssa.BasicBlock")
				continue
			}
			handlerBlock(succ)
		}
	}

	for _, bb := range fun.Blocks {
		block, ok := ssa.ToBasicBlock(bb)
		if !ok {
			log.Warnf("function %s has a non-block instruction: %s", fun.GetName(), bb.GetName())
			continue
		}
		handlerBlock(block)
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
		if b := block.GetBlockById(ssa.LoopLatch); b != nil {
			skipBlock[b] = struct{}{}
		}
	}

	// handler normal
	var cond ssa.Value
	for _, preRaw := range block.Preds {
		pre, ok := preRaw.(*ssa.BasicBlock)
		if !ok {
			log.Warn("BUG: pre is not *ssa.BasicBlock")
			continue
		}

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

func newBinOp(op ssa.BinaryOpcode, x, y ssa.Value, block ssa.Value) ssa.Value {
	b := ssa.NewBinOp(op, x, y)
	// if b, ok := ssa.ToBinOp(b); ok {
	// 	block.EmitInst(b)
	// }
	return b
}

func newUnOp(op ssa.UnaryOpcode, x ssa.Value, block *ssa.BasicBlock) ssa.Value {
	u := ssa.NewUnOp(op, x)
	// if u, ok := ssa.ToUnOp(u); ok {
	// 	block.EmitInst(u)
	// }
	return u
}
