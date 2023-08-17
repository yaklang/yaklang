package yakvm

import (
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm/vmstack"

	"github.com/pkg/errors"
)

var (
	// 由yakast包注入
	YakDebugCompiler CompilerWrapperInterface
)

type LinesFirstCodeStateMap = map[int]*CodeState
type BreakpointMap = map[int]*Breakpoint

type Debugger struct {
	vm           *VirtualMachine
	once         sync.Once
	startWG      sync.WaitGroup  // 用于等待程序启动
	started      bool            // 表示程序是否已经启动
	finished     bool            // 表示程序是否已经结束
	wg           sync.WaitGroup  // 多个异步函数同时执行时回调断点,阻塞执行
	initFunc     func(*Debugger) // 初始化函数
	callbackFunc func(*Debugger) // 断点回调函数

	description string     // 回调时信息
	frame       *Frame     // 存储当前执行的frame
	state       string     // 表示当前处于哪个函数
	lock        sync.Mutex // 用于ShouldCallback的同步

	sourceFilePath                string
	sourceCode                    string
	sourceCodeLines               []string
	codes                         map[string][]*Code // state -> []code
	codePointer                   int
	linePointer                   int
	currentLinesFirstCodeStateMap LinesFirstCodeStateMap            // 每行第一个opcode索引
	lineFirstCodeStateMap         map[string]LinesFirstCodeStateMap // 文件路径 -> LinesFirstCodeStateMap

	// 断点
	breakPointCount      int32
	currentBreakPoint    *Breakpoint              // 当前断点
	currentBreakPointMap BreakpointMap            // 行 -> 断点
	breakpointMap        map[string]BreakpointMap // 文件路径 -> 断点

	// 用于步过，步入，步出
	jmpIndex    int
	stepOut     bool
	nextState   *StepStack
	stepInState *StepStack

	// 停止
	halt bool

	// 停止事件原因
	stopReason string

	// panic
	vmPanic *VMPanic

	// 堆栈跟踪
	StackTraces      map[int]*vmstack.Stack // threadID -> stacktraces
	ThreadStackTrace map[int]*StepStack     // 每个线程对应的当前的stackTrace

	// Reference,用于存储帧,作用域,变量引用的信息
	Reference *Reference

	// 观察断点
	observeBreakPointExpressions map[string]*Value

	// 观察表达式
	observeExpressions map[string]*Value
}

type StepStack struct {
	code                 *Code
	lineInedx, codeIndex int
	state                string
	stateName            string
	frame                *Frame
}
type CodeState struct {
	codeIndex int
	state     string
}

type StackTrace struct {
	ID   int
	Name string

	Frame *Frame

	Source             *string
	SourceCode         *string
	Line, Column       int
	EndLine, EndColumn int
}

type StackTraces struct {
	ThreadID    int
	StackTraces []StackTrace
}

func NewDebugger(vm *VirtualMachine, sourceCode string, codes []*Code, init, callback func(*Debugger)) *Debugger {

	debugger := &Debugger{
		started:               false,
		finished:              false,
		jmpIndex:              -1,
		StackTraces:           map[int]*vmstack.Stack{int(vm.ThreadIDCount): vmstack.New()},
		ThreadStackTrace:      make(map[int]*StepStack, 0),
		vm:                    vm,
		startWG:               sync.WaitGroup{},
		wg:                    sync.WaitGroup{},
		initFunc:              init,
		callbackFunc:          callback,
		sourceCode:            sourceCode,
		sourceCodeLines:       strings.Split(strings.ReplaceAll(sourceCode, "\r", ""), "\n"),
		codes:                 make(map[string][]*Code),
		linePointer:           0,
		lineFirstCodeStateMap: make(map[string]LinesFirstCodeStateMap),
		breakpointMap:         make(map[string]BreakpointMap),
		// currentLinesFirstCodeStateMap: make(LinesFirstCodeStateMap),

		Reference:                    NewReference(),
		currentBreakPointMap:         make(map[int]*Breakpoint, 0),
		observeExpressions:           make(map[string]*Value),
		observeBreakPointExpressions: make(map[string]*Value),
	}
	debugger.Init(codes)
	return debugger
}

