package yakgrpc

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp"
	rawmcp "github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const globalHotPatchProbeHeader = "X-Mcp-Global-Hotpatch"
const globalHotPatchPlainBody = "plain-body"
const globalHotPatchPatchedBody = "global-patched-body"

var globalHotPatchYakCode = `
beforeRequest = func(isHttps, originReq, req) {
    return poc.ReplaceHTTPPacketHeader(req, "` + globalHotPatchProbeHeader + `", "on")
}
afterRequest = func(isHttps, originReq, req, originRsp, rsp) {
    return poc.ReplaceHTTPPacketBody(rsp, "` + globalHotPatchPatchedBody + `")
}
`

func newMCPGlobalHotPatchTestServer(t *testing.T, yakClient ypb.YakClient) *mcp.MCPServer {
	t.Helper()
	mcpClient, ok := yakClient.(*Client)
	require.True(t, ok, "expected *Client for MCP integration test")

	s, err := mcp.NewMCPServer(
		mcp.WithEnableGlobalHotPatchToolSet(),
		mcp.WithGRPCClient(mcpClient),
	)
	require.NoError(t, err)
	return s
}

func callMCPGlobalHotPatchTool(t *testing.T, s *mcp.MCPServer, ctx context.Context, name string, args map[string]any) map[string]any {
	t.Helper()
	result, err := mcp.CallBuiltinTool(s, ctx, name, args)
	require.NoError(t, err)
	require.NotEmpty(t, result.Content)

	text, ok := result.Content[0].(rawmcp.TextContent)
	require.True(t, ok, "expected text content, got %T", result.Content[0])

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(text.Text), &payload))
	return payload
}

func mcpConfigEnabled(cfg map[string]any) bool {
	if cfg == nil {
		return false
	}
	enabled, ok := cfg["Enabled"].(bool)
	if !ok {
		// proto json omitempty: false/0 fields are omitted on default config
		return false
	}
	return enabled
}

func assertMCPGlobalHotPatchEnabled(t *testing.T, cfg map[string]any, want bool) {
	t.Helper()
	require.Equal(t, want, mcpConfigEnabled(cfg), "unexpected Enabled in config: %#v", cfg)
}

func setupGlobalHotPatchTemplate(t *testing.T, client ypb.YakClient, name string) {
	t.Helper()
	ctx := utils.TimeoutContextSeconds(12)
	_, err := client.CreateHotPatchTemplate(ctx, &ypb.HotPatchTemplate{
		Name:    name,
		Type:    "global",
		Content: globalHotPatchYakCode,
	})
	require.NoError(t, err)
}

func TestMCP_GlobalHotPatch_MITM_EnableDisableCycle(t *testing.T) {
	client, _, err := NewLocalClientAndServerWithTempDatabase(t)
	require.NoError(t, err)

	ctx := utils.TimeoutContextSeconds(40)
	tplName := "mcp-global-mitm-" + utils.RandStringBytes(8)
	setupGlobalHotPatchTemplate(t, client, tplName)

	t.Cleanup(func() {
		_, _ = client.ResetGlobalHotPatchConfig(context.Background(), &ypb.Empty{})
	})

	mcpServer := newMCPGlobalHotPatchTestServer(t, client)

	var lastReq atomic.Value
	mockHost, mockPort := utils.DebugMockHTTPEx(func(req []byte) []byte {
		lastReq.Store(string(req))
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: " + utils.InterfaceToString(len(globalHotPatchPlainBody)) + "\r\n\r\n" + globalHotPatchPlainBody)
	})
	target := utils.HostPort(mockHost, mockPort)

	mitmPort := utils.GetRandomAvailableTCPPort()
	mitmCtx, mitmCancel := context.WithCancel(ctx)
	defer mitmCancel()

	mitmReady := make(chan struct{})
	var mitmReadyOnce sync.Once
	go func() {
		RunMITMTestServerEx(client, mitmCtx,
			func(stream ypb.Yak_MITMClient) {
				_ = stream.Send(&ypb.MITMRequest{
					Host: "127.0.0.1",
					Port: uint32(mitmPort),
				})
			},
			func(stream ypb.Yak_MITMClient) {
				_ = stream.Send(&ypb.MITMRequest{
					SetAutoForward:   true,
					AutoForwardValue: true,
				})
				mitmReadyOnce.Do(func() { close(mitmReady) })
			},
			func(stream ypb.Yak_MITMClient, msg *ypb.MITMResponse) {},
		)
	}()

	select {
	case <-mitmReady:
	case <-time.After(8 * time.Second):
		t.Fatal("mitm server did not become ready in time")
	}

	sendViaMITM := func() (reqSnapshot string, respBody string) {
		packet := "GET /probe HTTP/1.1\r\nHost: " + target + "\r\n\r\n"
		rsp, _, err := poc.HTTPEx(lowhttp.FixHTTPRequest([]byte(packet)), poc.WithProxy("http://"+utils.HostPort("127.0.0.1", mitmPort)))
		require.NoError(t, err)
		reqSnapshot, _ = lastReq.Load().(string)
		return reqSnapshot, string(lowhttp.GetHTTPPacketBody(rsp.RawPacket))
	}

	// 1) disabled: plain response, no global header on upstream request
	cfg := callMCPGlobalHotPatchTool(t, mcpServer, ctx, "get_global_hotpatch_config", nil)
	assertMCPGlobalHotPatchEnabled(t, cfg, false)

	reqSnap, body := sendViaMITM()
	require.Equal(t, globalHotPatchPlainBody, body)
	require.NotContains(t, reqSnap, globalHotPatchProbeHeader+": on")

	// 2) enabled via MCP: global hooks modify request/response
	enableCfg := callMCPGlobalHotPatchTool(t, mcpServer, ctx, "enable_global_hotpatch", map[string]any{
		"templateName": tplName,
	})
	assertMCPGlobalHotPatchEnabled(t, enableCfg, true)

	reqSnap, body = sendViaMITM()
	require.Equal(t, globalHotPatchPatchedBody, body)
	require.Contains(t, reqSnap, globalHotPatchProbeHeader+": on")

	// 3) disabled via MCP: back to baseline
	disableCfg := callMCPGlobalHotPatchTool(t, mcpServer, ctx, "disable_global_hotpatch", map[string]any{})
	assertMCPGlobalHotPatchEnabled(t, disableCfg, false)

	reqSnap, body = sendViaMITM()
	require.Equal(t, globalHotPatchPlainBody, body)
	require.NotContains(t, reqSnap, globalHotPatchProbeHeader+": on")
}

