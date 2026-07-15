package crep

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

func websocketUpgradeTestRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "http://example.com/ws?ticket=secret", nil)
	require.NoError(t, err)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	return req
}

func websocketUpgradeTestResponse(accept string, extra string) []byte {
	return []byte("HTTP/1.1 101 Switching Protocols\r\n" +
		"Connection: Upgrade\r\n" +
		"Upgrade: websocket\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n" +
		extra + "\r\n")
}

func TestSelectWebsocketUpgradeResponseAcceptsValidModification(t *testing.T) {
	req := websocketUpgradeTestRequest(t)
	accept := lowhttp.ComputeWebsocketAcceptKey(req.Header.Get("Sec-WebSocket-Key"))
	original := websocketUpgradeTestResponse(accept, "")
	candidate := websocketUpgradeTestResponse(accept, "X-Test: modified\r\n")

	selected, rsp, err := selectWebsocketUpgradeResponse(req, original, candidate)
	require.NoError(t, err)
	require.True(t, bytes.Equal(candidate, selected))
	require.Equal(t, "modified", rsp.Header.Get("X-Test"))
}

func TestSelectWebsocketUpgradeResponseFallsBackFromInvalidModification(t *testing.T) {
	req := websocketUpgradeTestRequest(t)
	accept := lowhttp.ComputeWebsocketAcceptKey(req.Header.Get("Sec-WebSocket-Key"))
	original := websocketUpgradeTestResponse(accept, "")
	candidate := websocketUpgradeTestResponse("invalid", "")

	selected, rsp, err := selectWebsocketUpgradeResponse(req, original, candidate)
	require.ErrorContains(t, err, "using upstream response")
	require.True(t, bytes.Equal(original, selected))
	require.Equal(t, accept, utils.GetHTTPHeader(rsp.Header, "Sec-WebSocket-Accept"))
}

func TestSelectWebsocketUpgradeResponseRejectsInvalidUpstreamHandshake(t *testing.T) {
	req := websocketUpgradeTestRequest(t)
	invalid := websocketUpgradeTestResponse("invalid", "")

	selected, rsp, err := selectWebsocketUpgradeResponse(req, invalid, invalid)
	require.Error(t, err)
	require.Nil(t, selected)
	require.Nil(t, rsp)
}

func TestWebsocketLogTargetOmitsQuery(t *testing.T) {
	target := websocketLogTarget(websocketUpgradeTestRequest(t))
	require.Equal(t, "example.com/ws", target)
	require.False(t, strings.Contains(target, "secret"))
}

func TestModifyWebsocketOpeningHandshakeMarksOnlyCallbackWindow(t *testing.T) {
	req := websocketUpgradeTestRequest(t)
	httpctx.SetContextValueInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest, true)
	called := false
	modifier := &WebSocketModifier{
		ResponseHijackCallback: func(gotReq *http.Request, rsp *http.Response, raw []byte) []byte {
			called = true
			require.Same(t, req, gotReq)
			require.True(t, httpctx.IsWebsocketOpeningHandshake(req))
			require.True(t, httpctx.GetContextBoolInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest))
			return raw
		},
	}
	raw := []byte("HTTP/1.1 101 Switching Protocols\r\n\r\n")
	require.Equal(t, raw, modifier.modifyWebsocketOpeningHandshake(req, nil, raw))
	require.True(t, called)
	require.False(t, httpctx.IsWebsocketOpeningHandshake(req))
	require.True(t, httpctx.GetContextBoolInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_ShouldBeHijackedFromRequest))
}
