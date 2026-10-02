package yakvm

import (
	"context"
	"github.com/yaklang/yaklang/common/utils/limitedmap"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm/vmstack"
)

type Frame struct {
	originCode string

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
	GlobalVariables *limitedmap.SafeMap

	//yak函数
	// YakGlobalFunctions map[string]*Function
	// 运行栈
	stack operandStack
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
	ThreadID       int  // 当前线程的ID
	ownsThreadID   bool // this frame owns cleanup for an independently allocated ID
	// panic
	//panics   []*VMPanic
	tryStack *vmstack.Stack
	exitCode ExitCodeType

	// Immutable member-call handlers captured when this frame is created.
	hijackMapMemberCallHandlers mapMemberCallHandlers
	ctx                         context.Context
	contextData                 map[string]interface{} // 用于引擎执行时函数栈之间的数据传递
	runeCache                   map[*Value][]rune      // bounded frame-local cache for immutable string values
	runeCacheBytes              int
	pendingCallCode             *Code // the call immediately following an ellipsis expansion
	pendingCallArgCount         int
	executionDepth              int // recursive Exec depth for this exact frame

	coroutine      *Coroutine
	asyncExecution *asyncExecution

	// ownerGoroutineID is resolved once for a top-level execution and reused by
	// its synchronous subframes. It keeps goroutine-local frame isolation while
	// avoiding runtime.Stack on every nested function entry and frame pop.
	ownerGoroutineID int64
}

type Coroutine struct {
	lastPanic *VMPanic
}

func NewCoroutine() *Coroutine {
	return &Coroutine{lastPanic: nil}
}

func (v *Frame) SetOriginCode(s string) {
	v.originCode = s
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
	if v == nil || v.codePointer < 0 || v.codePointer >= len(v.codes) {
		return nil
	}
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
		originCode:          parent.originCode,
		vm:                  parent.vm,
		parent:              parent,
		codePointer:         0,
		BinaryOperatorTable: parent.BinaryOperatorTable,
		UnaryOperatorTable:  parent.UnaryOperatorTable,
		GlobalVariables:     parent.GlobalVariables,
		// YakGlobalFunctions:  parent.YakGlobalFunctions,
		iteratorStack:  vmstack.New(),
		tryStack:       vmstack.New(),
		scope:          parent.scope,
		debug:          parent.debug,
		exitCode:       NoneExit,
		ctx:            parent.ctx,
		contextData:    parent.contextData,
		coroutine:      parent.coroutine,
		asyncExecution: parent.asyncExecution,
		ThreadID:       parent.ThreadID,
	}
	frame.hijackMapMemberCallHandlers = parent.hijackMapMemberCallHandlers
	return frame
}

func NewFrame(vm *VirtualMachine) *Frame {
	binaryHandlers := buildinBinaryOperatorHandler[vm.config.vmMode]
	unaryHandlers := buildinUnaryOperatorOperatorHandler[vm.config.vmMode]
	frame := &Frame{
		vm:                  vm,
		codePointer:         0,
		BinaryOperatorTable: make(map[OpcodeFlag]func(*Value, *Value) *Value, len(binaryHandlers)),
		UnaryOperatorTable:  make(map[OpcodeFlag]func(*Value) *Value, len(unaryHandlers)),
		GlobalVariables:     vm.runtimeGlobalVar,
		tryStack:            vmstack.New(),
		// YakGlobalFunctions:  make(map[string]*Function),
		iteratorStack:  vmstack.New(),
		scope:          vm.rootScope,
		debug:          false,
		exitCode:       NoneExit,
		contextData:    make(map[string]interface{}),
		coroutine:      NewCoroutine(),
		asyncExecution: &asyncExecution{},
		ownsThreadID:   true,
	}
	for k, v := range binaryHandlers {
		frame.BinaryOperatorTable[k] = v
	}
	for k, v := range unaryHandlers {
		frame.UnaryOperatorTable[k] = v
	}

	// VM initialization and ImportLibs link runtimeGlobalVar to the immutable
	// global library chain.
	// Re-linking that shared SafeMap for every call races under concurrent hooks
	// and is redundant after engine initialization.
	frame.hijackMapMemberCallHandlers = vm.hijackMapMemberCallHandlers.loadSnapshot()

	// debug, 将rootScope加入到debugger中
	if vm.debugMode && vm.debugger != nil {
		vm.debugger.AddScopeRef(vm.rootScope)
	}
	return frame
}
