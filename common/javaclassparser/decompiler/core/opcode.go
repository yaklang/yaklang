package core

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
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
	//StackSimulation                *StackSimulationImpl
	StackEntry                     *StackItem
	ExceptionTypeIndex             uint16
	SwitchJmpCase                  *omap.OrderedMap[int, int32]
	SwitchJmpCase1                 *omap.OrderedMap[int, int]
	stackProduced                  []values.JavaValue
	stackConsumed                  []values.JavaValue
	Source                         []*OpCode
	Target                         []*OpCode
	TrueNode, FalseNode, MergeNode *OpCode
	StackInfo                      *utils.Stack[*values.JavaValue]
	IsTernaryNode                  bool
	IfNode                         *OpCode
	Info                           any
}
