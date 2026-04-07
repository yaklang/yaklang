// Package pir defines the VM Intermediate Representation (PIR),
// a compact bytecode-like IR for virtualized functions. PIR functions are
// produced by lowering selected SSA functions and consumed by the VM executor.
package pir

import "fmt"

// ---------------------------------------------------------------------------
// Opcodes
// ---------------------------------------------------------------------------

// Opcode represents a single PIR operation.
type Opcode uint8

const (
	// Arithmetic
	OpNop   Opcode = iota
	OpConst        // load immediate: dst = imm
	OpAdd          // dst = lhs + rhs
	OpSub          // dst = lhs - rhs
	OpMul          // dst = lhs * rhs
	OpDiv          // dst = lhs / rhs (signed)
	OpMod          // dst = lhs % rhs (signed)
	OpNeg          // dst = -src

	// Bitwise
	OpAnd    // dst = lhs & rhs
	OpOr     // dst = lhs | rhs
	OpXor    // dst = lhs ^ rhs
	OpShl    // dst = lhs << rhs
	OpShr    // dst = lhs >> rhs (arithmetic)
	OpAndNot // dst = lhs &^ rhs

	// Comparison (result: 0 or 1)
	OpEq  // dst = (lhs == rhs)
	OpNeq // dst = (lhs != rhs)
	OpLt  // dst = (lhs <  rhs)
	OpLe  // dst = (lhs <= rhs)
	OpGt  // dst = (lhs >  rhs)
	OpGe  // dst = (lhs >= rhs)

	// Logical
	OpLogicAnd // dst = lhs && rhs (short-circuit semantics encoded by CFG)
	OpLogicOr  // dst = lhs || rhs

	// Control flow
	OpJump   // unconditional branch to block
	OpBranch // conditional branch: if cond goto trueBlk else falseBlk
	OpReturn // return value to caller
	OpPhi    // phi node: dst = phi(edges...)

	// Call
	OpCall     // internal PIR function call
	OpHostCall // bridge back to native host (via InvokeContext)

	// Stack / memory
	OpLoad  // dst = stack[slot]
	OpStore // stack[slot] = src
	OpArg   // dst = argument[index]

	// Sentinel
	opCount
)

var opcodeNames = [opCount]string{
	OpNop: "nop", OpConst: "const",
	OpAdd: "add", OpSub: "sub", OpMul: "mul", OpDiv: "div", OpMod: "mod", OpNeg: "neg",
	OpAnd: "and", OpOr: "or", OpXor: "xor", OpShl: "shl", OpShr: "shr", OpAndNot: "andnot",
	OpEq: "eq", OpNeq: "neq", OpLt: "lt", OpLe: "le", OpGt: "gt", OpGe: "ge",
	OpLogicAnd: "land", OpLogicOr: "lor",
	OpJump: "jump", OpBranch: "br", OpReturn: "ret", OpPhi: "phi",
	OpCall: "call", OpHostCall: "hostcall",
	OpLoad: "load", OpStore: "store", OpArg: "arg",
}

func (op Opcode) String() string {
	if int(op) < len(opcodeNames) && opcodeNames[op] != "" {
		return opcodeNames[op]
	}
	return fmt.Sprintf("op(%d)", op)
}

// ---------------------------------------------------------------------------
// Instruction
// ---------------------------------------------------------------------------

// Inst is a single PIR instruction. All operands are register indices
// (into the function's virtual register file) or block indices.
type Inst struct {
	Op  Opcode
	Dst int // destination register (-1 if none)

	// Operands – interpretation depends on Op:
	//   arithmetic/compare: Src[0]=lhs, Src[1]=rhs
	//   OpConst:            Imm = constant value
	//   OpJump:             Block = target block index
	//   OpBranch:           Src[0]=cond, Block=trueBlk, AuxBlock=falseBlk
	//   OpReturn:           Src[0]=return value (-1 for void)
	//   OpPhi:              Edges = [(blockIdx, regIdx), ...]
	//   OpCall/OpHostCall:  Src[0]=callee, CallArgs=argument regs
	//   OpArg:              Imm = argument index
	//   OpLoad/OpStore:     Imm = stack slot, Src[0]=value (store only)
	Src      [2]int
	Imm      int64
	Block    int // primary target block index
	AuxBlock int // secondary target block index (false branch)

	Edges    []PhiEdge // for OpPhi
	CallArgs []int     // for OpCall/OpHostCall
}

