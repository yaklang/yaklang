package yakvm

import (
	"context"
	"sync"
	"yaklang.io/yaklang/common/yak/antlr4yak/yakvm/vmstack"
)

var buildinBinaryOperatorHandler = make(map[OpcodeFlag]func(*Value, *Value) *Value)
var buildinUnaryOperatorOperatorHandler = make(map[OpcodeFlag]func(*Value) *Value)
var buildinGlobalVariables = make(map[string]interface{})

func ImportBinaryOperator(flag OpcodeFlag, handler func(*Value, *Value) *Value) {
	buildinBinaryOperatorHandler[flag] = handler
}

func Import(k string, v interface{}) {
	buildinGlobalVariables[k] = v
}

func ImportUnaryOperator(flag OpcodeFlag, handler func(*Value) *Value) {
	buildinUnaryOperatorOperatorHandler[flag] = handler
}

type Frame struct {
	frameVerbose string
	vm           *VirtualMachine
	parent       *Frame
	// 字节码内容
	codes []*Code
	// 字节码指针
	codePointer int

	// 运算符的 Opcode
	BinaryOperatorTable map[OpcodeFlag]func(*Value, *Value) *Value
	UnaryOperatorTable  map[OpcodeFlag]func(*Value) *Value

	// yak函数, 内置函数，乃至变量聚集地
	GlobalVariables map[string]interface{}

	//yak函数
	// YakGlobalFunctions map[string]*Function
	// 运行栈
	stack *vmstack.Stack
	// 计数器栈，一般用于 for range 的计数
	iteratorStack *vmstack.Stack
	// 定义域栈
	//scopeStack *vmstack.Stack
	scope *Scope

	lastStackValue *Value

	// 当前执行的函数
	function *Function

	// debug: 打开之后将会输出很多调试信息
	debug          bool
	indebuggerEval bool // 在debugger中执行代码
	// panic
	panics   []*VMPanic
	tryStack *vmstack.Stack
	exitCode ExitCodeType

	// hijacks map[sha1(libName, memberName)]func(any)any
	hijackMapMemberCallHandlers sync.Map
	ctx                         context.Context
}

func (v *Frame) EnableDebuggerEval() {
	v.indebuggerEval = true
}
func (v *Frame) GetVirtualMachine() *VirtualMachine {
	return v.vm
}
func (v *Frame) GetFunction() *Function {
	return v.function
}

func (v *Frame) CurrentCode() *Code {
	return v.codes[v.codePointer]
}
func (v *Frame) GetVerbose() string {
	return v.frameVerbose
}
func (v *Frame) GetLastStackValue() *Value {
	if v == nil {
		return nil
	}
	return v.lastStackValue
}

func (v *Frame) SetVerbose(s string) {
	v.frameVerbose = s
}
func (v *Frame) SetScope(scope *Scope) {
	v.scope = scope
}
func (v *Frame) SetFunction(f *Function) {
	v.function = f
}
func (v *Frame) GetCodes() []*Code {
	return v.codes[:]
}
func (v *Frame) GetContext() context.Context {
	return v.ctx
}

//func (v *Frame) CreateSubFrame(code []*Code, symbolTable *SymbolTable) *Frame {
//	return v.CreateSubVirtualMachineWithScope(code, symbolTable, nil)
//}

func NewSubFrame(parent *Frame) *Frame {

	frame := &Frame{
		vm:                  parent.vm,
		parent:              parent,
		codePointer:         0,
		BinaryOperatorTable: parent.BinaryOperatorTable,
		UnaryOperatorTable:  parent.UnaryOperatorTable,
		GlobalVariables:     parent.GlobalVariables,
		// YakGlobalFunctions:  parent.YakGlobalFunctions,
		stack:         vmstack.New(),
		iteratorStack: vmstack.New(),
		tryStack:      vmstack.New(),
		scope:         parent.scope,
		debug:         parent.debug,
		exitCode:      NoneExit,
	}
	parent.hijackMapMemberCallHandlers.Range(func(key, value any) bool {
		frame.hijackMapMemberCallHandlers.Store(key, value)
		return true
	})

	return frame
}

func NewFrame(vm *VirtualMachine) *Frame {

	frame := &Frame{
		vm:                  vm,
		codePointer:         0,
		BinaryOperatorTable: make(map[OpcodeFlag]func(*Value, *Value) *Value),
		UnaryOperatorTable:  make(map[OpcodeFlag]func(*Value) *Value),
		GlobalVariables:     make(map[string]interface{}),
		tryStack:            vmstack.New(),
		// YakGlobalFunctions:  make(map[string]*Function),
		stack:         vmstack.New(),
		iteratorStack: vmstack.New(),
		scope:         vm.rootScope,
		debug:         false,
		exitCode:      NoneExit,
	}

	for k, v := range buildinBinaryOperatorHandler {
		frame.BinaryOperatorTable[k] = v
	}

	for k, v := range buildinUnaryOperatorOperatorHandler {
		frame.UnaryOperatorTable[k] = v
	}

	for k, v := range buildinGlobalVariables {
		frame.GlobalVariables[k] = v
	}
	for k, v := range vm.globalVar {
		frame.GlobalVariables[k] = v
	}

	vm.hijackMapMemberCallHandlers.Range(func(key, value any) bool {
		frame.hijackMapMemberCallHandlers.Store(key, value)
		return true
	})
	return frame
}
