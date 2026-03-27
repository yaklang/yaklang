package yakgrpc

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_MITM_HotPatch_PhaseClientResponseShortCircuit(t *testing.T) {
	client, _ := newIsolatedMITMHotPatchClient(t)
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
	runMITMPhaseHotPatchSession(t, client, ctx, mitmPort, `
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

func newIsolatedMITMHotPatchClient(t *testing.T) (ypb.YakClient, *Server) {
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

func runMITMPhaseHotPatchSession(
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
	RunMITMTestServerEx(client, ctx, func(stream ypb.Yak_MITMClient) {
		stream.Send(&ypb.MITMRequest{
			Host: "127.0.0.1",
			Port: uint32(mitmPort),
		})
	}, func(stream ypb.Yak_MITMClient) {
		stream.Send(&ypb.MITMRequest{
			SetYakScript:     true,
			YakScriptContent: script,
		})
		stream.Send(&ypb.MITMRequest{
			SetAutoForward:   true,
			AutoForwardValue: autoForward,
		})
		stream.Send(&ypb.MITMRequest{
			GetCurrentHook: true,
		})
	}, func(stream ypb.Yak_MITMClient, msg *ypb.MITMResponse) {
		if !msg.GetCurrentHook || len(msg.GetHooks()) == 0 {
			return
		}
		once.Do(func() {
			go onReady(proxy)
		})
	})
}
