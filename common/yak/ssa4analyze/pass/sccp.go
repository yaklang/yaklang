package pass

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
)

const SCCPTAG ssa.ErrorTag = "sccppass"

func init() {
	RegisterFunctionPass(&SCCP{})
}

// sccp
// implement simple conditional constant propagation
type SCCP struct {
	edge   Edge
	Finish map[*ssa.BasicBlock]struct{}
}

// map to -> from -> condition
type Edge map[*ssa.BasicBlock]map[*ssa.BasicBlock]ssa.Value

func (s *SCCP) RunOnFunction(fun *ssa.Function) {
	s.edge = make(Edge)
	s.Finish = make(map[*ssa.BasicBlock]struct{})
	newEdge := func(to, from *ssa.BasicBlock, condition ssa.Value) {
		fromtable, ok := s.edge[to]
		if !ok {
			fromtable = make(map[*ssa.BasicBlock]ssa.Value)
		}
		fromtable[from] = condition
		s.edge[to] = fromtable
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
			canReach := newBinOpOnly(cond.Op, x, y, cond.Block)
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
				defaultCond = newBinOp(ssa.OpOr, defaultCond, newUnOp(ssa.OpNot, cond, sw.DefaultBlock), sw.DefaultBlock)
			}
		}
		newEdge(sw.DefaultBlock, from, defaultCond)
	}

	deleteStmt := make([]ssa.Instruction, 0)
	// handler instruction
	for _, b := range fun.Blocks {
		for _, inst := range b.Instrs {
			switch inst := inst.(type) {
			// calc instruction
			case *ssa.BinOp:
				if ret := handlerBinOp(inst); ret != inst {
					deleteStmt = append(deleteStmt, inst)
				}
			case *ssa.UnOp:
				if ret := handlerUnOp(inst); ret != inst {
					deleteStmt = append(deleteStmt, inst)
				}
			// call function
			case *ssa.Call:
				// TODO: config can set funciton return is a const
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

		// dfs
		for _, succ := range bb.Succs {
			handlerBlock(succ)
		}
	}

	for _, bb := range fun.Blocks {
		handlerBlock(bb)
	}
}

func (s *SCCP) calcCondition(block *ssa.BasicBlock) ssa.Value {
	getCondition := func(to, from *ssa.BasicBlock) ssa.Value {
		var edgeCond ssa.Value
		if fromtable, ok := s.edge[to]; ok {
			if value, ok := fromtable[from]; ok {
				edgeCond = value
			}
		}
		if edgeCond == nil {
			return from.Condition
		} else {
			return newBinOp(ssa.OpAnd, from.Condition, edgeCond, from)
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
			cond = newBinOp(ssa.OpOr, cond, getCondition(block, pre), pre)
		}
	}
	return cond
}

func emit(v ssa.Value, block *ssa.BasicBlock) ssa.Value {
	if inst, ok := v.(ssa.Instruction); ok {
		if _, ok := inst.GetParent().InstReg[inst]; !ok {
			ssa.EmitBefore(block.LastInst(), inst)
		} else {
			return v
		}
	}
	return v
}
func newBinOpOnly(op ssa.BinaryOpcode, x, y ssa.Value, block *ssa.BasicBlock) ssa.Value {
	return handlerBinOpWithOutput(ssa.NewBinOp(op, x, y, block), false)
}

func newBinOp(op ssa.BinaryOpcode, x, y ssa.Value, block *ssa.BasicBlock) ssa.Value {
	return emit(newBinOpOnly(op, x, y, block), block)
}

func newUnOp(op ssa.UnaryOpcode, x ssa.Value, block *ssa.BasicBlock) ssa.Value {
	return emit(handlerUnOpWithOutput(ssa.NewUnOp(op, x, block), false), block)
}

func handlerBinOp(bin *ssa.BinOp) ssa.Value {
	return handlerBinOpWithOutput(bin, true)
}
func handlerBinOpWithOutput(bin *ssa.BinOp, output bool) ssa.Value {
	replace := func(ret ssa.Value) {
		if output {
			bin.NewError(ssa.Warn, SCCPTAG, "this binary expression can be replace with %s", ret)
		}
		ssa.ReplaceValue(bin, ret)
	}
	// merge const
	x, xIsConst := bin.X.(*ssa.Const)
	y, yIsConst := bin.Y.(*ssa.Const)
	if xIsConst && yIsConst {
		if ret := ConstBinary(x, y, bin.Op); ret != nil {
			replace(ret)
			return ret
		}
	}
	// one side
	if xIsConst {
		if ret := ConstOneSide(bin.Op, x, bin.Y); ret != nil {
			replace(ret)
			return ret
		}
	}
	if yIsConst {
		if ret := ConstOneSide(bin.Op, y, bin.X); ret != nil {
			replace(ret)
			return ret
		}
	}
	return bin
}

func handlerUnOp(u *ssa.UnOp) ssa.Value {
	return handlerUnOpWithOutput(u, true)
}
func handlerUnOpWithOutput(u *ssa.UnOp, output bool) ssa.Value {
	if x, ok := u.X.(*ssa.Const); ok {
		if ret := ConstUnary(x, u.Op); ret != nil {
			if output {
				u.NewError(ssa.Warn, SCCPTAG, "this unary expression can be replace with %s", ret.String())
			}
			ssa.ReplaceValue(u, ret)
			return ret
		}
	}
	return u
}

// OpShl BinaryOpcode = iota // <<

// OpShr    // >>
// OpAnd    // &
// OpAndNot // &^
// OpOr     // |
// OpXor    // ^
// OpAdd    // +
// OpSub    // -
// OpDiv    // /
// OpMod    // %
// // mul
// OpMul // *

// // boolean opcode
// OpGt    // >
// OpLt    // <
// OpGtEq  // >=
// OpLtEq  // <=
// OpEq    // ==
// OpNotEq // != <>

func ConstOneSide(op ssa.BinaryOpcode, b *ssa.Const, v ssa.Value) ssa.Value {
	if op == ssa.OpAnd && b.IsBoolean() {
		if b.Boolean() {
			// true && A = A
			return v
		} else {
			// false && A = false
			return b
		}
	}
	if op == ssa.OpOr && b.IsBoolean() {
		if b.Boolean() {
			// true || A = true
			return b
		} else {
			// false || A = A
			return v
		}
	}

	if op == ssa.OpMul && b.IsNumber() && b.Number() == 1 {
		// 1 * A = A
		return v
	}
	return nil
}

func ConstBinary(x, y *ssa.Const, op ssa.BinaryOpcode) *ssa.Const {
	switch op {
	case ssa.OpShl:
		if x.IsNumber() && y.IsNumber() {
			return ssa.NewConst(x.Number() << y.Number())
		}
	case ssa.OpShr:
		if x.IsNumber() && y.IsNumber() {
			return ssa.NewConst(x.Number() >> y.Number())
		}
	case ssa.OpAnd:
		if x.IsNumber() && y.IsNumber() {
			return ssa.NewConst(x.Number() & y.Number())
		} else if x.IsBoolean() && y.IsBoolean() {
			return ssa.NewConst(x.Boolean() && y.Boolean())
		}
	case ssa.OpAndNot:

	case ssa.OpOr:
		if x.IsNumber() && y.IsNumber() {
			return ssa.NewConst(x.Number() | y.Number())
		} else if x.IsBoolean() && y.IsBoolean() {
			return ssa.NewConst(x.Boolean() || y.Boolean())
		}
	case ssa.OpXor:
		if x.IsNumber() && y.IsNumber() {
			return ssa.NewConst(x.Number() ^ y.Number())
		}
	case ssa.OpAdd:
		if x.IsNumber() && y.IsNumber() {
			return ssa.NewConst(x.Number() + y.Number())
		}
		if x.IsString() && y.IsString() {
			return ssa.NewConst(x.VarString() + y.VarString())
		}
	case ssa.OpSub:
		if x.IsNumber() && y.IsNumber() {
			return ssa.NewConst(x.Number() - y.Number())
		}
	case ssa.OpDiv:
		if x.IsNumber() && y.IsNumber() {
			return ssa.NewConst(x.Number() / y.Number())
		}
	case ssa.OpMod:
		if x.IsNumber() && y.IsNumber() {
			return ssa.NewConst(x.Number() % y.Number())
		}
	case ssa.OpMul:
		if x.IsNumber() && y.IsNumber() {
			return ssa.NewConst(x.Number() * y.Number())
		}
	case ssa.OpGt:
		if x.IsNumber() && y.IsNumber() {
			return ssa.NewConst(x.Number() > y.Number())
		}
	case ssa.OpLt:
		if x.IsNumber() && y.IsNumber() {
			return ssa.NewConst(x.Number() < y.Number())
		}
	case ssa.OpGtEq:
		if x.IsNumber() && y.IsNumber() {
			return ssa.NewConst(x.Number() >= y.Number())
		}
	case ssa.OpLtEq:
		if x.IsNumber() && y.IsNumber() {
			return ssa.NewConst(x.Number() <= y.Number())
		}
	case ssa.OpEq:
		if x.IsNumber() && y.IsNumber() {
			return ssa.NewConst(x.Number() == y.Number())
		}
	case ssa.OpNotEq:
		if x.IsNumber() && y.IsNumber() {
			return ssa.NewConst(x.Number() != y.Number())
		}
	}

	return nil
}

// OpNone UnaryOpcode = iota
// OpNot              // !
// OpPlus             // +
// OpNeg              // -
// OpChan             // ->
func ConstUnary(x *ssa.Const, op ssa.UnaryOpcode) *ssa.Const {
	switch op {
	case ssa.OpNone:
		return x
	case ssa.OpNot:
		if x.IsBoolean() {
			return ssa.NewConst(!x.Boolean())
		}
	case ssa.OpPlus:
		if x.IsNumber() {
			return ssa.NewConst(+x.Number())
		}
	case ssa.OpNeg:
		if x.IsNumber() {
			return ssa.NewConst(-x.Number())
		}
	case ssa.OpChan:
	}
	return nil
}
