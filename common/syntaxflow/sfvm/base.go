package sfvm

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func validSSAOpcode(raw string) ssa.Opcode {
	text := yakunquote.TryUnquote(raw)
	switch text {
	case "call":
		return ssa.SSAOpcodeCall
	case "phi":
		return ssa.SSAOpcodePhi
	case "const", "constant":
		return ssa.SSAOpcodeConstInst
	case "param", "formal_param":
		return ssa.SSAOpcodeParameter
	case "return":
		return ssa.SSAOpcodeReturn
	case "function", "func", "def":
		return ssa.SSAOpcodeFunction
	default:
		log.Errorf("unknown opcode: %s", raw)
		return -1
	}
}

func validSSABinOpcode(raw string) string {
	text := yakunquote.TryUnquote(raw)
	switch text {
	case "add":
		return ssa.OpAdd
	case "sub":
		return ssa.OpSub
	case "mul":
		return ssa.OpMul
	case "div":
		return ssa.OpDiv
	case "mod":
		return ssa.OpMod
	case "gt":
		return ssa.OpGt
	case "lt":
		return ssa.OpLt
	default:
		log.Errorf("unknown opcode: %s", raw)
		return ""
	}
}
