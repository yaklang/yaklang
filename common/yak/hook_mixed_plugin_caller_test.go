package yak

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestMixCaller_load_Plugin_Timeout(t *testing.T) {
	code := `
	ch = make(chan var)
	<- ch
`
	consts.GetGormProjectDatabase()
	tempName, clearFunc, err := yakit.CreateTemporaryYakScriptEx("mitm", code)
	if err != nil {
		t.Fatal(err)
	}
	defer clearFunc()

	caller, err := NewMixPluginCaller()
	if err != nil {
		t.Fatal(err)
	}
	caller.SetLoadPluginTimeout(1)
	loadCheckChan := make(chan struct{})

	go func() {
		err = caller.LoadPlugin(tempName)
		close(loadCheckChan)
		if err != nil {
			fmt.Println(err)
		}
	}()
	select {
	case <-time.After(5 * time.Second):
		t.Fatal("load timeout")
	case <-loadCheckChan:
	}
}

func TestMixCaller_call_Plugin_Timeout(t *testing.T) {
	code := `
	mirrorHTTPFlow = func(isHttps, url, req , rsp , body ) {
	ch = make(chan var)
	<- ch
	}
	
`
	consts.GetGormProjectDatabase()
	tempName, clearFunc, err := yakit.CreateTemporaryYakScriptEx("mitm", code)
	if err != nil {
		t.Fatal(err)
	}
	defer clearFunc()

	caller, err := NewMixPluginCaller()
	if err != nil {
		t.Fatal(err)
	}
	caller.SetLoadPluginTimeout(1)
	err = caller.LoadPlugin(tempName)
	if err != nil {
		t.Fatal(err)
	}

	caller.SetCallPluginTimeout(1)
	callCheckChan := make(chan struct{})
	go func() {
		item := []interface{}{false, "http://www.baidu.com", []byte{}, []byte{}, []byte{}}
		caller.callers.CallByNameSync(HOOK_MirrorHTTPFlow, item...)
		close(callCheckChan)
	}()

	select {
	case <-time.After(3 * time.Second):
		t.Fatal("call timeout")
	case <-callCheckChan:

	}
}

func TestMixCaller_call_Plugin_Timeout2(t *testing.T) {
	code := `
handle = result => {
	yakit.Info("开始执行")
	go fn {
		for {
			sleep(1)
			yakit.Info("执行中...")
		}
	}
	sleep(9)
	yakit.Info("执行结束了")

	return "ok"
}
`
	consts.GetGormProjectDatabase()
	tempName, clearFunc, err := yakit.CreateTemporaryYakScriptEx("port-scan", code)
	if err != nil {
		t.Fatal(err)
	}
	defer clearFunc()

	caller, err := NewMixPluginCaller()
	if err != nil {
		t.Fatal(err)
	}
	caller.SetCallPluginTimeout(5)
	caller.SetDividedContext(true)
	err = caller.LoadPlugin(tempName)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	caller.HandleServiceScanResult(nil)
	caller.Wait()
	executionDuration := time.Since(now)
	if executionDuration > 6*time.Second {
		t.Fatalf("context timeout setting might be incorrect: expected execution time <= 7s, got %v", executionDuration)
	}
}

func TestMixCaller_call_Plugin_TimeoutPanic(t *testing.T) {
	code := `
handle = result => {
	yakit.Info("开始执行")
	go fn {
		for {
			sleep(1)
			yakit.Info("执行中...")
		}
	}
	panic("aaaaa")
	yakit.Info("执行结束了")

	return "ok"
}
`
	consts.GetGormProjectDatabase()
	tempName, clearFunc, err := yakit.CreateTemporaryYakScriptEx("port-scan", code)
	if err != nil {
		t.Fatal(err)
	}
	defer clearFunc()

	caller, err := NewMixPluginCaller()
	if err != nil {
		t.Fatal(err)
	}
	caller.SetCallPluginTimeout(5)
	caller.SetDividedContext(true)
	err = caller.LoadPlugin(tempName)
	if err != nil {
		t.Fatal(err)
	}

	caller.HandleServiceScanResult(nil)
	caller.Wait()
}

func TestMixCaller_load_Plugin_Timeout_effect_call(t *testing.T) {
	code := `
	mirrorHTTPFlow = func(isHttps, url, req , rsp , body ) {
	for {
		println("loading")
		sleep(0.5)
	}
	}
	
`
	consts.GetGormProjectDatabase()
	tempName, clearFunc, err := yakit.CreateTemporaryYakScriptEx("mitm", code)
	if err != nil {
		t.Fatal(err)
	}
	defer clearFunc()

	caller, err := NewMixPluginCaller()
	if err != nil {
		t.Fatal(err)
	}
	caller.SetLoadPluginTimeout(1)
	err = caller.LoadPlugin(tempName)
	if err != nil {
		t.Fatal(err)
	}

	callCheckChan := make(chan struct{})
	go func() {
		item := []interface{}{false, "http://www.baidu.com", []byte{}, []byte{}, []byte{}}
		caller.callers.CallByNameSync(HOOK_MirrorHTTPFlow, item...)
		close(callCheckChan)
	}()

	select {
	case <-time.After(4 * time.Second):
	case <-callCheckChan:
		t.Fatal("load timeout effect call timeout")
	}
}

func TestMixCaller_Wait(t *testing.T) {
	code := `
	mirrorHTTPFlow = func(isHttps, url, req , rsp , body ) {
	for {
		println("loading")
		sleep(0.5)
	}
	}
	
`
	consts.GetGormProjectDatabase()
	tempName, clearFunc, err := yakit.CreateTemporaryYakScriptEx("mitm", code)
	if err != nil {
		t.Fatal(err)
	}
	defer clearFunc()

	caller, err := NewMixPluginCaller()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	caller.SetCtx(ctx)
	caller.SetLoadPluginTimeout(1)

	err = caller.callers.SetConcurrent(2)
	if err != nil {
		t.Fatal(err)
	}

	err = caller.LoadPlugin(tempName)
	if err != nil {
		t.Fatal(err)
	}

	item := []interface{}{false, "http://www.baidu.com", []byte{}, []byte{}, []byte{}}
	caller.callers.CallByName(HOOK_MirrorHTTPFlow, item...)

	callCheckChan := make(chan struct{})
	go func() {
		caller.Wait()
		close(callCheckChan)
	}()

	cancel()
	time.Sleep(1)
	select {
	case <-callCheckChan:
	default:
		t.Fatal("wait timeout")
	}
}

func TestMixCaller_load_Plugin_Passing_Code(t *testing.T) {
	var check bool
	server, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		check = true
		return []byte(`HTTP/1.1 200 OK

aaa`)
	})
	code := fmt.Sprintf(`
	mirrorHTTPFlow = func(isHttps, url, req , rsp , body ) {
		poc.Get("%s")
	}
	
`, fmt.Sprintf("http://%s:%d", server, port))
	caller, err := NewMixPluginCaller()
	if err != nil {
		t.Fatal(err)
	}
	caller.SetLoadPluginTimeout(1)
	err = caller.LoadPluginByName(context.Background(), "test", []*ypb.ExecParamItem{}, code)
	require.NoError(t, err)

	callCheckChan := make(chan struct{})
	go func() {
		item := []interface{}{false, "http://www.baidu.com", []byte{}, []byte{}, []byte{}}
		caller.callers.CallByNameSync(HOOK_MirrorHTTPFlow, item...)
		close(callCheckChan)
	}()

	select {
	case <-time.After(4 * time.Second):
		t.Fatal("time out")
	case <-callCheckChan:
	}
	require.True(t, check)
}