func NewStepStackWithLineIndex(lineInedx int, state string) *StepStack {
	return &StepStack{
		lineInedx: lineInedx,
		state:     state,
	}
}

func NewStepStackWithCodeIndex(code *Code, codeIndex int, state, stateName string, frame *Frame) *StepStack {
	return &StepStack{
		code:      code,
		codeIndex: codeIndex,
		state:     state,
		stateName: stateName,
		frame:     frame,
	}
}

func NewCodeState(codeIndex int, state string) *CodeState {
	return &CodeState{
		codeIndex: codeIndex,
		state:     state,
	}
}

func (g *Debugger) Init(codes []*Code) {
	g.StartWGAdd()

	g.codes[""] = codes

	// 找出所有的函数及其opcode
	for _, code := range codes {
		if code.Opcode == OpPush {
			v := code.Op1
			if !v.IsYakFunction() {
				continue
			}
			f, _ := v.Value.(*Function)
			funcUUID := f.GetUUID()

			g.codes[funcUUID] = f.codes
		}
	}

	hasSet := false

	for state, codes := range g.codes {
		for index, code := range codes {
			current, ok := g.lineFirstCodeStateMap[*code.SourceCodeFilePath]
			if !ok {
				current = make(LinesFirstCodeStateMap)
				g.lineFirstCodeStateMap[*code.SourceCodeFilePath] = current
			}

			if _, ok := current[code.StartLineNumber]; !ok {
				current[code.StartLineNumber] = NewCodeState(index, state)
			}

			// 设置currentLinesFirstCodeStateMap和sourceFilePath
			// 使用比较笨的办法,找到传入的sourceCode与code绑定的sourceCode相同的第一个code
			if !hasSet && code.SourceCodePointer != nil && *code.SourceCodePointer == g.sourceCode {
				hasSet = true
				g.SwitchByOtherFileOpcode(code)
			}
		}
	}

}

func (g *Debugger) InitCallBack() {
	g.once.Do(func() {
		g.initFunc(g)
	})
}

func (g *Debugger) SwitchByOtherFileOpcode(code *Code) {
	newFilePath := *code.SourceCodeFilePath
	if newFilePath != g.sourceFilePath {
		g.sourceFilePath = newFilePath
		g.sourceCode = *code.SourceCodePointer
		g.sourceCodeLines = strings.Split(strings.ReplaceAll(g.sourceCode, "\r", ""), "\n")

		// 修改currentLinesFirstCodeStateMap
		g.currentLinesFirstCodeStateMap = g.lineFirstCodeStateMap[newFilePath]

		// 修改currentBreakPointMap
		bpm, ok := g.breakpointMap[newFilePath]
		if !ok {
			bpm = make(BreakpointMap)
			g.breakpointMap[newFilePath] = bpm
		}
		g.currentBreakPointMap = bpm
	}
}

func (g *Debugger) StartWGAdd() {
	g.startWG.Add(1)
}

func (g *Debugger) StartWGDone() {
	g.startWG.Done()
}

func (g *Debugger) StartWGWait() {
	g.startWG.Wait()
}

func (g *Debugger) Wait() {
	g.wg.Wait()
}

func (g *Debugger) Add() {
	g.wg.Add(1)
}

func (g *Debugger) WaitGroupDone() {
	g.wg.Done()
}

func (g *Debugger) Finished() bool {
	return g.finished
}

func (g *Debugger) SetFinished() {
	g.description = "The program is finished"
	g.finished = true
}

func (g *Debugger) CurrentCodeIndex() int {
	return g.codePointer
}

func (g *Debugger) CurrentLine() int {
	return g.linePointer
}

func (g *Debugger) CurrentBreakPoint() *Breakpoint {
	return g.currentBreakPoint
}

func (g *Debugger) Breakpoints() map[int]*Breakpoint {
	return g.currentBreakPointMap
}

func (g *Debugger) SourceCodeLines() []string {
	return g.sourceCodeLines
}

func (g *Debugger) InRootState() bool {
	return g.State() == ""
}

