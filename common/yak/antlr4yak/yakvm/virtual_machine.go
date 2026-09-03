package yakvm

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"

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

		frameStacksMu sync.RWMutex
		frameStacks   map[int64]*vmstack.Stack
		activeFrames  atomic.Int64
		rootScope     *Scope

		// asyncWaitGroup
		asyncWaitGroup *sync.WaitGroup
		// debug
		debug         bool // 内部debug
		debugMode     bool // 外部debugger
		debugger      *Debugger
		BreakPoint    []BreakPointFactoryFun
		ThreadIDCount uint64 // atomically allocated VM execution thread IDs
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
		frameStacks:      make(map[int64]*vmstack.Stack),
		globalVar:        limitedmap.NewReadOnlyMap(map[string]any{}),
		runtimeGlobalVar: limitedmap.NewSafeMap(map[string]any{}),
		config:           NewVMConfig(),
		// asyncWaitGroup
		asyncWaitGroup: new(sync.WaitGroup),
		// Thread IDs start at one. ThreadIDCount stores the last allocated ID,
		// so its zero value lets nextThreadID preserve that public convention.
		ThreadIDCount: 0,
	}
	// Establish the immutable-library parent before the VM can be shared. This
	// keeps even a bare VM from lazily mutating the parent link on its first
	// concurrent GetVar call; ImportLibs refreshes it when libraries are added.
	v.runtimeGlobalVar.SetPred(v.globalVar)
	v.runtimeGlobalVar.Store("runtime", newRuntimeLib(v))
	return v
}

func (v *VirtualMachine) nextThreadID() int {
	return int(atomic.AddUint64(&v.ThreadIDCount, 1))
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
	frame := n.peekCurrentFrame()
	if frame == nil {
		val, ok := n.rootScope.GetValueByName(name)
		if ok {
			return val.Value, true
		}
		return n.GetVarWithoutFrame(name)
	}

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
	return v.execYakFunctionWithParentFrame(ctx, nil, f, args, nil, flags...)
}

func (v *VirtualMachine) ExecYakFunctionEx(ctx context.Context, f *Function, args map[int]*Value, frameCallback func(*Frame), flags ...ExecFlag) (interface{}, error) {
	return v.execYakFunctionWithParentFrame(ctx, nil, f, args, frameCallback, flags...)
}

