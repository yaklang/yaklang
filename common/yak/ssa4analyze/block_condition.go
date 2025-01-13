package ssa4analyze

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
	newEdge := func(to, from *ssa.BasicBlock, condition ssa.Value) {
		fromTable, ok := s.edge[to]
		if !ok {
			fromTable = make(map[ssa.Value]ssa.Value)
		}
		fromTable[from] = condition
		s.edge[to] = fromTable
	}

	handleIfEdge := func(i *ssa.If) {
		cond := i.GetValueById(i.Cond)
		fromBlock := i.GetBlock()
		trueBlock := i.GetBasicBlockByID(i.True)
		falseBlock := i.GetBasicBlockByID(i.False)
		if utils.IsNil(cond) || cond.GetOpcode() == ssa.SSAOpcodeConstInst {
			cond.NewError(ssa.Warn, BCTag, ConditionIsConst("if"))
		}
		newEdge(trueBlock, fromBlock, cond)
		newEdge(falseBlock, fromBlock, newUnOp(ssa.OpNot, cond, i.GetBlock()))
	}
	handleLoopEdge := func(l *ssa.Loop) {
		cond := l.GetValueById(l.Cond)
		canReach := func() bool {
			if l.Key <= 0 || l.Init <= 0 || l.Cond <= 0 {
				return true
			}
			cond, ok := ssa.ToBinOp(cond)
			if !ok {
				return true
			}
			var x, y ssa.Value

			if l.Key == cond.X {
				x = l.GetValueById(l.Init)
				y = l.GetValueById(cond.Y)
			} else {
				x = l.GetValueById(cond.X)
				y = l.GetValueById(l.Init)
			}
			canReach := newBinOp(cond.Op, x, y, cond.GetBlock())
			can, ok := canReach.(*ssa.ConstInst)
			if ok && can.IsBoolean() {
				return can.Boolean()
			}
			return true
		}
		from := l.GetBlock()
		bodyBlock := l.GetBasicBlockByID(l.Body)
		exitBlock := l.GetBasicBlockByID(l.Exit)
		if !canReach() {
			newEdge(bodyBlock, from, ssa.NewConst(false))
			newEdge(exitBlock, from, ssa.NewConst(true))
		} else {
			newEdge(bodyBlock, from, cond)
			newEdge(exitBlock, from, newUnOp(ssa.OpNot, cond, l.GetBlock()))
		}
	}

	handleSwitchEdge := func(sw *ssa.Switch) {
		fromBlock := sw.GetBlock()
		switchCondition := sw.GetValueById(sw.Cond)
		if utils.IsNil(switchCondition) || switchCondition.GetOpcode() == ssa.SSAOpcodeConstInst {
			sw.NewError(ssa.Warn, BCTag, ConditionIsConst("switch"))
		}
		var defaultCond ssa.Value
		for _, lab := range sw.Label {
			value := sw.GetValueById(lab.Value)
			dest := sw.GetBasicBlockByID(lab.Dest)
			var cond ssa.Value
			if utils.IsNil(switchCondition) {
				cond = value
			} else {
				cond = newBinOp(ssa.OpEq, switchCondition, value, dest)
			}
			newEdge(dest, fromBlock, cond)
			// lab.Dest.Condition = cond
			if defaultCond == nil {
				defaultCond = newUnOp(ssa.OpNot, cond, sw.DefaultBlock)
			} else {
				defaultCond = newBinOp(ssa.OpLogicOr, defaultCond, newUnOp(ssa.OpNot, cond, sw.DefaultBlock), sw.DefaultBlock)
			}
		}
		newEdge(sw.DefaultBlock, fromBlock, defaultCond)
	}

	fixupBlockPos := func(b *ssa.BasicBlock) memedit.RangeIf {
		var start memedit.PositionIf
		var end memedit.PositionIf
		for _, instId := range b.Insts {
			inst := b.GetInstructionById(instId)
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
			instId := b.Insts[i]
			inst := b.GetInstructionById(instId)
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
		for _, instId := range b.Insts {
			inst := b.GetInstructionById(instId)
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
		block := fun.GetBasicBlockByID(bRaw)
		if block == nil {
			log.Warnf("function %s has a non-block instruction: %s", fun.GetName(), block.GetName())
			continue
		}

		for _, instId := range block.Insts {
			inst := block.GetInstructionById(instId)
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
	enter := fun.GetBasicBlockByID(fun.EnterBlock)
	if enter != nil {
		enter.SetReachable(true)
	} else {
		log.Warnf("BUG: function %s has a non-block instruction: %s", fun.GetName(), enter.GetName())
	}

	// deep first search
	var handlerBlock func(*ssa.BasicBlock)
	handlerBlock = func(bb *ssa.BasicBlock) {
		if bb == nil {
			return
		}
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
		bb.Condition = cond.GetId()

		if bb.Reachable() == ssa.BasicBlockUnReachable {
			bb.NewError(ssa.Warn, BCTag, BlockUnreachable())
		}

		// dfs
		for _, succId := range bb.Succs {
			succBlock := bb.GetBasicBlockByID(succId)
			handlerBlock(succBlock)
		}
	}

	for _, bb := range fun.Blocks {
		block := fun.GetBasicBlockByID(bb)
		if block == nil {
			log.Warnf("function %s has a non-block instruction: %s", fun.GetName(), block.GetName())
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
		cond := block.GetValueById(from.Condition)
		if edgeCond == nil {
			return cond
		} else {
			return newBinOp(ssa.OpLogicAnd, cond, edgeCond, from)
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
	for _, id := range block.Preds {
		preRaw := block.GetValueById(id)
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
		if pre.Condition <= 0 {
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