// PhiEdge associates a predecessor block with the value arriving from it.
type PhiEdge struct {
	Block int // predecessor block index in the PIR function
	Reg   int // register holding the incoming value
}

func (inst *Inst) String() string {
	switch inst.Op {
	case OpConst:
		return fmt.Sprintf("r%d = const %d", inst.Dst, inst.Imm)
	case OpAdd, OpSub, OpMul, OpDiv, OpMod,
		OpAnd, OpOr, OpXor, OpShl, OpShr, OpAndNot,
		OpEq, OpNeq, OpLt, OpLe, OpGt, OpGe,
		OpLogicAnd, OpLogicOr:
		return fmt.Sprintf("r%d = %s r%d, r%d", inst.Dst, inst.Op, inst.Src[0], inst.Src[1])
	case OpNeg:
		return fmt.Sprintf("r%d = neg r%d", inst.Dst, inst.Src[0])
	case OpJump:
		return fmt.Sprintf("jump bb%d", inst.Block)
	case OpBranch:
		return fmt.Sprintf("br r%d, bb%d, bb%d", inst.Src[0], inst.Block, inst.AuxBlock)
	case OpReturn:
		if inst.Src[0] >= 0 {
			return fmt.Sprintf("ret r%d", inst.Src[0])
		}
		return "ret void"
	case OpPhi:
		s := fmt.Sprintf("r%d = phi", inst.Dst)
		for _, e := range inst.Edges {
			s += fmt.Sprintf(" [bb%d: r%d]", e.Block, e.Reg)
		}
		return s
	case OpCall, OpHostCall:
		s := fmt.Sprintf("r%d = %s r%d(", inst.Dst, inst.Op, inst.Src[0])
		for i, a := range inst.CallArgs {
			if i > 0 {
				s += ", "
			}
			s += fmt.Sprintf("r%d", a)
		}
		return s + ")"
	case OpLoad:
		return fmt.Sprintf("r%d = load [%d]", inst.Dst, inst.Imm)
	case OpStore:
		return fmt.Sprintf("store [%d], r%d", inst.Imm, inst.Src[0])
	case OpArg:
		return fmt.Sprintf("r%d = arg %d", inst.Dst, inst.Imm)
	default:
		return fmt.Sprintf("%s r%d", inst.Op, inst.Dst)
	}
}

// ---------------------------------------------------------------------------
// Block
// ---------------------------------------------------------------------------

// Block is a basic block in a PIR function.
type Block struct {
	Index int
	Insts []Inst
	Preds []int // predecessor block indices
	Succs []int // successor block indices
}

// ---------------------------------------------------------------------------
// Function
// ---------------------------------------------------------------------------

// Function is a single protected function in PIR form.
type Function struct {
	Name       string
	SSAID      int64 // original SSA function value ID
	NumRegs    int   // total virtual registers allocated
	NumArgs    int   // number of parameters
	NumSlots   int   // local stack slots
	Blocks     []Block
	EntryBlock int // index of the entry block
}

// Dump returns a human-readable text representation of the PIR function.
func (f *Function) Dump() string {
	s := fmt.Sprintf("pir func %s (args=%d, regs=%d, slots=%d):\n",
		f.Name, f.NumArgs, f.NumRegs, f.NumSlots)
	for i := range f.Blocks {
		b := &f.Blocks[i]
		s += fmt.Sprintf("  bb%d: ; preds=%v succs=%v\n", b.Index, b.Preds, b.Succs)
		for j := range b.Insts {
			s += fmt.Sprintf("    %s\n", b.Insts[j].String())
		}
	}
	return s
}

// ---------------------------------------------------------------------------
// Region
// ---------------------------------------------------------------------------

// Region is a collection of PIR functions that form a protected region.
type Region struct {
	Functions []*Function
	// HostSymbols lists native symbols referenced by OpHostCall instructions.
	HostSymbols []string
}
