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
type DAPDebugger struct {
	debugger *yakvm.Debugger // yak debugger
	session  *DebugSession   // DAP session

	initWG sync.WaitGroup // 用于等待初始化

	hasSendTerminateEvent bool // 是否已经发送了terminate事件

	selectFrame *yakvm.Frame // 选择的frame

	finished   bool          // 是否程序已经结束
	timeout    time.Duration // 超时时间
	inCallback bool          // 是否在回调状态
	continueCh chan struct{} // 继续执行
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
		go func() {
			d.continueCh <- struct{}{}
		}()
	}
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
		log.Debug("dap debugger init")

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

		desc := g.Description()
		log.Debugf("callback: %s", desc)
		g.ResetDescription()

		if g.Finished() {
			d.finished = true
			// 如果程序已经结束且已经发送了结束事件,则不再回调
			if d.hasSendTerminateEvent {
				return
			}
		}

		select {
		case <-d.continueCh:
		case <-time.After(d.timeout):
			// todo: 超时处理
			return
		}

		if d.finished && !d.hasSendTerminateEvent {
			d.session.send(&dap.TerminatedEvent{Event: *newEvent("terminated")})
			d.hasSendTerminateEvent = true
		}
	}
}

func NewDAPDebugger() *DAPDebugger {
	return &DAPDebugger{
		continueCh:            make(chan struct{}),
		timeout:               10 * time.Minute,
		initWG:                sync.WaitGroup{},
		hasSendTerminateEvent: false,
	}
}
