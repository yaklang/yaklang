package core

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/utils"
)

type OpCode struct {
	Id                             int
	Instr                          *Instruction
	CurrentOffset                  uint16
	Data                           []byte
	Jmp                            int
	IsWide                         bool
	IsCatch                        bool
	GetStackChange                 func()
	IsTryCatchParent               bool
	TryNode                        *OpCode
	CatchNode                      []*OpCode
	ExceptionTypeIndex             uint16
	SwitchJmpCase                  map[int]uint32
	SwitchJmpCase1                 map[int]int
	stackProduced                  []values.JavaValue
	stackConsumed                  []values.JavaValue
	Source                         []*OpCode
	Target                         []*OpCode
	TrueNode, FalseNode, MergeNode *OpCode
	StackInfo                      *utils.Stack[*values.JavaValue]
	IsTernaryNode                  bool
}
