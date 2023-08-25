package dap

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/go-dap"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

type Thread struct {
	Id   int
	Name string
}

type Source struct {
	Name    string // 文件名
	AbsPath string // 完整路径
}
type DAPDebugger struct {
	debugger *yakvm.Debugger // yak debugger
	session  *DebugSession   // DAP session

	initWG sync.WaitGroup // 用于等待初始化

	selectFrame *yakvm.Frame // 选择的frame

	finished   bool          // 是否程序已经结束
	restart    bool          // 是否需要重启
	timeout    time.Duration // 超时时间
	inCallback bool          // 是否在回调状态
	continueCh chan struct{} // 继续执行

	source *Source // 源码相关
}

func (d *DAPDebugger) InitWGAdd() {
	d.initWG.Add(1)
}

func (d *DAPDebugger) WaitInit() {
	d.initWG.Wait()
}

func (d *DAPDebugger) WaitProgramStart() {
	d.initWG.Wait()
	d.debugger.StartWGWait()
}

func (d *DAPDebugger) Continue() {
	// 如果在回调状态则写入continueCh,使callback立即返回,程序继续执行
	if d.inCallback {
		log.Debugf("[dap debugger] continue")
		go func() {
			d.continueCh <- struct{}{}
		}()
	}
}

func (d *DAPDebugger) StepIn() error {
	d.debugger.StepIn()
	d.Continue()
	return nil
}

func (d *DAPDebugger) StepOut() error {
	err := d.debugger.StepOut()
	if err != nil {
		return err
	}
	d.Continue()
	return nil
}

func (d *DAPDebugger) StepNext() error {
	d.debugger.StepNext()
	d.Continue()
	return nil
}

func (d *DAPDebugger) VMPanic() *yakvm.VMPanic {
	return d.debugger.VMPanic()
}

func (d *DAPDebugger) AddObserveBreakPoint(expr string) error {
	return d.debugger.AddObserveBreakPoint(expr)
}

func (d *DAPDebugger) RemoveObserveBreakPoint(expr string) error {
	return d.debugger.RemoveObserveBreakPoint(expr)
}

func (d *DAPDebugger) AddObserveExpression(expr string) error {
	return d.debugger.AddObserveExpression(expr)
}

func (d *DAPDebugger) RemoveObserveExpression(expr string) error {
	return d.debugger.RemoveObserveExpression(expr)
}

func (d *DAPDebugger) GetAllObserveExpressions() map[string]*yakvm.Value {
	return d.debugger.GetAllObserveExpressions()
}

func (d *DAPDebugger) ExistBreakPoint(path string, lineIndex int) (*yakvm.Breakpoint, bool) {
	return d.debugger.ExistBreakPointInLineWithSource(path, lineIndex)
}

func (d *DAPDebugger) SetBreakPoint(path string, lineIndex int, condition, hitCondition string) (int, error) {
	return d.debugger.SetBreakPointWithSource(path, lineIndex, condition, hitCondition)
}

func (d *DAPDebugger) ClearOtherBreakPoints(path string, existLines []int) {
	d.debugger.ClearOtherBreakpointsWithSource(path, existLines)
}

func (d *DAPDebugger) EvalExpression(expr string, frameID int) (*yakvm.Value, error) {
	return d.debugger.EvalExpressionWithFrameID(expr, frameID)
}

func (d *DAPDebugger) GetThreads() []*Thread {
	return lo.MapToSlice(d.debugger.GetStackTraces(), func(threadID int, st *yakvm.StackTraces) *Thread {
		topStackTrace := st.StackTraces[0]
		return &Thread{
			Id:   int(threadID),
			Name: fmt.Sprintf("[Yak %d] %s", threadID, topStackTrace.Name),
		}
	})
}

func (d *DAPDebugger) GetStackTraces() map[int]*yakvm.StackTraces {
	return d.debugger.GetStackTraces()
}

func (d *DAPDebugger) GetScopes(frameID int) map[int]*yakvm.Scope {
	return d.debugger.GetScopesByFrameID(frameID)
}

func (d *DAPDebugger) GetVariablesByReference(ref int) (interface{}, bool) {
	return d.debugger.GetVariablesByRef(ref)
}

func (d *DAPDebugger) GetVariablesReference(v interface{}) (int, bool) {
	return d.debugger.GetVariablesRef(v)
}

func (d *DAPDebugger) AddVariableRef(v interface{}) int {
	return d.debugger.AddVariableRef(v)
}

func (d *DAPDebugger) ForceSetVariableRef(id int, v interface{}) {
	d.debugger.ForceSetVariableRef(id, v)
}

func (d *DAPDebugger) CurrentThreadID() int {
	return d.debugger.CurrentThreadID()
}

func (d *DAPDebugger) CurrentFrameID() int {
	return d.debugger.CurrentFrameID()
}

func (d *DAPDebugger) IsFinished() bool {
	return d.finished
}

func (d *DAPDebugger) Restart() bool {
	return d.restart
}

func (d *DAPDebugger) SetRestart(b bool) {
	d.restart = b
}

func (d *DAPDebugger) SetDescription(desc string) {
	d.debugger.SetDescription(desc)
}

func (d *DAPDebugger) InCallbackState() {
	d.inCallback = true
}

func (d *DAPDebugger) OutCallbackState() {
	d.inCallback = false
}

func (d *DAPDebugger) Halt() error {
	// 如果已经处在回调状态则直接返回
	if d.inCallback {
		return nil
	}
	if d.finished {
		return errors.New("program finished")
	}

	d.debugger.Pause() // 设置Pause,在执行下一个opcode的时候会停止
	return nil
}

func (d *DAPDebugger) Init() func(g *yakvm.Debugger) {
	return func(g *yakvm.Debugger) {
		log.Debug("[dap debugger] init")

		d.debugger = g

		// 表示初始化完成
		d.initWG.Done()

		// 一开始先将程序挂起
		d.debugger.Callback()
	}
}

func (d *DAPDebugger) CallBack() func(g *yakvm.Debugger) {
	return func(g *yakvm.Debugger) {
		d.InCallbackState()
		defer d.OutCallbackState()

		defer g.ResetDescription()
		desc := g.Description()
		log.Debugf("[dap debugger] callback: %s", desc)

		// 停止事件
		session := d.session
		stopReason := g.StopReason()
		isPanic, isNormallyFinished := stopReason == "exception", stopReason == "finished"
		if stopReason != "" && !isNormallyFinished {
			frame := g.Frame()
			threadID := 0
			if frame != nil {
				threadID = int(frame.ThreadID)
			}
			event := &dap.StoppedEvent{Event: *newEvent("stopped"), Body: dap.StoppedEventBody{ThreadId: threadID, Reason: stopReason, Description: desc, AllThreadsStopped: true}}
			if isPanic {
				event.Body.Text = g.VMPanic().GetDataDescription()
			}

			session.send(event)
		}

		if isNormallyFinished && !d.finished {
			d.finished = true
			// 程序正常结束且不需要重启,发送terminated事件(真实client不需要发送,因为不想让client退出)
			if !d.restart && !d.session.isRealClient {
				d.session.send(&dap.TerminatedEvent{Event: *newEvent("terminated")})
			}
			return
		}

		select {
		case <-d.continueCh:
		case <-time.After(d.timeout):
			// todo: 超时处理
			return
		}

	}
}

func NewDAPDebugger() *DAPDebugger {
	return &DAPDebugger{
		continueCh: make(chan struct{}),
		timeout:    10 * time.Minute,
		initWG:     sync.WaitGroup{},
	}
}
