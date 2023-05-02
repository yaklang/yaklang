package yakvm

import (
	"fmt"
	"strings"
	"sync"
	"yaklang/common/go-funk"
	"yaklang/common/utils"
	"yaklang/common/yak/antlr4yak/yakvm/vmstack"

	"github.com/pkg/errors"
)

var (
	// 由yakast包注入
	YakDebugCompiler CompilerWrapperInterface
)

type Debugger struct {
	vm                *VirtualMachine
	once              sync.Once
	finished          bool
	wg                sync.WaitGroup  // 多个异步函数同时执行时回调断点,阻塞执行
	initFunc          func(*Debugger) // 初始化函数
	callbackFunc      func(*Debugger) // 断点回调函数
	currentBreakPoint *Breakpoint
	breakPoints       []*Breakpoint // 断点
	description       string        // 回调时信息

	sourceCode                string
	sourceCodeLines           []string
	codes                     map[string][]*Code
	maxLine                   int
	codePointer               int
	linePointer               int
	linesFirstCodeAndStateMap map[int]*CodeState // 每行第一个opcode索引

	// 用于步过，步入，步出
	jmpIndex    int
	nextState   *StepState
	stepInState *StepState
	stepOut     bool
	stepInStack *vmstack.Stack

	// 观察断点
	observeBreakPointExpressions map[string]*Value

	// 观察表达式
	observeExpressions map[string]*Value
}

type StepState struct {
	lineInedx, codeIndex int
	state                string
}

type CodeState struct {
	codeIndex int
	state     string
}

func NewDebugger(vm *VirtualMachine, sourceCode string, codes []*Code, init, callback func(*Debugger)) *Debugger {
	debugger := &Debugger{
		finished:                     false,
		jmpIndex:                     -1,
		stepInStack:                  vmstack.New(),
		vm:                           vm,
		wg:                           sync.WaitGroup{},
		initFunc:                     init,
		callbackFunc:                 callback,
		sourceCode:                   sourceCode,
		sourceCodeLines:              strings.Split(strings.ReplaceAll(sourceCode, "\r", ""), "\n"),
		codes:                        make(map[string][]*Code),
		linePointer:                  0,
		linesFirstCodeAndStateMap:    make(map[int]*CodeState),
		breakPoints:                  make([]*Breakpoint, 0),
		observeExpressions:           make(map[string]*Value),
		observeBreakPointExpressions: make(map[string]*Value),
	}
	debugger.Init(codes)
	return debugger
}

func NewStepStateWithLineIndex(lineInedx int, state string) *StepState {
	return &StepState{
		lineInedx: lineInedx,
		state:     state,
	}
}

func NewStepStateWithCodeIndex(codeIndex int, state string) *StepState {
	return &StepState{
		codeIndex: codeIndex,
		state:     state,
	}
}

func NewCodeState(codeIndex int, state string) *CodeState {
	return &CodeState{
		codeIndex: codeIndex,
		state:     state,
	}
}

func (g *Debugger) Init(codes []*Code) {
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

	for state, codes := range g.codes {
		for index, code := range codes {
			if _, ok := g.linesFirstCodeAndStateMap[code.StartLineNumber]; !ok {
				g.linesFirstCodeAndStateMap[code.StartLineNumber] = NewCodeState(index, state)
				g.maxLine = code.StartLineNumber
			}
		}
	}
}

