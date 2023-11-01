package ssaapi

import "github.com/yaklang/yaklang/common/yak/ssa"

type Instruction struct {
	ssa.Instruction
}

func NewInstruction(i ssa.Instruction) *Instruction {
	return &Instruction{
		Instruction: i,
	}
}
