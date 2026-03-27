package yakgrpc

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_HTTPFuzzer_HotPatch_PhaseStateIsolatedPerRequest(t *testing.T) {
	client, _ := newIsolatedHTTPFuzzerHotPatchClient(t)

	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\norig")
	})
	target := utils.HostPort(host, port)

	recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), &ypb.FuzzerRequest{
		Request: "GET /{{yak(handle)}} HTTP/1.1\r\nHost: " + target + "\r\n\r\n",
		HotPatchCode: `
handle = func(params) { return ["alpha", "beta"] }
requestIngress = func(ctx) {
    raw = string(ctx.Request)
    if raw.Contains("/alpha ") {
        ctx.SetState("marker", "alpha")
        return
    }
    if raw.Contains("/beta ") {
        ctx.SetState("marker", "beta")
        return
    }
    die("unknown request path")
}
responseProcess = func(ctx) {
    marker = ctx.State["marker"]
    if marker == "" {
        die("missing marker")
    }
    ctx.Response = poc.ReplaceHTTPPacketBody(ctx.Response, marker)
}
`,
		Concurrent: 2,
		ForceFuzz:  true,
	})
	require.NoError(t, err)

	results := collectHTTPFuzzerResponses(t, recv)
	require.Len(t, results, 2)
	assertPhaseIsolationResult(t, results, "alpha")
	assertPhaseIsolationResult(t, results, "beta")
}

func TestGRPCMUSTPASS_HTTPFuzzer_HotPatch_PhaseGlobalModuleOrder(t *testing.T) {
	client, server := newIsolatedHTTPFuzzerHotPatchClient(t)

	enableSingleGlobalHotPatchTemplate(t, client, server, `
requestIngress = func(ctx) { ctx.Request = str.Replace(string(ctx.Request), "/phase", "/phase-gin", 1) }
requestProcess = func(ctx) { ctx.Request = str.Replace(string(ctx.Request), "/phase-gin-min", "/phase-gin-min-gproc", 1) }
requestEgress = func(ctx) { ctx.Request = str.Replace(string(ctx.Request), "/phase-gin-min-gproc-mproc-meg", "/phase-gin-min-gproc-mproc-meg-geg", 1) }
responseIngress = func(ctx) { ctx.Response = str.Replace(string(ctx.Response), "origin", "origin-gin", 1) }
responseProcess = func(ctx) { ctx.Response = str.Replace(string(ctx.Response), "origin-gin-min", "origin-gin-min-gproc", 1) }
responseEgress = func(ctx) { ctx.Response = str.Replace(string(ctx.Response), "origin-gin-min-gproc-mproc-meg", "origin-gin-min-gproc-mproc-meg-geg", 1) }
`)

	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: 6\r\n\r\norigin")
	})
	target := utils.HostPort(host, port)

	recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), &ypb.FuzzerRequest{
		Request: "GET /phase HTTP/1.1\r\nHost: " + target + "\r\n\r\n",
		HotPatchCode: `
requestIngress = func(ctx) { ctx.Request = str.Replace(string(ctx.Request), "/phase-gin", "/phase-gin-min", 1) }
requestProcess = func(ctx) { ctx.Request = str.Replace(string(ctx.Request), "/phase-gin-min-gproc", "/phase-gin-min-gproc-mproc", 1) }
requestEgress = func(ctx) { ctx.Request = str.Replace(string(ctx.Request), "/phase-gin-min-gproc-mproc", "/phase-gin-min-gproc-mproc-meg", 1) }
responseIngress = func(ctx) { ctx.Response = str.Replace(string(ctx.Response), "origin-gin", "origin-gin-min", 1) }
responseProcess = func(ctx) { ctx.Response = str.Replace(string(ctx.Response), "origin-gin-min-gproc", "origin-gin-min-gproc-mproc", 1) }
responseEgress = func(ctx) { ctx.Response = str.Replace(string(ctx.Response), "origin-gin-min-gproc-mproc", "origin-gin-min-gproc-mproc-meg", 1) }
`,
		ForceFuzz: true,
	})
	require.NoError(t, err)

	results := collectHTTPFuzzerResponses(t, recv)
	require.Len(t, results, 1)
	require.Contains(t, string(results[0].RequestRaw), "/phase-gin-min-gproc-mproc-meg-geg")
	require.Contains(t, string(results[0].ResponseRaw), "origin-gin-min-gproc-mproc-meg-geg")
}

func TestGRPCMUSTPASS_HTTPFuzzer_HotPatch_PhaseMixedModeRejected(t *testing.T) {
	testCases := []struct {
		name       string
		globalCode string
		moduleCode string
	}{
		{
			name:       "global legacy module phase",
			globalCode: `beforeRequest = func(req) { return req }`,
			moduleCode: `requestIngress = func(ctx) {}`,
		},
		{
			name:       "global phase module legacy",
			globalCode: `requestIngress = func(ctx) {}`,
			moduleCode: `beforeRequest = func(req) { return req }`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client, server := newIsolatedHTTPFuzzerHotPatchClient(t)
			enableSingleGlobalHotPatchTemplate(t, client, server, tc.globalCode)

			host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
				return []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
			})
			target := utils.HostPort(host, port)

			err := execHTTPFuzzerExpectStreamError(client, &ypb.FuzzerRequest{
				Request:      "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n",
				HotPatchCode: tc.moduleCode,
				ForceFuzz:    true,
			})
			require.Error(t, err)
			require.Contains(t, err.Error(), `build webfuzzer hotpatch runtime failed`)
			require.Contains(t, err.Error(), `conflicts with module hotpatch mode`)
		})
	}
}

func newIsolatedHTTPFuzzerHotPatchClient(t *testing.T) (ypb.YakClient, *Server) {
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

func enableSingleGlobalHotPatchTemplate(t *testing.T, client ypb.YakClient, server *Server, code string) {
	t.Helper()

	ctx := utils.TimeoutContextSeconds(12)
	name := "global-" + utils.RandStringBytes(8)
	_, err := client.CreateHotPatchTemplate(ctx, &ypb.HotPatchTemplate{
		Name:    name,
		Type:    "global",
		Content: code,
	})
	require.NoError(t, err)

	_, err = server.SetGlobalHotPatchConfig(ctx, &ypb.SetGlobalHotPatchConfigRequest{
		Config: &ypb.GlobalHotPatchConfig{
			Enabled: true,
			Items: []*ypb.GlobalHotPatchTemplateRef{
				{Name: name, Type: "global"},
			},
		},
	})
	require.NoError(t, err)
}

func collectHTTPFuzzerResponses(t *testing.T, recv ypb.Yak_HTTPFuzzerClient) []*ypb.FuzzerResponse {
	t.Helper()

	var results []*ypb.FuzzerResponse
	for {
		rsp, err := recv.Recv()
		if errors.Is(err, io.EOF) {
			return results
		}
		require.NoError(t, err)
		results = append(results, rsp)
	}
}

func assertPhaseIsolationResult(t *testing.T, results []*ypb.FuzzerResponse, marker string) {
	t.Helper()

	for _, rsp := range results {
		if !strings.Contains(string(rsp.RequestRaw), "GET /"+marker+" ") {
			continue
		}
		require.Contains(t, string(rsp.ResponseRaw), marker)
		return
	}
	t.Fatalf("marker %s not found in fuzzer results", marker)
}

func execHTTPFuzzerExpectStreamError(client ypb.YakClient, req *ypb.FuzzerRequest) error {
	recv, err := client.HTTPFuzzer(utils.TimeoutContextSeconds(10), req)
	if err != nil {
		return err
	}
	_, err = recv.Recv()
	return err
}
