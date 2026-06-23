package core

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type OpCode struct {
	Id               int
	Instr            *Instruction
	CurrentOffset    uint16
	Data             []byte
	Jmp              int
	IsWide           bool
	IsCatch          bool
	GetStackChange   func()
	IsTryCatchParent bool
	TryNode          *OpCode
	CatchNode        []*CatchNode
	//StackSimulation                *StackSimulationImpl
	StackEntry                     *StackItem
	Ref                            *values.JavaRef
	ExceptionTypeIndex             uint16
	SwitchJmpCase                  *omap.OrderedMap[int, int32]
	SwitchJmpCase1                 *omap.OrderedMap[int, int]
	stackProduced                  []values.JavaValue
	stackConsumed                  []values.JavaValue
	Source                         []*OpCode
	Target                         []*OpCode
	TrueNode, FalseNode, MergeNode *OpCode
	Negative                       bool
	StackInfo                      *utils.Stack[*values.JavaValue]
	IsTernaryNode                  bool
	IfNode                         *OpCode
	Info                           any
	IsCustom                       bool
	conditionOpId                  int
	// SelfOpFolded marks a putfield/putstatic whose stored value is the post-increment /
	// post-decrement of the field itself, with the old value reused on the stack (the
	// dup_x1/dup idiom). When set, statement generation skips the standalone assignment
	// because the side effect is folded into the `x++` / `x--` expression left on the stack,
	// keeping ternary/expression branches side-effect-free so they can be structured.
	SelfOpFolded bool
}
type CatchNode struct {
	ExceptionTypeIndex uint16
	StartIndex         uint16
	EndIndex           uint16
	OpCode            *OpCode
}
