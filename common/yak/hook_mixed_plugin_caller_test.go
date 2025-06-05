package yak

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
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

	require.Error(t, caller.LastErr())
	require.Contains(t, caller.callers.Err.Error(), "aaaaa")
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

func TestMixCaller_LoadHotPatch(t *testing.T) {
	caller, err := NewMixPluginCaller()
	require.NoError(t, err)
	caller.SetLoadPluginTimeout(1)
	code := fmt.Sprintf(`
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	s = "%s"
}
hijackHTTPRequest = func(isHttps, url, req, forward, drop) {
	s = "%s"
}
`, ksuid.New().String(), ksuid.New().String())
	checkCallerLoads := func(funcName string, wantLength int, hashes ...string) {
		res, ok := caller.callers.table.Load(funcName)
		require.True(t, ok)
		require.Len(t, res, wantLength, "callers length not match")
		require.Lenf(t, hashes, wantLength, "hashes length not match")
		callers := res.([]*Caller)
		for i, caller := range callers {
			require.Equalf(t, hashes[i], caller.Hash, "hash not match funcName: %s index: %d", funcName, i)
		}
	}

	// load hot patch
	err = caller.LoadHotPatch(utils.TimeoutContextSeconds(2), []*ypb.ExecParamItem{}, code)
	require.NoError(t, err)
	checkCallerLoads(HOOK_HijackHTTPRequest, 1, utils.CalcSha1(code, HOOK_HijackHTTPRequest, HotPatchScriptName))
	checkCallerLoads(HOOK_MirrorHTTPFlow, 1, utils.CalcSha1(code, HOOK_MirrorHTTPFlow, HotPatchScriptName))

	// load a plugin, check if mirrorHTTPFlow has two callers
	pluginName := utils.RandStringBytes(16)
	pluginCode := fmt.Sprintf(`
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	s = "%s"
}
`, ksuid.New().String())
	err = caller.LoadPluginEx(utils.TimeoutContextSeconds(2), &schema.YakScript{
		ScriptName: pluginName,
		Content:    pluginCode,
	})
	require.NoError(t, err)
	checkCallerLoads(HOOK_HijackHTTPRequest, 1, utils.CalcSha1(code, HOOK_HijackHTTPRequest, HotPatchScriptName))
	checkCallerLoads(HOOK_MirrorHTTPFlow, 2, utils.CalcSha1(code, HOOK_MirrorHTTPFlow, HotPatchScriptName), utils.CalcSha1(pluginCode, HOOK_MirrorHTTPFlow, pluginName))

	// reload plugin, overwrite mirrorHTTPFlow
	reloadPluginCode := fmt.Sprintf(`
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	s = "%s"	
}
`, ksuid.New().String())
	err = caller.LoadPluginEx(utils.TimeoutContextSeconds(2), &schema.YakScript{
		ScriptName: pluginName,
		Content:    reloadPluginCode,
	})
	require.NoError(t, err)
	checkCallerLoads(HOOK_HijackHTTPRequest, 1, utils.CalcSha1(code, HOOK_HijackHTTPRequest, HotPatchScriptName))
	checkCallerLoads(HOOK_MirrorHTTPFlow, 2, utils.CalcSha1(code, HOOK_MirrorHTTPFlow, HotPatchScriptName), utils.CalcSha1(reloadPluginCode, HOOK_MirrorHTTPFlow, pluginName))

	// reload hot patch, overwrite hijackHTTPRequest and mirrorHTTPFlow
	reloadHotPatchCode := fmt.Sprintf(`
mirrorHTTPFlow = func(isHttps, url, req, rsp, body) {
	s = "%s"
}
hijackHTTPRequest = func(isHttps, url, req, forward, drop) {
	s = "%s"
}
`, ksuid.New().String(), ksuid.New().String())
	err = caller.LoadHotPatch(utils.TimeoutContextSeconds(2), []*ypb.ExecParamItem{}, reloadHotPatchCode)
	require.NoError(t, err)
	checkCallerLoads(HOOK_HijackHTTPRequest, 1, utils.CalcSha1(reloadHotPatchCode, HOOK_HijackHTTPRequest, HotPatchScriptName))
	// because hot patch mirrorHTTPFlow caller remove first then add, so it should be second
	checkCallerLoads(HOOK_MirrorHTTPFlow, 2, utils.CalcSha1(reloadPluginCode, HOOK_MirrorHTTPFlow, pluginName), utils.CalcSha1(reloadHotPatchCode, HOOK_MirrorHTTPFlow, HotPatchScriptName))

	// reload hot patch with empty, check if hijackHTTPRequest and mirrorHTTPFlow is removed
	err = caller.LoadHotPatch(utils.TimeoutContextSeconds(2), []*ypb.ExecParamItem{}, "")
	require.NoError(t, err)
	checkCallerLoads(HOOK_HijackHTTPRequest, 0)
	checkCallerLoads(HOOK_MirrorHTTPFlow, 1, utils.CalcSha1(reloadPluginCode, HOOK_MirrorHTTPFlow, pluginName))
}

func TestPortscanPlugin(t *testing.T) {
	l1, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l1.Close()
	l2, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l2.Close()
	port1 := l1.Addr().(*net.TCPAddr).Port
	port2 := l2.Addr().(*net.TCPAddr).Port
	go func() {
		for {
			conn, _ := l1.Accept()
			if conn != nil {
				conn.Close()
			}
		}
	}()
	go func() {
		for {
			conn, _ := l2.Accept()
			if conn != nil {
				conn.Close()
			}
		}
	}()

	tmpFile, err := os.CreateTemp("", "log*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	code := `
logFile = "` + tmpFile.Name() + `"
	` +
		`
lock = sync.NewLock()
n = 0
handle = (result) => {
	lock.Lock()
	n += 1
	file.Save(logFile, string(n))
	lock.Unlock()
}
	`
	tempName, clearFunc, err := yakit.CreateTemporaryYakScriptEx("port-scan", code)
	if err != nil {
		t.Fatal(err)
	}
	defer clearFunc()
	caller, err := NewMixPluginCaller()
	require.NoError(t, err)
	caller.SetLoadPluginTimeout(10)
	err = caller.LoadPlugin(tempName)
	require.NoError(t, err)
	caller.MirrorHTTPFlowExSync(true, true, "http://127.0.0.1:"+strconv.Itoa(port1), []byte{}, []byte{}, []byte{})
	caller.MirrorHTTPFlowExSync(true, true, "http://127.0.0.1:"+strconv.Itoa(port2), []byte{}, []byte{}, []byte{})
	caller.Wait()
	content, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	require.Equal(t, "2", string(content))
}
