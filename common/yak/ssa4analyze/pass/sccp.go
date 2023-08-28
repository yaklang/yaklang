package pass

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

const SCCPTAG ssa.ErrorTag = "sccppass"

func init() {
	RegisterFunctionPass(&SCCP{})
}

// sccp
// implement simple conditional constant propagation
type SCCP struct {
}

func (s *SCCP) RunOnFunction(fun *ssa.Function) {
	edge = make(Edge)
	ifstmt := make([]*ssa.If, 0)
	switchstmt := make([]*ssa.Switch, 0)
	deleteStmt := make([]ssa.Instruction, 0)
	// handler instruction
	for _, b := range fun.Blocks {
		for _, inst := range b.Instrs {
			switch inst := inst.(type) {
			case *ssa.BinOp:
				if ret := handlerBinOp(inst); ret != inst {
					deleteStmt = append(deleteStmt, inst)
				}
			case *ssa.UnOp:
				if ret := handlerUnOp(inst); ret != inst {
					deleteStmt = append(deleteStmt, inst)
				}
			// collect if and switch
			case *ssa.If:
				ifstmt = append(ifstmt, inst)
			case *ssa.Switch:
				switchstmt = append(switchstmt, inst)
			}
		}
	}

	for _, inst := range deleteStmt {
		ssa.DeleteInst(inst)
	}

	// handler edge
	handlerEdge(ifstmt, switchstmt)

	// handler
	fun.EnterBlock.Condition = ssa.NewConst(true)
	fun.EnterBlock.Skip = true
	// deep first search
	worklist := make([]*ssa.BasicBlock, 0, len(fun.Blocks))
	worklist = append(worklist, fun.EnterBlock.Succs...)
	for i := 0; i < len(worklist); i++ {
		block := worklist[i]
		// println(block.Name)

		// handler condition
		block.Condition = calcCondition(block)

		for _, succ := range block.Succs {
			// avoid loop
			if succ.Skip {
				continue
			}
			worklist = append(worklist, succ)
			succ.Skip = true
		}
	}
}

// map to -> from -> condition
type Edge map[*ssa.BasicBlock]map[*ssa.BasicBlock]ssa.Value

var (
	edge Edge
)

func handlerEdge(ifstmt []*ssa.If, switchstmt []*ssa.Switch) {
	newEdge := func(to, from *ssa.BasicBlock, condition ssa.Value) {
		fromtable, ok := edge[to]
		if !ok {
			fromtable = make(map[*ssa.BasicBlock]ssa.Value)
		}
		fromtable[from] = condition
		edge[to] = fromtable
	}

	// mark
	for _, i := range ifstmt {
		from := i.Block
		newEdge(i.True, from, i.Cond)
		newEdge(i.False, from, newUnOp(ssa.OpNot, i.Cond, i.False))
	}

	for _, sw := range switchstmt {
		// defaultConds := make([]ssa.Value, 0)
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
		// default
		// sw.DefaultBlock.Condition = defaultCond
		newEdge(sw.DefaultBlock, from, defaultCond)
	}
}

func calcCondition(block *ssa.BasicBlock) ssa.Value {
	getCondition := func(to, from *ssa.BasicBlock) ssa.Value {
		var edgeCond ssa.Value
		if fromtable, ok := edge[to]; ok {
			if value, ok := fromtable[from]; ok {
				edgeCond = value
			}
		}
		if edgeCond == nil {
			return from.Condition
		} else {
			return newBinOp(ssa.OpAnd, from.Condition, edgeCond, to)
		}
	}

	if block.IsBlock(ssa.LoopExit) {
		// loop.exit just use loop.header
		if prev := block.GetBlock(ssa.LoopHeader); prev != nil {
			return getCondition(block, prev)
		}
	}

	if block.IsBlock(ssa.LoopBody) {
		if prev := block.GetBlock(ssa.LoopHeader); prev != nil {
			return getCondition(block, prev)
		}
	}

	if block.IsBlock(ssa.LoopLatch) {
		if prev := block.GetBlock(ssa.LoopBody); prev != nil {
			return prev.Condition
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
			panic(fmt.Sprintf("this cond is null: %s", pre.Name))
		}
		// calc
		if cond == nil {
			cond = getCondition(block, pre)
		} else {
			cond = newBinOp(ssa.OpOr, cond, getCondition(block, pre), block)
		}
	}
	return cond
}

func newBinOp(op ssa.BinaryOpcode, x, y ssa.Value, block *ssa.BasicBlock) ssa.Value {
	return handlerBinOpWithOutput(ssa.NewBinOp(op, x, y, block), false)
}

func newUnOp(op ssa.UnaryOpcode, x ssa.Value, block *ssa.BasicBlock) ssa.Value {
	return handlerUnOpWithOutput(ssa.NewUnOp(op, x, block), false)
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
			return ssa.NewConst(x.Number() - y.Number())
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
	case ssa.OpNeg:
	case ssa.OpChan:
	}
	return nil
}
