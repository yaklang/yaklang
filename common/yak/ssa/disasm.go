package ssa

import (
	"fmt"
)

func (f *Function) SetReg(i Instruction) {
	reg := fmt.Sprintf("t%d", len(f.instReg))
	f.instReg[i] = reg
}

func GetReg(I Instruction, f *Function) string {
	return f.instReg[I]
}

