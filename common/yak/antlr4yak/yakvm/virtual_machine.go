package yakvm

import (
	"context"
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/utils/limitedmap"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm/vmstack"
)

type ExecFlag int

const (
	None   ExecFlag = 1 << iota // 默认创建新栈帧执行代码，执行后出栈
	Trace                       // 执行后不出站
	Sub                         // 使用栈顶的数据继续执行
	Inline                      // 使用上一次执行的Trace继续执行
	Asnyc                       // 异步执行
	Sandbox
)

func GetFlag(flags ...ExecFlag) ExecFlag {
	flag := None
	for _, f := range flags {
		flag |= f
	}
	return flag
}

type YakitFeedbacker interface{}
type (
	BreakPointFactoryFun func(v *VirtualMachine) bool
	VirtualMachine       struct {
		// globalVar 是当前引擎的全局变量，属于引擎
		globalVar        *limitedmap.ReadOnlyMap
		runtimeGlobalVar *limitedmap.SafeMap

		VMStack   *vmstack.Stack
		rootScope *Scope

		// asyncWaitGroup
		asyncWaitGroup *sync.WaitGroup
		// debug
		debug         bool // 内部debug
		debugMode     bool // 外部debugger
		debugger      *Debugger
		BreakPoint    []BreakPointFactoryFun
		ThreadIDCount uint64
		config        *VirtualMachineConfig
		// map[sha1(caller, callee)]func(any)any
		hijackMapMemberCallHandlers sync.Map
		globalVarFallback           func(string) interface{}
		GetExternalVar              func(name string) (any, bool)

		// sandbox
		sandboxMode bool

		callFuncCallback func(caller *Value, wavy bool, args []*Value)
	}
)

func (n *VirtualMachine) RegisterMapMemberCallHandler(caller, callee string, h func(interface{}) interface{}) {
	n.hijackMapMemberCallHandlers.Store(utils.CalcSha1(caller, callee), h)
}

func (n *VirtualMachine) RegisterGlobalVariableFallback(h func(string) interface{}) {
	n.globalVarFallback = h
}

func (v *VirtualMachine) AddBreakPoint(fun BreakPointFactoryFun) {
	v.BreakPoint = append(v.BreakPoint, fun)
}

func (n *VirtualMachine) GetExternalVariableNames() []string {
	vs := []string{}
	result := make(map[string]struct{})
	n.globalVar.ForEachKey(func(_ any, key string) error {
		_, existed := result[key]
		if existed {
			return nil
		}
		result[key] = struct{}{}
		vs = append(vs, key)
		return nil
	})
	n.runtimeGlobalVar.ForEachKey(func(_ any, key string) error {
		_, existed := result[key]
		if existed {
			return nil
		}
		result[key] = struct{}{}
		vs = append(vs, key)
		return nil
	})
	return vs
}

func (v *VirtualMachine) SetCallFuncCallback(callback func(caller *Value, wavy bool, args []*Value)) {
	v.callFuncCallback = callback
}

func (v *VirtualMachine) SetDebug(debug bool) {
	v.debug = debug
}

func (v *VirtualMachine) SetSandboxMode(mode bool) {
	v.sandboxMode = mode
}

func (v *VirtualMachine) SetDebugMode(debug bool, sourceCode string, codes []*Code, debugInit, debugCallback func(*Debugger)) {
	v.debugMode = debug
	if !debug {
		return
	}
	if v.debugger == nil {
		v.debugger = NewDebugger(v, sourceCode, codes, debugInit, debugCallback)
	}
}

func (v *VirtualMachine) SetSymboltable(table *SymbolTable) {
	v.rootScope = NewScope(table)
}

func (v *VirtualMachine) AsyncStart() {
	v.asyncWaitGroup.Add(1)
}

func (v *VirtualMachine) AsyncEnd() {
	v.asyncWaitGroup.Done()
}

func (v *VirtualMachine) AsyncWait() {
	v.asyncWaitGroup.Wait()
}

func NewWithSymbolTable(table *SymbolTable) *VirtualMachine {
	v := &VirtualMachine{
		// rootSymbol: table,
		rootScope:        NewScope(table),
		VMStack:          vmstack.New(),
		globalVar:        limitedmap.NewReadOnlyMap(map[string]any{}),
		runtimeGlobalVar: limitedmap.NewSafeMap(map[string]any{}),
		config:           NewVMConfig(),
		// asyncWaitGroup
		asyncWaitGroup: new(sync.WaitGroup),
		// debug
		ThreadIDCount: 1, // 初始是1
	}
	return v
}

