package yakgrpc

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_MITMV2_HotPatch_PhaseRequestResponseState(t *testing.T) {
	client, _ := newIsolatedMITMV2HotPatchClient(t)
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(20))
	defer cancel()

	reqCh := make(chan []byte, 1)
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		reqCh <- append([]byte(nil), req...)
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\norig")
	})
	target := fmt.Sprintf("http://%s/before", utils.HostPort(host, port))
	mitmPort := utils.GetRandomAvailableTCPPort()

	resultCh := make(chan *mitmPhaseRequestResult, 1)
	runMITMV2PhaseHotPatchSession(t, client, ctx, mitmPort, `
requestIngress = func(ctx) {
    ctx.Request = str.Replace(string(ctx.Request), "/before", "/after", 1)
    ctx.SetState("marker", "phase-ok")
}
requestProcess = func(ctx) {
    if ctx.State["marker"] != "phase-ok" {
        die("missing request state")
    }
    ctx.Request = poc.ReplaceHTTPPacketHeader(ctx.Request, "X-Phase-Req", ctx.State["marker"])
}
responseProcess = func(ctx) {
    if ctx.State["marker"] != "phase-ok" {
        die("missing response state")
    }
    ctx.Response = poc.ReplaceHTTPPacketBody(ctx.Response, ctx.State["marker"])
}
`, true, func(proxy string) {
		rsp, _, err := poc.DoGET(target, poc.WithProxy(proxy), poc.WithSave(false))
		resultCh <- &mitmPhaseRequestResult{response: rsp, err: err}
		cancel()
	})

	result := <-resultCh
	require.NoError(t, result.err)
	req := <-reqCh
	require.Contains(t, string(req), "GET /after HTTP/1.1")
	require.Contains(t, string(req), "X-Phase-Req: phase-ok")
	require.Contains(t, string(result.response.RawPacket), "phase-ok")
}

func TestGRPCMUSTPASS_MITMV2_HotPatch_PhaseGlobalModuleOrder(t *testing.T) {
	client, server := newIsolatedMITMV2HotPatchClient(t)
	enableSingleGlobalHotPatchTemplate(t, client, server, `
requestIngress = func(ctx) { ctx.Request = str.Replace(string(ctx.Request), "/phase", "/phase-gin", 1) }
requestProcess = func(ctx) { ctx.Request = str.Replace(string(ctx.Request), "/phase-gin-min", "/phase-gin-min-gproc", 1) }
requestEgress = func(ctx) { ctx.Request = str.Replace(string(ctx.Request), "/phase-gin-min-gproc-mproc-meg", "/phase-gin-min-gproc-mproc-meg-geg", 1) }
responseIngress = func(ctx) { ctx.Response = str.Replace(string(ctx.Response), "origin", "origin-gin", 1) }
responseProcess = func(ctx) { ctx.Response = str.Replace(string(ctx.Response), "origin-gin-min", "origin-gin-min-gproc", 1) }
responseEgress = func(ctx) { ctx.Response = str.Replace(string(ctx.Response), "origin-gin-min-gproc-mproc-meg", "origin-gin-min-gproc-mproc-meg-geg", 1) }
`)

	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(20))
	defer cancel()

	reqCh := make(chan []byte, 1)
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		reqCh <- append([]byte(nil), req...)
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: 6\r\n\r\norigin")
	})
	target := fmt.Sprintf("http://%s/phase", utils.HostPort(host, port))
	mitmPort := utils.GetRandomAvailableTCPPort()

	resultCh := make(chan *mitmPhaseRequestResult, 1)
	runMITMV2PhaseHotPatchSession(t, client, ctx, mitmPort, `
requestIngress = func(ctx) { ctx.Request = str.Replace(string(ctx.Request), "/phase-gin", "/phase-gin-min", 1) }
requestProcess = func(ctx) { ctx.Request = str.Replace(string(ctx.Request), "/phase-gin-min-gproc", "/phase-gin-min-gproc-mproc", 1) }
requestEgress = func(ctx) { ctx.Request = str.Replace(string(ctx.Request), "/phase-gin-min-gproc-mproc", "/phase-gin-min-gproc-mproc-meg", 1) }
responseIngress = func(ctx) { ctx.Response = str.Replace(string(ctx.Response), "origin-gin", "origin-gin-min", 1) }
responseProcess = func(ctx) { ctx.Response = str.Replace(string(ctx.Response), "origin-gin-min-gproc", "origin-gin-min-gproc-mproc", 1) }
responseEgress = func(ctx) { ctx.Response = str.Replace(string(ctx.Response), "origin-gin-min-gproc-mproc", "origin-gin-min-gproc-mproc-meg", 1) }
`, true, func(proxy string) {
		rsp, _, err := poc.DoGET(target, poc.WithProxy(proxy), poc.WithSave(false))
		resultCh <- &mitmPhaseRequestResult{response: rsp, err: err}
		cancel()
	})

	result := <-resultCh
	require.NoError(t, result.err)
	req := <-reqCh
	require.Contains(t, string(req), "/phase-gin-min-gproc-mproc-meg-geg")
	require.Contains(t, string(result.response.RawPacket), "origin-gin-min-gproc-mproc-meg-geg")
}