func (g *Debugger) StateName() string {
	frame := g.frame
	stateName := "__yak_main__"
	if frame == nil {
		return "unknown"
	}
	if f := frame.GetFunction(); f != nil {
		stateName = f.GetActualName()
	}
	return stateName
}

func (g *Debugger) State() string {
	return g.state
}

func (g *Debugger) UpdateByFrame(frame *Frame) {
	if f := frame.GetFunction(); f != nil {
		g.state = f.GetUUID()
	} else {
		g.state = ""
	}
	g.frame = frame
	g.AddFrameRef(frame)
}

func (g *Debugger) CurrentStackTrace() *vmstack.Stack {
	var (
		st *vmstack.Stack
		ok bool
	)
	frame := g.frame
	if frame == nil {
		return nil
	}

	if st, ok = g.StackTraces[g.frame.ThreadID]; !ok {
		st = vmstack.New()
		g.StackTraces[g.frame.ThreadID] = st
	}
	return st
}

func (g *Debugger) CurrentThreadID() int {
	frame := g.frame
	if frame == nil {
		return 0
	}
	return g.frame.ThreadID
}

func (g *Debugger) CurrentFrameID() int {
	frame := g.frame
	if frame == nil {
		return 0
	}
	ref := g.Reference
	i, ok := ref.FrameHM.getReverse(frame)
	if !ok {
		return 0
	}
	return i
}

func (g *Debugger) Codes() []*Code {
	return g.codes[g.State()]
}

func (g *Debugger) CodesInState(state string) []*Code {
	return g.codes[state]
}

func (g *Debugger) VM() *VirtualMachine {
	return g.vm
}
func (g *Debugger) Frame() *Frame {
	return g.frame
}

func (g *Debugger) Description() string {
	return g.description
}

func (g *Debugger) SetDescription(desc string) {
	g.description = desc
}

func (g *Debugger) ResetDescription() {
	g.description = ""
}

func (g *Debugger) StopReason() string {
	return g.stopReason
}

func (g *Debugger) SetStopReason(desc string) {
	g.stopReason = desc
}

func (g *Debugger) ResetStopReason() {
	g.stopReason = ""
}

func (g *Debugger) VMPanic() *VMPanic {
	return g.vmPanic
}

func (g *Debugger) SetVMPanic(p *VMPanic) {
	g.vmPanic = p
}

func (g *Debugger) AddFrameRef(frame *Frame) int {
	ref := g.Reference

	if i, ok := ref.FrameHM.getReverse(frame); !ok {
		return ref.FrameHM.create(frame)
	} else {
		return i
	}
}

func (g *Debugger) AddBreakPointRef(b *Breakpoint) int {
	ref := g.Reference

	if i, ok := ref.BreakPointHM.getReverse(b); !ok {
		return ref.BreakPointHM.create(b)
	} else {
		return i
	}
}
func (g *Debugger) ForceSetVariableRef(id int, v interface{}) {
	ref := g.Reference
	ref.VarHM.forceSet(id, v)
}

func (g *Debugger) AddVariableRef(v interface{}) int {
	ref := g.Reference
	if i, ok := ref.VarHM.getReverse(v); !ok {
		return ref.VarHM.create(v)
	} else {
		return i
	}
}

func (g *Debugger) AddScopeRef(scope *Scope) int {
	ref := g.Reference
	if i, ok := ref.VarHM.getReverse(scope); !ok {
		return ref.VarHM.create(scope)

	} else {
		return i
	}
}

func (g *Debugger) Pause() {
	g.halt = true
}

func (g *Debugger) IsPause() bool {
	return g.halt
}

func (g *Debugger) GetCode(state string, codeIndex int) *Code {
	codes := g.CodesInState(state)
	if codeIndex < 0 || codeIndex >= len(codes) {
		return nil
	}
	return codes[codeIndex]
}

func (g *Debugger) GetLineFirstCode(lineIndex int) (*Code, int, string) {
	if codeState, ok := g.currentLinesFirstCodeStateMap[lineIndex]; ok {
		return g.GetCode(codeState.state, codeState.codeIndex), codeState.codeIndex, codeState.state
	} else {
		return nil, -1, ""
	}
}