func New() *VirtualMachine {
	return NewWithSymbolTable(NewSymbolTable())
}

// deepCopyLib 拷贝yaklang依赖，防止多个engine并行运行时对lib进行hook导致concurrent write map error
//func deepCopyLib(libs map[string]interface{}) map[string]interface{} {
//	newLib := map[string]interface{}{}
//	for k, v := range libs {
//		if v1, ok := v.(map[string]interface{}); ok {
//			newLib[k] = deepCopyLib(v1)
//		} else {
//			newLib[k] = v
//		}
//	}
//	return newLib
//}

// ImportLibs 导入库到引擎的全局变量中
func (n *VirtualMachine) ImportLibs(libs map[string]interface{}) {
	n.globalVar = n.globalVar.Append(libs)
	n.runtimeGlobalVar.SetPred(n.globalVar)
}

// SetVars 导入变量到引擎的全局变量中
func (n *VirtualMachine) SetVars(m map[string]any) {
	n.runtimeGlobalVar = n.runtimeGlobalVar.Append(m)
}

func (n *VirtualMachine) GetNaslGlobalVarTable() (map[int]*Value, error) {
	tableRaw, ok := n.GetVarWithoutFrame("__nasl_global_var_table")
	if !ok {
		return nil, utils.Error("BUG: __nasl_global_var_table cannot be found")
	}
	table, ok := tableRaw.(map[int]*Value)
	if !ok {
		return nil, utils.Error("BUG: __nasl_global_var_table is not a map")
	}
	return table, nil
}

func (n *VirtualMachine) GetVarWithoutFrame(name string) (any, bool) {
	if !n.runtimeGlobalVar.Existed(n.globalVar) {
		n.runtimeGlobalVar.SetPred(n.globalVar)
	}
	// 和引擎绑定的用于覆盖 global var 的 fake lib 层
	var_, ok := n.runtimeGlobalVar.Load(name)
	if ok {
		return var_, true
	}

	if n.globalVarFallback != nil {
		hijackedGlobal := n.globalVarFallback(name)
		if hijackedGlobal != nil {
			return hijackedGlobal, true
		}
	}

	return undefined, false
}

func (n *VirtualMachine) GetVar(name string) (interface{}, bool) {
	ivm := n.VMStack.Peek()
	if ivm == nil {
		val, ok := n.rootScope.GetValueByName(name)
		if ok {
			return val.Value, true
		}
		return n.GetVarWithoutFrame(name)
	}

	// ivm 存在的时候，从 frame 中找变量
	frame := ivm.(*Frame)
	val, ok := frame.CurrentScope().GetValueByName(name)
	if ok {
		return val.Value, true
	}
	return frame.GlobalVariables.Load(name)
}

func (n *VirtualMachine) GetGlobalVar() *limitedmap.ReadOnlyMap {
	return n.globalVar
}

func (n *VirtualMachine) GetRuntimeGlobalVar() *limitedmap.SafeMap {
	return n.runtimeGlobalVar
}

func (n *VirtualMachine) GetDebugger() *Debugger {
	return n.debugger
}

func (v *VirtualMachine) ExecYakFunction(ctx context.Context, f *Function, args map[int]*Value, flags ...ExecFlag) (interface{}, error) {
	return v.ExecYakFunctionEx(ctx, f, args, nil, flags...)
}

func (v *VirtualMachine) ExecYakFunctionEx(ctx context.Context, f *Function, args map[int]*Value, frameCallback func(*Frame), flags ...ExecFlag) (interface{}, error) {
	var value interface{}
	finalFlags := []ExecFlag{Sub}
	if len(flags) > 0 {
		finalFlags = flags
	}
	err := v.Exec(ctx, func(frame *Frame) {
		if v.sandboxMode && f.defineFrame != nil {
			frame = NewSubFrame(f.defineFrame)
		}
		name := f.GetActualName()
		frame.SetVerbose(fmt.Sprintf("function: %s", name))
		frame.SetFunction(f)
		if f.sourceCode != "" {
			frame.SetOriginCode(f.sourceCode)
		}
		// 闭包继承父作用域
		// if v.config.GetClosureSupport() {
		frame.scope = f.scope
		frame.CreateAndSwitchSubScope(f.symbolTable)
		for id, arg := range args {
			frame.CurrentScope().NewValueByID(id, arg)
		}
		if frameCallback != nil {
			frameCallback(frame)
		}
		frame.Exec(f.codes)
		if frame.lastStackValue != nil {
			value = frame.lastStackValue.Value
		}
		frame.ExitScope()
	}, finalFlags...)
	if err != nil {
		return nil, err
	}
	return value, nil
}