func TestGRPCMUSTPASS_MITMV2_HotPatch_PhaseClientResponseShortCircuit(t *testing.T) {
	client, _ := newIsolatedMITMV2HotPatchClient(t)
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(20))
	defer cancel()

	var called atomic.Bool
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		called.Store(true)
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: 6\r\n\r\norigin")
	})
	target := fmt.Sprintf("http://%s/phase", utils.HostPort(host, port))
	mitmPort := utils.GetRandomAvailableTCPPort()

	resultCh := make(chan *mitmPhaseRequestResult, 1)
	runMITMV2PhaseHotPatchSession(t, client, ctx, mitmPort, `
requestEgress = func(ctx) {
    ctx.SetClientResponse("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nphase")
}
`, true, func(proxy string) {
		rsp, _, err := poc.DoGET(target, poc.WithProxy(proxy), poc.WithSave(false))
		resultCh <- &mitmPhaseRequestResult{response: rsp, err: err}
		cancel()
	})

	result := <-resultCh
	require.NoError(t, result.err)
	require.False(t, called.Load())
	require.Contains(t, string(result.response.RawPacket), "phase")
}

func TestGRPCMUSTPASS_MITMV2_HotPatch_PhaseConflictFallsBackToLegacy(t *testing.T) {
	client, server := newIsolatedMITMV2HotPatchClient(t)
	enableSingleGlobalHotPatchTemplate(t, client, server, `
beforeRequest = func(isHttps, originReq, req) {
    return poc.ReplaceHTTPPacketHeader(req, "X-Legacy-Global", "1")
}
afterRequest = func(isHttps, originReq, req, originRsp, rsp) {
    return poc.ReplaceHTTPPacketBody(rsp, "legacy-rsp")
}
`)

	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(20))
	defer cancel()

	reqCh := make(chan []byte, 1)
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		reqCh <- append([]byte(nil), req...)
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\norig")
	})
	target := fmt.Sprintf("http://%s/origin", utils.HostPort(host, port))
	mitmPort := utils.GetRandomAvailableTCPPort()

	resultCh := make(chan *mitmPhaseRequestResult, 1)
	runMITMV2PhaseHotPatchSession(t, client, ctx, mitmPort, `
requestIngress = func(ctx) {
    ctx.Request = str.Replace(string(ctx.Request), "/origin", "/phase-should-not-run", 1)
}
responseProcess = func(ctx) {
    ctx.Response = poc.ReplaceHTTPPacketBody(ctx.Response, "phase-rsp")
}
`, true, func(proxy string) {
		rsp, _, err := poc.DoGET(target, poc.WithProxy(proxy), poc.WithSave(false))
		resultCh <- &mitmPhaseRequestResult{response: rsp, err: err}
		cancel()
	})

	result := <-resultCh
	require.NoError(t, result.err)
	req := <-reqCh
	require.Contains(t, string(req), "GET /origin HTTP/1.1")
	require.NotContains(t, string(req), "/phase-should-not-run")
	require.Contains(t, string(req), "X-Legacy-Global: 1")
	require.Contains(t, string(result.response.RawPacket), "legacy-rsp")
	require.NotContains(t, string(result.response.RawPacket), "phase-rsp")
}

type mitmPhaseRequestResult struct {
	response *lowhttp.LowhttpResponse
	err      error
}

func newIsolatedMITMV2HotPatchClient(t *testing.T) (ypb.YakClient, *Server) {
	t.Helper()

	client, server, err := NewLocalClientAndServerWithTempDatabase(t)
	require.NoError(t, err)
	_, err = server.ResetGlobalHotPatchConfig(context.Background(), &ypb.Empty{})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = server.ResetGlobalHotPatchConfig(context.Background(), &ypb.Empty{})
	})
	return client, server
}

func runMITMV2PhaseHotPatchSession(
	t *testing.T,
	client ypb.YakClient,
	ctx context.Context,
	mitmPort int,
	script string,
	autoForward bool,
	onReady func(proxy string),
) {
	t.Helper()

	proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
	var once sync.Once
	RunMITMV2TestServerEx(client, ctx, func(stream ypb.Yak_MITMV2Client) {
		stream.Send(&ypb.MITMV2Request{
			Host: "127.0.0.1",
			Port: uint32(mitmPort),
		})
	}, func(stream ypb.Yak_MITMV2Client) {
		stream.Send(&ypb.MITMV2Request{
			SetYakScript:     true,
			YakScriptContent: script,
		})
		stream.Send(&ypb.MITMV2Request{
			SetAutoForward:   true,
			AutoForwardValue: autoForward,
		})
		stream.Send(&ypb.MITMV2Request{
			GetCurrentHook: true,
		})
	}, func(stream ypb.Yak_MITMV2Client, msg *ypb.MITMV2Response) {
		if !msg.GetCurrentHook || len(msg.GetHooks()) == 0 {
			return
		}
		once.Do(func() {
			go onReady(proxy)
		})
	})
}
