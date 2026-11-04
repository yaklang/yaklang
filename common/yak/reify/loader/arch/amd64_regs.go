package arch

import (
	"fmt"
	"strings"

	"golang.org/x/arch/x86/x86asm"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func amd64Register(reg x86asm.Reg) (string, uint16, bool) {
	switch {
	case reg >= x86asm.AL && reg <= x86asm.R15B:
		return strings.ToLower(reg.String()), 8, true
	case reg >= x86asm.AX && reg <= x86asm.R15W:
		return strings.ToLower(reg.String()), 16, true
	case reg >= x86asm.EAX && reg <= x86asm.R15L:
		return strings.ToLower(reg.String()), 32, true
	case reg >= x86asm.RAX && reg <= x86asm.R15:
		return strings.ToLower(reg.String()), 64, true
	case reg == x86asm.RIP:
		return "rip", 64, true
	case reg >= x86asm.X0 && reg <= x86asm.X15:
		return fmt.Sprintf("xmm%d", int(reg-x86asm.X0)), 128, true
	default:
		return "", 0, false
	}
}

func amd64GPR(reg x86asm.Reg) bool {
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

func amd64Condition(op x86asm.Op) string {
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

func amd64DirectTarget(ins amd64Instruction) (uint64, bool) {
	if len(ins.inst.Args) == 0 {
		return 0, false
	}
	rel, ok := ins.inst.Args[0].(x86asm.Rel)
	if !ok {
		return 0, false
	}
	return ins.address + uint64(ins.size) + uint64(int64(rel)), true
}

func amd64ConditionalJump(op x86asm.Op) bool {
	name := op.String()
	return strings.HasPrefix(name, "J") && name != "JMP"
}

func amd64Terminator(ins amd64Instruction) bool {
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
		return amd64ConditionalJump(ins.inst.Op)
	}
}

func amd64HasPrefix(ins amd64Instruction, prefix x86asm.Prefix) bool {
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

func amd64ControlTransfer(kind core.Opcode) bool {
	switch kind {
	case core.OpJump, core.OpBranch, core.OpReturn, core.OpTrap:
		return true
	default:
		return false
	}
}
