package yakgrpc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestGRPCMUSTPASS_MCP_MITMV2_StreamControl(t *testing.T) {
	client := isolateMITMTestSideEffects(t)
	// Keep real gRPC transport and the isolated backend. MCP owns the MITM
	// stream itself; the test never sends MITMV2 control messages directly.
	server, err := mcp.NewMCPServer(mcp.WithEnableMITMToolSet(), mcp.WithGRPCClient(&Client{YakClient: client}))
	require.NoError(t, err)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	call := func(name string, args map[string]any) {
		t.Helper()
		result, err := mcp.CallBuiltinTool(server, ctx, name, args)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.False(t, result.IsError)
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		if strings.HasSuffix(r.URL.Path, ".js") {
			w.Header().Set("Content-Type", "application/javascript")
		}
		fmt.Fprint(w, "original-body")
	}))
	defer target.Close()
	port := utils.GetRandomAvailableTCPPort()
	address := utils.HostPort("127.0.0.1", port)
	setFilter := func(data map[string]any) { call("set_mitm_filter", map[string]any{"filterData": data}) }
	setFilter(map[string]any{})
	call("start_mitm_v2", map[string]any{"host": "127.0.0.1", "port": port, "disableSystemProxy": true})
	require.NoError(t, utils.WaitConnect(address, 5))
	checkTraffic := func(path string, want int) string {
		t.Helper()
		token := uuid.NewString()
		response, err := lowhttp.HTTPWithoutRedirect(
			lowhttp.WithPacketBytes([]byte(fmt.Sprintf("GET %s?token=%s HTTP/1.1\r\nHost: %s\r\n\r\n", path, token, strings.TrimPrefix(target.URL, "http://")))),
			lowhttp.WithProxy("http://"+address),
		)
		require.NoError(t, err)
		waitMITMFlowWrites(t)
		require.Equal(t, want, yakit.QuickSearchMITMHTTPFlowCount(token), "capture count for %s", path)
		return string(lowhttp.GetHTTPPacketBody(response.RawPacket))
	}
	checkTraffic("/blocked", 1)
	setFilter(map[string]any{"excludeUri": []any{map[string]any{"matcherType": "word", "group": []string{"/blocked"}}}})
	checkTraffic("/blocked", 0)
	checkTraffic("/allowed", 1)
	setFilter(map[string]any{"excludeMIME": []any{map[string]any{"matcherType": "mime", "group": []string{"text/plain"}}}})
	checkTraffic("/allowed", 0)
	setFilter(map[string]any{})
	checkTraffic("/blocked", 1)
	setFilter(map[string]any{"filterBundledStaticJS": true})
	checkTraffic("/static/js/main.js", 0)
	setFilter(map[string]any{"filterBundledStaticJS": false})
	checkTraffic("/static/js/main.js", 1)

	// A second control uses the same owned stream and receive loop.
	call("set_current_rules", map[string]any{"rules": []any{map[string]any{
		"rule": "original-body", "result": "patched-body", "enableForResponse": true, "enableForBody": true,
	}}})
	require.Equal(t, "patched-body", checkTraffic("/allowed", 1))
	call("set_current_rules", map[string]any{"rules": []any{}})
	require.Equal(t, "original-body", checkTraffic("/allowed", 1))
}
