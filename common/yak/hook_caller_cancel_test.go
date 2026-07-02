package yak

import (
	"context"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/schema"
)

func TestYakToCallerManagerCancelAllPluginsStopsExecution(t *testing.T) {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	manager := NewYakToCallerManager()
	manager.SetContext(rootCtx)
	manager.SetDividedContext(true)
	manager.SetCallPluginTimeout(30)

	script := &schema.YakScript{
		ScriptName: "cancel-test-plugin",
		Content: `
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	for i = 0; i < 1000; i++ {
		sleep(0.05)
	}
}
`,
		Type: "mitm",
	}

	err := manager.Add(rootCtx, script, map[string]any{}, script.Content, nil, "mirrorHTTPFlow")
	if err != nil {
		t.Fatalf("add plugin failed: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		manager.CallByName("mirrorHTTPFlow", true, "http://example.com", []byte("req"), []byte("rsp"), []byte("body"))
	}()

	time.Sleep(200 * time.Millisecond)
	manager.CancelAllPlugins()
	rootCancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("plugin call did not stop after CancelAllPlugins")
	}
}

func TestMixPluginCallerCancelStopsMirrorFlowDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	caller, err := NewMixPluginCaller()
	if err != nil {
		t.Fatalf("create mix caller failed: %v", err)
	}
	caller.SetCtx(ctx)

	script := &schema.YakScript{
		ScriptName: "mix-cancel-test",
		Content: `
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
}
`,
		Type: "mitm",
	}
	err = caller.LoadPluginEx(ctx, script)
	if err != nil {
		t.Fatalf("load plugin failed: %v", err)
	}

	cancel()
	caller.Cancel()

	// should return immediately without dispatching when ctx is canceled
	caller.MirrorHTTPFlowEx(false, false, "http://example.com", []byte("req"), []byte("rsp"), []byte("body"))
}

func TestMergeCancelContexts(t *testing.T) {
	parent, parentCancel := context.WithCancel(context.Background())
	child, childCancel := context.WithCancel(context.Background())
	merged, mergedCancel := mergeCancelContexts(parent, child)
	defer mergedCancel()

	childCancel()
	select {
	case <-merged.Done():
	case <-time.After(time.Second):
		t.Fatal("merged context should cancel when child cancels")
	}
	parentCancel()
}
