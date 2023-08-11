package dap

import (
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

func (d *DAPDebugger) SetBreakPoint(lineIndex int, condition, hitCondition string) error {
	return d.debugger.SetBreakPoint(lineIndex, condition, hitCondition)
}

func (d *DAPDebugger) EvalExpression(expr string, frameID int) (*yakvm.Value, error) {
	return d.debugger.EvalExpressionWithFrameID(expr, frameID)
}

func (d *DAPDebugger) GetThreads() []*Thread {
	return lo.Map(d.debugger.GetStackTraces(), func(st *yakvm.StackTraces, index int) *Thread {
		topStackTrace := st.StackTraces[0]
		return &Thread{
			Id:   int(st.ThreadID),
			Name: fmt.Sprintf("[Yak %d] %s", index, topStackTrace.Name),
		}
	})
}

func (d *DAPDebugger) GetStackTraces() []*yakvm.StackTraces {
	return d.debugger.GetStackTraces()
}

func (d *DAPDebugger) GetScopes(frameID int) map[int]*yakvm.Scope {
	return d.debugger.GetScopesByFrameID(frameID)
}

func (d *DAPDebugger) IsFinished() bool {
	return d.finished
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

		// 程序结束,发送terminated事件
		if g.Finished() {
			d.finished = true
			d.session.send(&dap.TerminatedEvent{Event: *newEvent("terminated")})
			return
		}

		// 停止事件
		session := d.session
		stopReason := g.StopReason()
		if stopReason != "" {
			frame := g.Frame()
			threadID := 0
			if frame != nil {
				threadID = int(frame.ThreadID)
			}
			event := &dap.StoppedEvent{Event: *newEvent("stopped"), Body: dap.StoppedEventBody{ThreadId: threadID, Reason: stopReason, Description: desc, AllThreadsStopped: true}}
			if stopReason == "exception" {
				event.Body.Text = g.VMPanic().Error()
			}

			session.send(event)
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
