package x86

import (
	"strings"

	"golang.org/x/arch/x86/x86asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func isGPR(reg x86asm.Reg) bool {
	switch {
	case reg >= x86asm.AL && reg <= x86asm.R15B:
		return true
	case reg >= x86asm.AX && reg <= x86asm.R15W:
		return true
	case reg >= x86asm.EAX && reg <= x86asm.R15L:
		return true
	case reg >= x86asm.RAX && reg <= x86asm.R15:
		return true
	default:
		return false
	}
}

func conditionName(op x86asm.Op) string {
	name := op.String()
	switch {
	case strings.HasPrefix(name, "CMOV"):
		return strings.ToLower(name[len("CMOV"):])
	case strings.HasPrefix(name, "SET"):
		return strings.ToLower(name[len("SET"):])
	case strings.HasPrefix(name, "J") && name != "JMP":
		return strings.ToLower(name[len("J"):])
	default:
		return ""
	}
}

func directTarget(ins instruction) (uint64, bool) {
	if len(ins.inst.Args) == 0 {
		return 0, false
	}
	rel, ok := ins.inst.Args[0].(x86asm.Rel)
	if !ok {
		return 0, false
	}
	return ins.address + uint64(ins.size) + uint64(int64(rel)), true
}

func isTerminator(ins instruction) bool {
	if ins.endbr != "" {
		return false
	}
	switch ins.inst.Op {
	case x86asm.JMP, x86asm.RET, x86asm.UD2, x86asm.HLT, x86asm.IRET:
		return true
	case x86asm.INT:
		imm, ok := ins.inst.Args[0].(x86asm.Imm)
		return ok && imm == 3
	default:
		return isConditionalJump(ins.inst.Op)
	}
}

func hasPrefix(ins instruction, prefix x86asm.Prefix) bool {
	for _, value := range ins.inst.Prefix {
		if value == 0 {
			break
		}
		if value&0xff == prefix&0xff {
			return true
		}
	}
	return false
}

// isConditionalJump reports whether an opcode is a conditional branch. x86asm
// names every conditional branch with a J prefix and no other opcode shares it
// except JMP, so the prefix is the complete test.
func isConditionalJump(op x86asm.Op) bool {
	name := op.String()
	return strings.HasPrefix(name, "J") && name != "JMP"
}

// controlTransfer reports whether an opcode ends a basic block.
func controlTransfer(kind core.Opcode) bool {
	switch kind {
	case core.OpJump, core.OpBranch, core.OpReturn, core.OpTrap:
		return true
	default:
		return false
	}
}