func (g *Debugger) stepStackToStackTrace(stepStack *StepStack) StackTrace {
	frame := stepStack.frame
	fid, ok := g.Reference.FrameHM.getReverse(frame)
	if !ok {
		fid = -1
	}
	return StackTrace{
		ID:         fid,
		Name:       stepStack.stateName,
		Frame:      frame,
		SourceCode: stepStack.code.SourceCodePointer,
		Source:     stepStack.code.SourceCodeFilePath,
		Line:       stepStack.code.StartLineNumber,
		Column:     stepStack.code.StartColumnNumber,
		EndLine:    stepStack.code.EndLineNumber,
		EndColumn:  stepStack.code.EndColumnNumber,
	}
}

func (g *Debugger) GetStackTraces() map[int]*StackTraces {
	stackTrace := g.CurrentStackTrace()
	if stackTrace == nil {
		return nil
	}
	if g.linePointer == 0 {
		return nil
	}

	ret := make(map[int]*StackTraces, len(g.StackTraces))

	for threadID, stack := range g.StackTraces {

		ret[threadID] = &StackTraces{
			ThreadID: threadID,
		}

		sts := make([]StackTrace, stack.Len()+1)

		// 加入ThreadStackTrace
		if stepStack, ok := g.ThreadStackTrace[threadID]; ok && stepStack.code != nil {
			sts[0] = g.stepStackToStackTrace(stepStack)
		}

		index2 := 1
		stack.GetAll(func(i any) {
			if stepStack, ok := i.(*StepStack); ok {
				if stepStack.code != nil {
					sts[index2] = g.stepStackToStackTrace(stepStack)
					index2++
				}
			}
		})

		ret[threadID].StackTraces = sts
	}

	return ret
}

func (g *Debugger) AddObserveBreakPoint(expr string) error {
	frame := g.frame
	if frame == nil {
		g.observeBreakPointExpressions[expr] = undefined
	} else {
		_, _, err := g.Compile(expr)
		if err != nil {
			return errors.Wrap(err, "add observe breakpoint error")
		}
	}
	return nil
}

func (g *Debugger) RemoveObserveBreakPoint(expr string) error {
	if _, ok := g.observeBreakPointExpressions[expr]; ok {
		delete(g.observeBreakPointExpressions, expr)
		return nil
	}

	return utils.Errorf("expression [%s] not in observe breakpoint", expr)
}

func (g *Debugger) AddObserveExpression(expr string) error {
	_, _, err := g.Compile(expr)
	if err != nil {
		return errors.Wrap(err, "add observe expression error")
	}
	g.observeExpressions[expr] = undefined
	return nil
}

func (g *Debugger) RemoveObserveExpression(expr string) error {
	if _, ok := g.observeExpressions[expr]; ok {
		delete(g.observeExpressions, expr)
		return nil
	}

	return utils.Errorf("expression [%s] not in observe expression", expr)
}

func (g *Debugger) GetAllObserveExpressions() map[string]*Value {
	return g.observeExpressions
}

func (g *Debugger) addBreakPoint(codeIndex, lineIndex int, condition, hitCondition, state string) (int, error) {
	if _, ok := g.currentBreakPointMap[lineIndex]; !ok {
		bp := g.NewBreakPoint(codeIndex, lineIndex, condition, hitCondition, state)
		g.currentBreakPointMap[lineIndex] = bp
		ref := g.AddBreakPointRef(bp)
		return ref, nil
	} else {
		return -1, errors.Errorf("breakpoint already exists in line %d", lineIndex)
	}
}

func (g *Debugger) SetBreakPoint(lineIndex int, condition, hitCondition string) (int, error) {
	code, codeIndex, state := g.GetLineFirstCode(lineIndex)
	if code == nil {
		return -1, utils.Errorf("Can't set breakPoint in line %d", lineIndex)
	} else {
		return g.addBreakPoint(codeIndex, lineIndex, condition, hitCondition, state)
	}
}

func (g *Debugger) SetNormalBreakPoint(lineIndex int) (int, error) {
	return g.SetBreakPoint(lineIndex, "", "")
}