func (v *VirtualMachine) ExecAsyncYakFunction(ctx context.Context, f *Function, args map[int]*Value) error {
	return v.Exec(ctx, func(frame *Frame) {
		if v.sandboxMode && f.defineFrame != nil {
			frame = NewSubFrame(f.defineFrame)
		}
		name := f.GetActualName()
		frame.SetVerbose("function: " + name)
		frame.SetFunction(f)
		frame.SetScope(f.scope)
		frame.CreateAndSwitchSubScope(f.symbolTable)
		for id, arg := range args {
			frame.CurrentScope().NewValueByID(id, arg)
		}
		go func() {
			defer func() {
				v.AsyncEnd()
				if err := frame.recover(); err != nil {
					log.Errorf("yakvm async function panic: %v", err)
					// utils.PrintCurrentGoroutineRuntimeStack()
				}
				if err := recover(); err != nil {
					log.Errorf("yakvm async function panic: %v", err)
					utils.PrintCurrentGoroutineRuntimeStack()
				}
			}()

			frame.Exec(f.codes)
			frame.ExitScope()
		}()
	}, Sub, Asnyc)
}

func (v *VirtualMachine) ExecYakCode(ctx context.Context, sourceCode string, codes []*Code, flags ...ExecFlag) error {
	return v.Exec(ctx, func(frame *Frame) {
		frame.SetVerbose("__yak_main__")
		frame.SetOriginCode(sourceCode)
		frame.Exec(codes)
	}, flags...)
}

func (v *VirtualMachine) InlineExecYakCode(ctx context.Context, codes []*Code, flags ...ExecFlag) error {
	return v.Exec(ctx, func(frame *Frame) {
		frame.Exec(codes)
	}, Trace|Sub)
}

var vmstackLock = new(sync.Mutex)

func (v *VirtualMachine) Exec(ctx context.Context, f func(frame *Frame), flags ...ExecFlag) error {
	// 先检查 context 是否已取消
	if ctx.Err() != nil {
		return ctx.Err()
	}

	flag := GetFlag(flags...)

	var frame *Frame
	if flag&Sub == Sub {

		vmstackLock.Lock()
		topFrame := v.VMStack.Peek()
		vmstackLock.Unlock()

		if topFrame == nil {
			log.Errorf("BUG: VMStack is empty(Sub)")
			return utils.Error("BUG: VMStack is empty(Sub)")
		}
		frame = NewSubFrame(topFrame.(*Frame))
	} else if flag&Inline == Inline {
		vmstackLock.Lock()
		topFrame := v.VMStack.Peek()
		vmstackLock.Unlock()

		if topFrame == nil {
			topFrame = NewFrame(v)
			vmstackLock.Lock()
			v.VMStack.Push(topFrame)
			vmstackLock.Unlock()
			log.Debugf("VMStack is empty(Inline), we create new frame")
		}

		frame = topFrame.(*Frame)
		codes := frame.codes
		p := frame.codePointer

		frame.GlobalVariables = v.runtimeGlobalVar.SetPred(v.globalVar)
		defer func() {
			frame.codes = codes
			frame.codePointer = p
		}()
	} else {
		frame = NewFrame(v)
		frame.GlobalVariables = v.runtimeGlobalVar.SetPred(v.globalVar)
	}

	if flag&Asnyc == Asnyc {
		frame.coroutine = NewCoroutine()
	}

	vmstackLock.Lock()
	v.VMStack.Push(frame)
	vmstackLock.Unlock()

	frame.debug = v.debug
	// 初始化debugger
	if v.debugMode && v.debugger != nil && v.debugger.initFunc != nil {
		v.debugger.InitCallBack()
	}
	frame.ctx = ctx

	f(frame)

	// 未设置Trace时执行后出站
	if flag&Trace != Trace {
		vmstackLock.Lock()
		v.VMStack.Pop()
		vmstackLock.Unlock()
	}
	if flag&Asnyc != Asnyc {
		if lastPanic := frame.recover(); lastPanic != nil {
			lastPanic.contextInfos.Peek().(*PanicInfo).SetPositionVerbose(frame.GetVerbose())
			if exitValue, ok := lastPanic.data.(*VMPanicSignal); ok {
				panic(exitValue)
			} else {
				panic(lastPanic)
			}
		}
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func (v *VirtualMachine) CurrentFM() *Frame {
	return v.VMStack.Peek().(*Frame)
}

func (v *VirtualMachine) GetConfig() *VirtualMachineConfig {
	return v.config
}

func (v *VirtualMachine) SetConfig(config *VirtualMachineConfig) {
	v.config = config
}
