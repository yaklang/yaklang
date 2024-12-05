package templateLanguage

import "github.com/yaklang/yaklang/common/utils/memedit"

type Instructions []*Instruction

type Opcode int

const (
	OpPureText Opcode = iota
	OpOutput
	OpEscapeOutput
	OpPureOutPut
	OpPureCode
	OpImport
	OpIfStmt
)

type Instruction struct {
	Opcode     Opcode
	Attributes map[string]string
	Text       string
	Range      memedit.RangeIf

	ifBuilder *IfBuilder
}

func newInstruction(opcode Opcode, insRange ...memedit.RangeIf) *Instruction {
	var r memedit.RangeIf
	if len(insRange) > 0 {
		r = insRange[0]
	}
	return &Instruction{
		Opcode:     opcode,
		Attributes: map[string]string{},
		Range:      r,
	}
}
