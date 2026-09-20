package dap

import (
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func TestContinueImmediatelyAfterDebuggerInit(t *testing.T) {
	d := NewDAPDebugger()
	d.session = &DebugSession{}
	d.InitWGAdd()
	callbackEntered := make(chan struct{})
	allowCallback := make(chan struct{})
	initReturned := make(chan struct{})
	g := yakvm.NewDebugger(yakvm.New(), "", nil, nil, func(g *yakvm.Debugger) {
		close(callbackEntered)
		<-allowCallback
		d.CallBack()(g)
	})
	go func() {
		d.Init()(g)
		close(initReturned)
	}()
	d.WaitInit()
	<-callbackEntered
	// Configuration/continue requests may arrive as soon as WaitInit returns,
	// before the VM has reached the callback's receive on continueCh.
	d.Continue()
	close(allowCallback)
	select {
	case <-initReturned:
	case <-time.After(5 * time.Second):
		// Release the callback even when the assertion fails.
		d.continueCh <- struct{}{}
		<-initReturned
		t.Fatal("continue was lost between debugger initialization and callback entry")
	}
}