func (g *Debugger) ClearAllBreakPoints() {
	g.currentBreakPointMap = make(map[int]*Breakpoint, 0)
}

func (g *Debugger) ClearBreakpointsInLine(lineIndex int) {
	if _, ok := g.currentBreakPointMap[lineIndex]; ok {
		delete(g.currentBreakPointMap, lineIndex)
	}
}

func (g *Debugger) EnableAllBreakPoints() {
	for _, breakpoint := range g.currentBreakPointMap {
		breakpoint.On = true
	}
}

func (g *Debugger) EnableBreakpointsInLine(lineIndex int) {
	for _, breakpoint := range g.currentBreakPointMap {
		if breakpoint.LineIndex == lineIndex {
			breakpoint.On = true
		}
	}
}

func (g *Debugger) DisableAllBreakPoints() {
	for _, breakpoint := range g.currentBreakPointMap {
		breakpoint.On = false
	}
}

func (g *Debugger) DisableBreakpointsInLine(lineIndex int) {
	for _, breakpoint := range g.currentBreakPointMap {
		if breakpoint.LineIndex == lineIndex {
			breakpoint.On = false
		}
	}
}

func (g *Debugger) StepNext() error {
	g.nextState = NewStepStackWithLineIndex(g.linePointer, g.State())
	return nil
}

func (g *Debugger) StepIn() error {
	g.GetLineFirstCode(g.linePointer)
	g.stepInState = NewStepStackWithLineIndex(g.linePointer, g.State())
	return nil
}

func (g *Debugger) StepOut() error {
	stackTrace := g.CurrentStackTrace()
	if stackTrace != nil && stackTrace.Len() > 0 {
		g.stepOut = true
		return nil
	} else {
		return utils.Errorf("Can't not step out")
	}
}

func (g *Debugger) HitCount(breakpoint *Breakpoint) bool {
	// 如果命中次数大于0，则命中次数减1,如果还大于0则不断点
	if breakpoint.HitCount > 0 {
		breakpoint.HitCount--
	}
	return breakpoint.HitCount > 0
}

func (g *Debugger) HandleForStepNext() {
	g.nextState = nil
	g.SetStopReason("step")
	g.Callback()
}

func (g *Debugger) HandleForStepIn() {
	g.stepInState = nil
	g.SetStopReason("step")
	g.Callback()
}

func (g *Debugger) HandleForStepOut() {
	g.stepOut = false
	g.SetStopReason("step")
	g.Callback()
}

func (g *Debugger) HandleForPause() {
	g.SetStopReason("pause")
	g.Callback()
}

func (g *Debugger) HandleForBreakPoint() {
	g.SetStopReason("breakpoint")
	g.Callback()
}

