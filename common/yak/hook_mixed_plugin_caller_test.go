package yak

import (
	"context"
	"fmt"
	"testing"
	"time"

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
