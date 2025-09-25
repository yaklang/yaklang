package sfvm

import (
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func validSSAOpcode(raw string) ssa.Opcode {
	text := yakunquote.TryUnquote(raw)
	switch text {
	case "und", "undefined":
		return ssa.SSAOpcodeUndefined
	case "free", "freeValue":
		return ssa.SSAOpcodeFreeValue
	case "extendLib", "lib":
		return ssa.SSAOpcodeExternLib
	case "call":
		return ssa.SSAOpcodeCall
	case "phi":
		return ssa.SSAOpcodePhi
	case "const", "constant":
		return ssa.SSAOpcodeConstInst
	case "param", "formal_param":
		return ssa.SSAOpcodeParameter
	case "param_member", "parammember":
		return ssa.SSAOpcodeParameterMember
	case "return":
		return ssa.SSAOpcodeReturn
	case "function", "func", "def":
		return ssa.SSAOpcodeFunction
	case "basicblock", "basic_block", "block":
		return ssa.SSAOpcodeBasicBlock
	case "if":
		return ssa.SSAOpcodeIf
	case "try": // "error_handler"
		return ssa.SSAOpcodeErrorHandler
	case "catch":
		return ssa.SSAOpcodeErrorCatch
	case "throw", "panic":
		return ssa.SSAOpcodePanic
	case "switch":
		return ssa.SSAOpcodeSwitch
	case "loop":
		return ssa.SSAOpcodeLoop
	case "typecast":
		return ssa.SSAOpcodeTypeCast
	case "make":
		return ssa.SSAOpcodeMake
	default:
		return -1
	}
}

func validSSABinOpcode(raw string) string {
	text := yakunquote.TryUnquote(raw)
	switch text {
	case "add", "+":
		return ssa.OpAdd
	case "sub", "-":
		return ssa.OpSub
	case "mul", "*":
		return ssa.OpMul
	case "div", "/":
		return ssa.OpDiv
	case "mod", "%":
		return ssa.OpMod
	case "gt", ">":
		return ssa.OpGt
	case "lt", "<":
		return ssa.OpLt
	case "gteq", ">=":
		return ssa.OpGtEq
	case "lteq", "<=":
		return ssa.OpLtEq
	case "neq", "!=":
		return ssa.OpNotEq
	case "eq", "==":
		return ssa.OpEq
	case "and", "&&":
		return ssa.OpLogicAnd
	case "or", "||":
		return ssa.OpLogicOr
	case "xor", "^":
		return ssa.OpXor
	case "shl", "<<":
		return ssa.OpShl
	case "shr", ">>":
		return ssa.OpShr
	case "not", "!":
		return ssa.OpNot
	case "plus", "++":
		return ssa.OpPlus
	case "neg", "--":
		return ssa.OpNeg
	case "bitwise-not", "~":
		return ssa.OpBitwiseNot
	case "pow", "**=":
		return ssa.OpPow
	default:
		return ""
	}
}