func (g *Debugger) ShouldCallback(frame *Frame) {
	g.lock.Lock()
	defer g.lock.Unlock()

	if !g.started {
		g.started = true
		g.StartWGDone()
	}

	codeIndex := frame.codePointer
	g.UpdateByFrame(frame)

	state, stateName := g.State(), g.StateName()
	code := g.GetCode(state, codeIndex)
	g.codePointer = codeIndex
	g.linePointer = code.StartLineNumber

	g.SwitchByOtherFileOpcode(code)

	stackTrace := g.CurrentStackTrace()

	if code.Opcode == OpCall {
		v := frame.peekN(code.Unary)
		// 如果同步调用函数，则push stepIn栈
		if v != nil && v.Callable() {
			defer func() {
				if stackTrace != nil {
					stackTrace.Push(NewStepStackWithCodeIndex(code, codeIndex, state, stateName, frame))
				}
			}()
		}
	}

	// 更新ThreadStackTrace
	g.ThreadStackTrace[g.frame.ThreadID] = NewStepStackWithCodeIndex(code, codeIndex, state, stateName, frame)

	// 捕捉错误
	defer func() {
		if r := recover(); r != nil {
			if rerr, ok := r.(error); ok {
				g.description = fmt.Sprintf("Runtime error: %v", rerr)
			} else {
				g.description = fmt.Sprintf("Runtime error: %v", r)
			}
			g.SetStopReason("exception")
			g.Callback()
		}
	}()

	// 如果halt,则回调
	if g.halt {
		g.halt = false
		g.HandleForPause()
		return
	}

	// 步进
	if g.nextState != nil {
		// 如果debugger想要步过且出现了jmp,则回调
		if g.jmpIndex == codeIndex {
			g.jmpIndex = -1
			g.HandleForStepNext()
		} else if g.linePointer > g.nextState.lineInedx {
			// 如果debugger想要步过且确实在后面行,则回调
			g.HandleForStepNext()
		} else if g.nextState.state != state {
			// 如果debugger想要步进且state不同，证明进入了函数，也应该回调
			g.HandleForStepNext()
		}
		return
	}

	// 步入
	if g.stepInState != nil {
		// 如果debugger想要步进且state不同，则回调
		if g.stepInState.state != state {
			// g.HandleForStepIn(codeIndex, state)
			g.HandleForStepIn()
		} else if g.stepInState.lineInedx < g.linePointer {
			// 如果已经超出此行，则回调
			g.HandleForStepIn()
		}
		return
	}

	// 步出
	if stackTrace != nil && stackTrace.Len() > 0 {
		stepStack := stackTrace.Peek().(*StepStack)
		// pop stepIn栈
		if stepStack.state == state && g.codePointer > stepStack.codeIndex {
			stackTrace.Pop()
			// 如果debugger想要步出且执行到了call后面的opcode，则应该回调
			if g.stepOut {
				g.HandleForStepOut()
			}
		}
	}
	// 如果处于stepOut状态，则不应该触发断点
	if g.stepOut {
		return
	}

	if len(g.observeBreakPointExpressions) > 0 {
		for expr, v := range g.observeBreakPointExpressions {
			nv, err := g.EvalExpression(expr)
			if nv == nil || err != nil {
				nv = undefined
			}
			if !v.Equal(nv) {
				g.observeBreakPointExpressions[expr] = nv
				g.description = fmt.Sprintf("Trigger observe breakpoint at line %d in %s", g.linePointer, g.StateName())
				g.HandleForBreakPoint()
				return
			}
		}
	}

	triggered := false

	// 如果存在于断点列表中，则回调
	for _, breakpoint := range g.currentBreakPointMap {

		// 如果断点被禁用则不应该触发
		if !breakpoint.On {
			continue
		}

		// 如果不在同一个state里则不应该触发
		if state != breakpoint.State {
			continue
		}

		// 行断点,包含普通断点和条件断点
		if breakpoint.CodeIndex == codeIndex {
			// 条件断点

			condition, hitCondition := breakpoint.Condition, breakpoint.HitCondition
			if condition == "" {
				// 如果命中次数大于0，则命中次数减1,如果还大于0则不断点
				if g.HitCount(breakpoint) {
					continue
				}
			}

			if condition != "" || hitCondition != "" {
				// 如果condition为空，则使用hitCondition
				cond := condition
				if condition == "" {
					cond = hitCondition
				}
				value, err := g.EvalExpression(cond)

				// 如果condition不成立,则不断点
				if err != nil || value.False() {
					continue
				}

				// 如果命中次数大于0，则命中次数减1,如果还大于0则不断点
				if g.HitCount(breakpoint) {
					continue
				}

				// 如果hitCondition都不为空，则还需要判断hitCondition
				if hitCondition != "" {
					value, err := g.EvalExpression(hitCondition)

					// 如果条件不成立,则不断点
					if err != nil || value.False() {
						continue
					}

					cond = fmt.Sprintf("%s && %s", condition, hitCondition)
				}

				// 触发条件断点的条件:
				// 1. condition成立,没有hitCount和hitCondition
				// 2. hitCount存在并减为0
				// 3. condition成立,hitCondition成立

				g.description = fmt.Sprintf("Trigger condtional breakpoint [%s] at line %d in %s", cond, g.linePointer, g.StateName())
			} else {
				// 普通断点
				g.description = fmt.Sprintf("Trigger normal breakpoint at line %d in %s", g.linePointer, g.StateName())
			}
			g.currentBreakPoint = breakpoint

			triggered = true

			break
		}
	}

	if triggered {
		g.HandleForBreakPoint()
	}

}