func TestMCP_GlobalHotPatch_WebFuzzer_EnableDisableCycle(t *testing.T) {
	client, _, err := NewLocalClientAndServerWithTempDatabase(t)
	require.NoError(t, err)

	ctx := utils.TimeoutContextSeconds(40)
	tplName := "mcp-global-fuzzer-" + utils.RandStringBytes(8)
	setupGlobalHotPatchTemplate(t, client, tplName)

	t.Cleanup(func() {
		_, _ = client.ResetGlobalHotPatchConfig(context.Background(), &ypb.Empty{})
	})

	mcpServer := newMCPGlobalHotPatchTestServer(t, client)

	var lastReq atomic.Value
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		lastReq.Store(string(req))
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: " + utils.InterfaceToString(len(globalHotPatchPlainBody)) + "\r\n\r\n" + globalHotPatchPlainBody)
	})
	target := utils.HostPort(host, port)
	requestLine := "GET /fuzzer-probe HTTP/1.1\r\nHost: " + target + "\r\n\r\n"

	runFuzzerOnce := func() (reqRaw string, respBody string) {
		recv, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
			Request:   requestLine,
			ForceFuzz: true,
		})
		require.NoError(t, err)

		rsp, err := recv.Recv()
		require.NoError(t, err)
		_, err = recv.Recv()
		require.Error(t, err)

		reqRaw, _ = lastReq.Load().(string)
		return reqRaw, string(lowhttp.GetHTTPPacketBody(rsp.GetResponseRaw()))
	}

	// 1) disabled
	cfg := callMCPGlobalHotPatchTool(t, mcpServer, ctx, "get_global_hotpatch_config", nil)
	assertMCPGlobalHotPatchEnabled(t, cfg, false)

	reqSnap, body := runFuzzerOnce()
	require.Equal(t, globalHotPatchPlainBody, body)
	require.NotContains(t, reqSnap, globalHotPatchProbeHeader+": on")

	// 2) enabled
	enableCfg := callMCPGlobalHotPatchTool(t, mcpServer, ctx, "enable_global_hotpatch", map[string]any{
		"templateName": tplName,
	})
	assertMCPGlobalHotPatchEnabled(t, enableCfg, true)

	reqSnap, body = runFuzzerOnce()
	require.Equal(t, globalHotPatchPatchedBody, body)
	require.Contains(t, reqSnap, globalHotPatchProbeHeader+": on")

	// 3) disabled
	disableCfg := callMCPGlobalHotPatchTool(t, mcpServer, ctx, "disable_global_hotpatch", map[string]any{})
	assertMCPGlobalHotPatchEnabled(t, disableCfg, false)

	reqSnap, body = runFuzzerOnce()
	require.Equal(t, globalHotPatchPlainBody, body)
	require.NotContains(t, reqSnap, globalHotPatchProbeHeader+": on")
}

func TestMCP_GlobalHotPatch_QueryTemplateList(t *testing.T) {
	client, _, err := NewLocalClientAndServerWithTempDatabase(t)
	require.NoError(t, err)

	ctx := utils.TimeoutContextSeconds(12)
	tplName := "mcp-global-list-" + utils.RandStringBytes(8)
	setupGlobalHotPatchTemplate(t, client, tplName)

	mcpServer := newMCPGlobalHotPatchTestServer(t, client)
	result, err := mcp.CallBuiltinTool(mcpServer, ctx, "query_hotpatch_template_list", map[string]any{
		"type": "global",
	})
	require.NoError(t, err)

	text, ok := result.Content[0].(rawmcp.TextContent)
	require.True(t, ok)

	var payload struct {
		Name []string `json:"Name"`
	}
	require.NoError(t, json.Unmarshal([]byte(text.Text), &payload))
	require.True(t, strings.Contains(strings.Join(payload.Name, ","), tplName))
}
