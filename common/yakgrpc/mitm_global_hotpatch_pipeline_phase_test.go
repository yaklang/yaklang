package yakgrpc

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestLoadHotPatchRejectsMixedPhaseAndLegacyHooks(t *testing.T) {
	caller, err := yak.NewMixPluginCaller()
	require.NoError(t, err)

	err = caller.LoadHotPatch(context.Background(), nil, `
beforeRequest = func(req) {
    return req
}
requestIngress = func(ctx) {
}
`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "mixed legacy hotpatch hooks and phase hooks")
}

func TestDetectHotPatchRuntimeModeUsesStaticScan(t *testing.T) {
	mode, err := yak.DetectHotPatchRuntimeMode(context.Background(), `
die("top-level should not execute during mode detect")
beforeRequest = func(req) {
    return req
}
`)
	require.NoError(t, err)
	require.Equal(t, yak.HotPatchRuntimeModeLegacy, mode)
}

func TestDetectHotPatchRuntimeModeTracksFinalHookBindings(t *testing.T) {
	mode, err := yak.DetectHotPatchRuntimeMode(context.Background(), `
handler = func(req) { return req }
beforeRequest = handler
beforeRequest = "not-a-hook"
requestIngress = func(ctx) {}
`)
	require.NoError(t, err)
	require.Equal(t, yak.HotPatchRuntimeModePhase, mode)
}

func TestMitmGlobalHotPatchPipelineRequestPhaseOrder(t *testing.T) {
	globalCaller := newPhaseHotPatchCaller(t, `
requestIngress = func(ctx) {
    ctx.Request = str.Replace(string(ctx.Request), "/phase", "/phase-gin", 1)
}
requestProcess = func(ctx) {
    ctx.Request = str.Replace(string(ctx.Request), "/phase-gin-min", "/phase-gin-min-gproc", 1)
}
requestEgress = func(ctx) {
    ctx.Request = str.Replace(string(ctx.Request), "/phase-gin-min-gproc-mproc-meg", "/phase-gin-min-gproc-mproc-meg-geg", 1)
}
`)
	moduleCaller := newPhaseHotPatchCaller(t, `
requestIngress = func(ctx) {
    ctx.Request = str.Replace(string(ctx.Request), "/phase-gin", "/phase-gin-min", 1)
}
requestProcess = func(ctx) {
    ctx.Request = str.Replace(string(ctx.Request), "/phase-gin-min-gproc", "/phase-gin-min-gproc-mproc", 1)
}
requestEgress = func(ctx) {
    ctx.Request = str.Replace(string(ctx.Request), "/phase-gin-min-gproc-mproc", "/phase-gin-min-gproc-mproc-meg", 1)
}
`)

	pipeline := newMitmGlobalHotPatchPipeline(context.Background(), moduleCaller, globalCaller, nil)
	_, version, _ := yakit.GetGlobalHotPatchVersionAndCode()
	pipeline.loadedVersion = version

	req := []byte("GET /phase HTTP/1.1\r\nHost: example.com\r\n\r\n")
	phaseCtx := pipeline.CallBeforeRequestWithCtx(context.Background(), false, "http://example.com/phase", req, req)
	require.NotNil(t, phaseCtx)
	require.Contains(t, string(phaseCtx.Request), "/phase-gin-min-gproc-mproc-meg-geg")
}