func (g *Debugger) Callback() {
	g.Add()
	defer g.WaitGroupDone()
	defer g.ResetStopReason()

	// 更新观察表达式
	if len(g.observeExpressions) > 0 {
		for expr := range g.observeExpressions {
			value, err := g.EvalExpression(expr)
			if err != nil {
				value = undefined
			}
			g.observeExpressions[expr] = value
		}
	}

	g.callbackFunc(g)
}

func (g *Debugger) GetScopesByFrameID(frameID int) map[int]*Scope {
	ref := g.Reference
	frame, ok := ref.FrameHM.get(frameID)
	if !ok {
		return nil
	}
	scopes := make(map[int]*Scope, 0)
	scope := frame.CurrentScope()
	for scope != nil {
		if id, ok := ref.VarHM.getReverse(scope); ok {
			scopes[id] = scope
		}
		scope = scope.parent
	}
	return scopes
}

func (g *Debugger) GetVariablesByRef(ref int) (interface{}, bool) {
	v, ok := g.Reference.VarHM.get(ref)
	if !ok {
		return nil, false
	}
	return v, true
}

func (g *Debugger) GetVariablesRef(v interface{}) (int, bool) {
	i, ok := g.Reference.VarHM.getReverse(v)
	if !ok {
		return 0, false
	}
	return i, true
}

func (g *Debugger) CompileWithFrame(code string, frame *Frame) (*Frame, CompilerWrapperInterface, error) {
	var err error
	frame.EnableDebuggerEval()
	sym, err := frame.CurrentScope().GetSymTable().GetRoot()
	if err != nil {
		return nil, nil, errors.Wrap(err, "find symboltable error")
	}

	YakDebugCompiler = YakDebugCompiler.NewWithSymbolTable(sym)
	YakDebugCompiler.Compiler(code)
	exist, err := YakDebugCompiler.GetNormalErrors()
	if exist {
		return nil, nil, errors.Wrap(err, "compile code error")
	}
	return frame, YakDebugCompiler, nil
}

func (g *Debugger) CompileWithFrameID(code string, frameID int) (*Frame, CompilerWrapperInterface, error) {
	targetFrame, ok := g.Reference.FrameHM.get(frameID)
	if !ok {
		return nil, nil, errors.New("frame not found")
	}

	return g.CompileWithFrame(code, targetFrame)
}

func (g *Debugger) Compile(code string) (*Frame, CompilerWrapperInterface, error) {
	frame := NewSubFrame(g.frame)
	return g.CompileWithFrame(code, frame)
}

func (g *Debugger) evalExpressionWithOpCodes(opcode []*Code, frame *Frame) (*Value, error) {
	var err error

	if len(opcode) == 0 {
		return nil, errors.New("eval code error: no opcode")
	}

	// 对opcode做特殊处理,把pop改成return
	if opcode[len(opcode)-1].Opcode == OpPop {
		opcode[len(opcode)-1].Opcode = OpReturn
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				if rerr, ok := r.(error); ok {
					err = errors.Wrap(rerr, "eval code error")
				} else if rstr, ok := r.(string); ok {
					err = errors.Wrap(errors.New(rstr), "eval code error")
				}
			}
		}()
		frame.Exec(opcode)
	}()

	return frame.GetLastStackValue(), err
}

func (g *Debugger) EvalExpressionWithFrameID(expr string, frameID int) (*Value, error) {
	var err error

	frame, compiler, err := g.CompileWithFrameID(expr, frameID)
	if err != nil {
		return nil, errors.Wrap(err, "eval code error")
	}

	opcode := compiler.GetOpcodes()
	return g.evalExpressionWithOpCodes(opcode, frame)
}

func (g *Debugger) EvalExpression(expr string) (*Value, error) {
	var err error

	frame, compiler, err := g.Compile(expr)
	if err != nil {
		return nil, errors.Wrap(err, "eval code error")
	}

	opcode := compiler.GetOpcodes()

	return g.evalExpressionWithOpCodes(opcode, frame)
}