func (g *Debugger) InitCallBack() {
	g.once.Do(func() {
		g.initFunc(g)
	})
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

func (g *Debugger) CurrentCodeIndex() int {
	return g.codePointer
}

func (g *Debugger) CurrentLine() int {
	return g.linePointer
}

func (g *Debugger) CurrentBreakPoint() *Breakpoint {
	return g.currentBreakPoint
}

func (g *Debugger) Breakpoints() []*Breakpoint {
	return g.breakPoints
}

func (g *Debugger) SourceCodeLines() []string {
	return g.sourceCodeLines
}

func (g *Debugger) InRootState() bool {
	return g.State() == ""
}

func (g *Debugger) StateName() string {
	frame := g.vm.CurrentFM()
	stateName := "global code"
	if f := frame.GetFunction(); f != nil {
		stateName = f.GetActualName()
	}
	return stateName
}

func (g *Debugger) State() string {
	frame := g.vm.CurrentFM()
	state := ""
	if f := frame.GetFunction(); f != nil {
		state = f.GetUUID()
	}
	return state
}

func (g *Debugger) Codes() []*Code {
	return g.codes[g.State()]
}

func (g *Debugger) VM() *VirtualMachine {
	return g.vm
}

func (g *Debugger) Description() string {
	return g.description
}

func (g *Debugger) ResetDescription() {
	g.description = ""
}

func (g *Debugger) GetCode(codeIndex int) *Code {
	return g.Codes()[codeIndex]
}

func (g *Debugger) GetLineFirstCode(lineIndex int) (*Code, int, string) {
	if codeState, ok := g.linesFirstCodeAndStateMap[lineIndex]; ok {
		return g.GetCode(codeState.codeIndex), codeState.codeIndex, codeState.state
	} else {
		return nil, -1, ""
	}
}

func (g *Debugger) AddObserveBreakPoint(expr string) error {
	_, _, err := g.Compile(expr)
	if err != nil {
		return errors.Wrap(err, "add observe breakpoint error")
	}
	v, _ := g.EvalExpression(expr)
	if v == nil || v.Value == nil {
		v = undefined
	}
	g.observeBreakPointExpressions[expr] = v
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
	if _, ok := g.observeBreakPointExpressions[expr]; ok {
		delete(g.observeBreakPointExpressions, expr)
		return nil
	}

	return utils.Errorf("expression [%s] not in observe expression", expr)
}

func (g *Debugger) GetAllObserveExpressions() map[string]*Value {
	return g.observeExpressions
}

func (g *Debugger) addBreakPoint(disposable bool, codeIndex, lineIndex int, conditionCode, state string) {
	g.breakPoints = append(g.breakPoints, NewBreakPoint(disposable, codeIndex, lineIndex, conditionCode, state))
}

func (g *Debugger) removeDisposableBreakPoints(indexs []int) {
	if len(g.breakPoints) == 0 || len(indexs) == 0 {
		return
	}
	indexs = funk.UniqInt(indexs)

	newBreakpoints := make([]*Breakpoint, 0, len(g.breakPoints)-len(indexs))
	for index, breakpoint := range g.breakPoints {
		if !breakpoint.Disposable || !funk.ContainsInt(indexs, index) {
			newBreakpoints = append(newBreakpoints, g.breakPoints[index])
		}
	}
	g.breakPoints = newBreakpoints
}

func (g *Debugger) SetBreakPoint(disposable bool, lineIndex int) error {
	code, codeIndex, state := g.GetLineFirstCode(lineIndex)
	if code == nil {
		return utils.Errorf("Can't set breakPoint in line %d", lineIndex)
	} else {
		g.addBreakPoint(disposable, codeIndex, lineIndex, "", state)
	}
	return nil
}

func (g *Debugger) SetNormalBreakPoint(lineIndex int) error {
	return g.SetBreakPoint(false, lineIndex)
}

func (g *Debugger) SetCondtionalBreakPoint(lineIndex int, conditonCode string) error {
	code, codeIndex, state := g.GetLineFirstCode(lineIndex)
	if code == nil {
		return utils.Errorf("Can't set breakPoint in line %d", lineIndex)
	} else {
		// 如果编译失败,则不应该设置断点
		_, _, err := g.Compile(conditonCode)
		if err != nil {
			return errors.Wrap(err, "Set condtional breakpoint error")
		}
		g.addBreakPoint(false, codeIndex, lineIndex, conditonCode, state)
	}
	return nil
}

func (g *Debugger) ClearAllBreakPoints() {
	g.breakPoints = make([]*Breakpoint, 0)
}

func (g *Debugger) ClearBreakpointsInLine(lineIndex int) {
	g.breakPoints = funk.Filter(g.breakPoints, func(breakpoint *Breakpoint) bool {
		return breakpoint.LineIndex != lineIndex
	}).([]*Breakpoint)
}

func (g *Debugger) EnableAllBreakPoints() {
	for _, breakpoint := range g.breakPoints {
		breakpoint.On = true
	}
}

func (g *Debugger) EnableBreakpointsInLine(lineIndex int) {
	for _, breakpoint := range g.breakPoints {
		if breakpoint.LineIndex == lineIndex {
			breakpoint.On = true
		}
	}
}

func (g *Debugger) DisableAllBreakPoints() {
	for _, breakpoint := range g.breakPoints {
		breakpoint.On = false
	}
}

func (g *Debugger) DisableBreakpointsInLine(lineIndex int) {
	for _, breakpoint := range g.breakPoints {
		if breakpoint.LineIndex == lineIndex {
			breakpoint.On = false
		}
	}
}

func (g *Debugger) StepNext() error {
	g.nextState = NewStepStateWithLineIndex(g.linePointer, g.State())
	return nil
}

func (g *Debugger) StepIn() error {
	g.GetLineFirstCode(g.linePointer)
	g.stepInState = NewStepStateWithLineIndex(g.linePointer, g.State())
	return nil
}

func (g *Debugger) StepOut() error {
	if g.stepInStack.Len() > 0 {
		g.stepOut = true
		return nil
	} else {
		return utils.Errorf("Can't not step out")
	}
}

// func (g *Debugger) HandleForStepNextJmp() {
// 	// g.addBreakPoint(true, g.jmpIndex, code.StartLineNumber, "", g.State())
// 	g.jmpIndex = -1
// 	g.nextState = nil
// 	g.Callback()
// }

func (g *Debugger) HandleForStepNext() {
	// g.addBreakPoint(true, codeIndex, g.linePointer, "", state)
	g.nextState = nil
	g.Callback()
}

func (g *Debugger) HandleForStepIn() {
	// g.addBreakPoint(true, codeIndex, g.linePointer, "", state)
	g.stepInState = nil
	g.Callback()
}

func (g *Debugger) HandleForStepOut() {
	// g.addBreakPoint(true, codeIndex, g.linePointer, "", state)
	g.stepOut = false
	g.Callback()
}

func (g *Debugger) BreakPointCallback(codeIndex int) {
	state := g.State()
	code := g.GetCode(codeIndex)
	g.codePointer = codeIndex
	g.linePointer = code.StartLineNumber

	defer func() {
		// 如果调用yak函数，则push stepIn栈
		if code.Opcode == OpCall || code.Opcode == OpAsyncCall {
			v := g.vm.CurrentFM().peekN(code.Unary)
			if v.IsYakFunction() {
				g.stepInStack.Push(NewStepStateWithCodeIndex(codeIndex, state))
			}
		}
	}()

	// 如果stepNext, stepIn, stepOut, 则一直运行直到触发回调
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

	// pop stepIn栈
	if g.stepInStack.Len() > 0 {
		stepState := g.stepInStack.Peek().(*StepState)

		if stepState.state == state && g.codePointer > stepState.codeIndex {
			g.stepInStack.Pop()
			// 如果debugger想要步出且执行到了call后面的opcode，则应该回调
			if g.stepOut {
				g.HandleForStepOut()
			}
		}
	}

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
				g.Callback()
				return
			}
		}
	}

	triggered := false
	removeBreakPointIndex := make([]int, 0)

	// 如果存在于断点列表中，则回调
	for index, breakpoint := range g.breakPoints {

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
			if breakpoint.ConditionCode != "" {
				value, err := g.EvalExpression(breakpoint.ConditionCode)

				// 如果条件不成立,则不断点
				if err != nil || value.False() {
					continue
				}

				g.description = fmt.Sprintf("Trigger condtional breakpoint [%s] at line %d in %s", breakpoint.ConditionCode, g.linePointer, g.StateName())
			} else {
				// 普通断点
				g.description = fmt.Sprintf("Trigger normal breakpoint at line %d in %s", g.linePointer, g.StateName())
			}
			g.currentBreakPoint = breakpoint

			triggered = true

			// 触发临时断点后删除
			if breakpoint.Disposable {
				removeBreakPointIndex = append(removeBreakPointIndex, index)
			}
			break
		}
	}

	if triggered {
		// 如果已经触发了其他断点，则删除该点上的所有临时断点
		for index, breakpoint := range g.breakPoints {
			if breakpoint.CodeIndex == codeIndex && breakpoint.Disposable && breakpoint != g.currentBreakPoint {
				removeBreakPointIndex = append(removeBreakPointIndex, index)
			}
		}
	}

	if len(removeBreakPointIndex) > 0 {
		g.removeDisposableBreakPoints(removeBreakPointIndex)
	}

	if triggered {
		g.Callback()
	}

}

func (g *Debugger) Callback() {
	g.Add()
	defer g.WaitGroupDone()

	// 清空临时断点的描述
	if g.currentBreakPoint != nil && g.currentBreakPoint.Disposable {
		g.description = ""
	}

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

func (g *Debugger) Compile(code string) (*Frame, CompilerWrapperInterface, error) {
	var err error
	frame := NewSubFrame(g.VM().CurrentFM())
	frame.EnableDebuggerEval()
	sym, err := frame.CurrentScope().GetSymTable().GetRoot()
	if err != nil {
		return nil, nil, errors.Wrap(err, "find symboltable error")
	}

	YakDebugCompiler = YakDebugCompiler.NewWithSymbolTable(sym)
	YakDebugCompiler.Compiler(code)
	exist, err := YakDebugCompiler.GetNormalErrors()
	if exist {
		return frame, nil, errors.Wrap(err, "compile code error")
	}
	return frame, YakDebugCompiler, nil
}

func (g *Debugger) EvalExpression(expr string) (*Value, error) {
	var err error

	frame, compiler, err := g.Compile(expr)
	if err != nil {
		return nil, errors.Wrap(err, "eval code error")
	}

	opcode := compiler.GetOpcodes()
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