func TestMitmGlobalHotPatchPipelineSupportsClientResponseAction(t *testing.T) {
	globalCaller := newPhaseHotPatchCaller(t, `
requestEgress = func(ctx) {
    ctx.SetClientResponse("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
}
`)
	moduleCaller, err := yak.NewMixPluginCaller()
	require.NoError(t, err)

	pipeline := newMitmGlobalHotPatchPipeline(context.Background(), moduleCaller, globalCaller, nil)
	_, version, _ := yakit.GetGlobalHotPatchVersionAndCode()
	pipeline.loadedVersion = version

	req := []byte("GET /phase HTTP/1.1\r\nHost: example.com\r\n\r\n")
	phaseCtx := pipeline.CallBeforeRequestWithCtx(context.Background(), false, "http://example.com/phase", req, req)
	require.NotNil(t, phaseCtx)
	require.Equal(t, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok", string(phaseCtx.ClientResponse))
	require.True(t, phaseCtx.Stopped)
}

func TestMitmGlobalHotPatchPipelineStopIsPhaseLocal(t *testing.T) {
	moduleCaller := newPhaseHotPatchCaller(t, `
requestProcess = func(ctx) {
    ctx.SetState("marker", "phase-stop")
    ctx.Stop()
}
requestEgress = func(ctx) {
    die("request stop should skip request egress")
}
responseProcess = func(ctx) {
    if ctx.State["marker"] != "phase-stop" {
        die("missing response state after request stop")
    }
    ctx.Response = poc.ReplaceHTTPPacketBody(ctx.Response, ctx.State["marker"])
}
`)
	pipeline := newMitmGlobalHotPatchPipeline(context.Background(), moduleCaller, nil, nil)
	_, version, _ := yakit.GetGlobalHotPatchVersionAndCode()
	pipeline.loadedVersion = version

	carrierReq, err := http.NewRequest(http.MethodGet, "http://example.com/phase", nil)
	require.NoError(t, err)

	req := []byte("GET /phase HTTP/1.1\r\nHost: example.com\r\n\r\n")
	rsp := []byte("HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\norig")

	requestCtx := pipeline.CallBeforeRequestWithReq(context.Background(), carrierReq, false, "http://example.com/phase", req, req)
	require.Nil(t, requestCtx)

	responseCtx := pipeline.CallAfterRequestWithReq(context.Background(), carrierReq, false, "http://example.com/phase", req, req, rsp, rsp)
	require.NotNil(t, responseCtx)
	require.Contains(t, string(responseCtx.Response), "phase-stop")
}

func TestMitmGlobalHotPatchPipelineCarriesRequestLocalStateIntoFlowArchive(t *testing.T) {
	moduleCaller := newPhaseHotPatchCaller(t, `
requestIngress = func(ctx) {
    ctx.SetState("marker", "archive-ok")
}
responseProcess = func(ctx) {
    if ctx.State["marker"] != "archive-ok" {
        die("missing response state")
    }
    ctx.Response = poc.ReplaceHTTPPacketBody(ctx.Response, ctx.State["marker"])
}
flowArchive = func(ctx) {
    if ctx.State["marker"] != "archive-ok" {
        die("missing archive state")
    }
    ctx.SetTag("phase-archive", ctx.State["marker"])
}
`)
	pipeline := newMitmGlobalHotPatchPipeline(context.Background(), moduleCaller, nil, nil)
	_, version, _ := yakit.GetGlobalHotPatchVersionAndCode()
	pipeline.loadedVersion = version

	carrierReq, err := http.NewRequest(http.MethodGet, "http://example.com/phase", nil)
	require.NoError(t, err)

	req := []byte("GET /phase HTTP/1.1\r\nHost: example.com\r\n\r\n")
	rsp := []byte("HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\norig")

	requestCtx := pipeline.CallBeforeRequestWithReq(context.Background(), carrierReq, false, "http://example.com/phase", req, req)
	require.Nil(t, requestCtx)

	responseCtx := pipeline.CallAfterRequestWithReq(context.Background(), carrierReq, false, "http://example.com/phase", req, req, rsp, rsp)
	require.NotNil(t, responseCtx)
	require.Contains(t, string(responseCtx.Response), "archive-ok")

	flow := &schema.HTTPFlow{
		Url:     "http://example.com/phase",
		Method:  http.MethodGet,
		Path:    "/phase",
		IsHTTPS: false,
	}
	flow.SetRequest(string(req))
	flow.SetResponse(string(responseCtx.Response))

	pipeline.HijackSaveHTTPFlowWithReqEx(context.Background(), carrierReq, flow, nil, nil, nil)

	require.Contains(t, flow.Tags, "phase-archive:archive-ok")
	require.Nil(t, getMitmHotPatchPhaseContext(carrierReq))
}

func TestMutateHookCallerChainedRequestPhaseOrder(t *testing.T) {
	before, _, _, _, _, _ := yak.MutateHookCallerChained(context.Background(), yak.HotPatchChain{
		GlobalCode: `
requestIngress = func(ctx) {
    ctx.Request = str.Replace(string(ctx.Request), "/phase", "/phase-gin", 1)
}
requestProcess = func(ctx) {
    ctx.Request = str.Replace(string(ctx.Request), "/phase-gin-min", "/phase-gin-min-gproc", 1)
}
requestEgress = func(ctx) {
    ctx.Request = str.Replace(string(ctx.Request), "/phase-gin-min-gproc-mproc-meg", "/phase-gin-min-gproc-mproc-meg-geg", 1)
}
`,
		ModuleCode: `
requestIngress = func(ctx) {
    ctx.Request = str.Replace(string(ctx.Request), "/phase-gin", "/phase-gin-min", 1)
}
requestProcess = func(ctx) {
    ctx.Request = str.Replace(string(ctx.Request), "/phase-gin-min-gproc", "/phase-gin-min-gproc-mproc", 1)
}
requestEgress = func(ctx) {
    ctx.Request = str.Replace(string(ctx.Request), "/phase-gin-min-gproc-mproc", "/phase-gin-min-gproc-mproc-meg", 1)
}
`,
	}, nil)
	require.NotNil(t, before)

	req := []byte("GET /phase HTTP/1.1\r\nHost: example.com\r\n\r\n")
	newReq := before(false, nil, req)
	require.Contains(t, string(newReq), "/phase-gin-min-gproc-mproc-meg-geg")
}

func TestMutateHookCallerChainedBridgesClientResponseToMock(t *testing.T) {
	before, _, _, _, _, mock := yak.MutateHookCallerChained(context.Background(), yak.HotPatchChain{
		GlobalCode: `
requestEgress = func(ctx) {
    ctx.SetClientResponse("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
}
`,
	}, nil)
	require.NotNil(t, before)
	require.NotNil(t, mock)

	req := []byte("GET /phase HTTP/1.1\r\nHost: example.com\r\n\r\n")
	newReq := before(false, nil, req)

	var mocked []byte
	mock(false, "http://example.com/phase", newReq, func(rsp interface{}) {
		mocked = utils.InterfaceToBytes(rsp)
	})

	require.Equal(t, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok", string(mocked))
}

func newPhaseHotPatchCaller(t *testing.T, code string) *yak.MixPluginCaller {
	t.Helper()

	caller, err := yak.NewMixPluginCaller()
	require.NoError(t, err)
	require.NoError(t, caller.LoadHotPatch(context.Background(), nil, code))
	return caller
}