func (v *VirtualMachine) execYakFunctionWithParentFrame(ctx context.Context, parentFrame *Frame, f *Function, args map[int]*Value, frameCallback func(*Frame), flags ...ExecFlag) (interface{}, error) {
	var value interface{}
	finalFlags := []ExecFlag{Sub}
	if len(flags) > 0 {
		finalFlags = flags
	}
	var frameFactory func(*Frame) *Frame
	if v.sandboxMode && f.defineFrame != nil {
		// Sandbox functions execute against the frame in which they were
		// defined. Select that frame before registering the execution with the
		// VM so CurrentFM, runtime.GetInfo and engine-backed logging all observe
		// the frame that is actually running, rather than the temporary Sub
		// frame created by exec.
		frameFactory = func(defaultFrame *Frame) *Frame {
			frame := NewSubFrame(f.defineFrame)
			// The definition frame supplies lexical scope, globals and VM
			// capabilities, but its coroutine belongs to the execution that
			// originally created the closure. Reusing it makes concurrent
			// sandboxes race on lastPanic and can leak a panic across callers.
			// Keep panic propagation within the current caller's chain instead.
			frame.coroutine = defaultFrame.coroutine
			frame.ownerGoroutineID = defaultFrame.ownerGoroutineID
			if frame.vm == defaultFrame.vm {
				// A same-VM sandbox call is still a synchronous nested call and
				// therefore belongs to the caller's debugger thread.
				frame.ThreadID = defaultFrame.ThreadID
				frame.ownsThreadID = defaultFrame.ownsThreadID
			} else {
				// The definition VM owns this frame, its globals and its debugger.
				// Give independent cross-VM calls distinct IDs in that VM so
				// concurrent callers cannot share debugger stack state.
				frame.ThreadID = frame.vm.nextThreadID()
				frame.ownsThreadID = true
			}
			return frame
		}
	}
	err := v.execWithFrameFactory(ctx, parentFrame, frameFactory, func(frame *Frame) {
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

func (v *VirtualMachine) ExecAsyncYakFunction(ctx context.Context, parentFrame *Frame, f *Function, args map[int]*Value) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if parentFrame == nil && !(v.sandboxMode && f.defineFrame != nil) {
		log.Errorf("BUG: current frame is empty(Sub)")
		return utils.Error("BUG: current frame is empty(Sub)")
	}

	var frame *Frame
	if v.sandboxMode && f.defineFrame != nil {
		frame = NewSubFrame(f.defineFrame)
	} else {
		frame = NewSubFrame(parentFrame)
	}
	executionVM := frame.vm
	if executionVM == nil {
		executionVM = v
	}
	// Allocate the VM thread ID before the goroutine starts. Loading the shared
	// counter inside Frame.Exec lets delayed siblings observe the same final ID
	// and also makes synchronous subframes change IDs when another async call is
	// spawned. A sandbox can invoke a function defined by another VM; in that
	// case the frame keeps the definition VM's globals, operators and debugger,
	// so its thread ID and current-frame registration must live there as well.
	frame.ThreadID = executionVM.nextThreadID()
	frame.ownsThreadID = true
	frame.coroutine = NewCoroutine()

	name := f.GetActualName()
	frame.SetVerbose("function: " + name)
	frame.SetFunction(f)
	frame.SetScope(f.scope)
	frame.CreateAndSwitchSubScope(f.symbolTable)
	for id, arg := range args {
		frame.CurrentScope().NewValueByID(id, arg)
	}

	frame.ctx = ctx

	// Register only once the async frame is fully constructed. Early context or
	// validation failures must not leave an unmatched WaitGroup increment.
	v.AsyncStart()
	go func() {
		executionVM.pushCurrentFrame(frame)
		defer func() {
			executionVM.popCurrentFrame(frame)
			v.AsyncEnd()
			if err := frame.recover(); err != nil {
				log.Errorf("yakvm async function panic: %v", err)
			}
			if err := recover(); err != nil {
				log.Errorf("yakvm async function panic: %v", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()

		frame.Exec(f.codes)
		frame.ExitScope()
	}()
	return nil
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

func (v *VirtualMachine) Exec(ctx context.Context, f func(frame *Frame), flags ...ExecFlag) error {
	return v.exec(ctx, nil, f, flags...)
}

func (v *VirtualMachine) exec(ctx context.Context, parentFrame *Frame, f func(frame *Frame), flags ...ExecFlag) error {
	return v.execWithFrameFactory(ctx, parentFrame, nil, f, flags...)
}

func (v *VirtualMachine) execWithFrameFactory(ctx context.Context, parentFrame *Frame, frameFactory func(*Frame) *Frame, f func(frame *Frame), flags ...ExecFlag) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	flag := GetFlag(flags...)

	var frame *Frame
	if flag&Sub == Sub {
		if parentFrame == nil {
			parentFrame = v.peekCurrentFrame()
		}
		if parentFrame == nil {
			log.Errorf("BUG: current frame is empty(Sub)")
			return utils.Error("BUG: current frame is empty(Sub)")
		}
		frame = NewSubFrame(parentFrame)
		// Synchronous Yak function calls stay on the parent's goroutine. Reuse
		// its already-resolved ID instead of calling runtime.Stack for every
		// nested frame. Async calls do not go through this path and resolve their
		// owner after the goroutine starts.
		frame.ownerGoroutineID = parentFrame.ownerGoroutineID
	} else if flag&Inline == Inline {
		topFrame := v.peekCurrentFrame()

		if topFrame == nil {
			topFrame = NewFrame(v)
			log.Debugf("current frame is empty(Inline), create new frame")
		}

		frame = topFrame
		codes := frame.codes
		p := frame.codePointer

		frame.GlobalVariables = v.runtimeGlobalVar
		defer func() {
			frame.codes = codes
			frame.codePointer = p
		}()
	} else {
		frame = NewFrame(v)
	}
	// Resolve the execution ID before a sandbox factory chooses the VM that
	// will own the actual frame. Synchronous subframes inherit a non-zero ID;
	// independent roots receive a fresh one.
	if frame.ThreadID == 0 {
		threadVM := frame.vm
		if threadVM == nil {
			threadVM = v
		}
		frame.ThreadID = threadVM.nextThreadID()
		frame.ownsThreadID = true
	}
	if frameFactory != nil {
		frame = frameFactory(frame)
		if frame == nil {
			return utils.Error("BUG: frame factory returned nil")
		}
	}
	executionVM := frame.vm
	if executionVM == nil {
		executionVM = v
	}

	if flag&Asnyc == Asnyc {
		frame.coroutine = NewCoroutine()
	}

	frame.ctx = ctx

	executionVM.pushCurrentFrame(frame)
	traceCompleted := false
	defer func() {
		// Trace deliberately retains a successfully initialized frame for later
		// Inline execution. A panic or cancellation is not a usable trace and
		// must not pin its scope, bytecode, or goroutine stack forever.
		if flag&Trace != Trace || !traceCompleted {
			executionVM.popCurrentFrame(frame)
		}
	}()

	frame.debug = executionVM.debug
	if executionVM.debugMode && executionVM.debugger != nil && executionVM.debugger.initFunc != nil {
		executionVM.debugger.InitCallBack()
	}

	f(frame)

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
	traceCompleted = true
	return nil
}

func (v *VirtualMachine) CurrentFM() *Frame {
	return v.peekCurrentFrame()
}

func currentGoroutineID() int64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	const prefix = "goroutine "
	if n <= len(prefix) {
		return 0
	}

	var id int64
	foundDigit := false
	for i := len(prefix); i < n; i++ {
		c := buf[i]
		if c < '0' || c > '9' {
			break
		}
		foundDigit = true
		id = id*10 + int64(c-'0')
	}
	if !foundDigit {
		return 0
	}
	return id
}

func (v *VirtualMachine) pushCurrentFrame(frame *Frame) {
	gid := frame.ownerGoroutineID
	if gid == 0 {
		gid = currentGoroutineID()
		frame.ownerGoroutineID = gid
	}

	v.frameStacksMu.Lock()
	defer v.frameStacksMu.Unlock()
	stack, ok := v.frameStacks[gid]
	if !ok {
		stack = vmstack.New()
		v.frameStacks[gid] = stack
	}
	stack.Push(frame)
	v.activeFrames.Add(1)
}

func (v *VirtualMachine) popCurrentFrame(expected *Frame) *Frame {
	gid := expected.ownerGoroutineID
	if gid == 0 {
		gid = currentGoroutineID()
	}

	v.frameStacksMu.Lock()
	defer v.frameStacksMu.Unlock()

	stack, ok := v.frameStacks[gid]
	if !ok || stack == nil || stack.Len() == 0 {
		return nil
	}
	if stack.Peek() != expected {
		log.Errorf("yakvm frame stack mismatch: refusing to pop a non-current frame")
		return nil
	}
	frame, _ := stack.Pop().(*Frame)
	if frame != nil {
		v.activeFrames.Add(-1)
	}
	if stack.Len() == 0 {
		delete(v.frameStacks, gid)
	}
	return frame
}

func (v *VirtualMachine) peekCurrentFrame() *Frame {
	// Most external GetVar calls happen after compilation has finished, when
	// there cannot be a current frame. Avoid runtime.Stack entirely in that
	// common hot-load path.
	if v.activeFrames.Load() == 0 {
		return nil
	}
	gid := currentGoroutineID()

	v.frameStacksMu.RLock()
	defer v.frameStacksMu.RUnlock()

	stack, ok := v.frameStacks[gid]
	if !ok || stack == nil {
		return nil
	}
	frame, _ := stack.Peek().(*Frame)
	return frame
}

func (v *VirtualMachine) GetConfig() *VirtualMachineConfig {
	return v.config
}

func (v *VirtualMachine) SetConfig(config *VirtualMachineConfig) {
	v.config = config
}
