package lowhttp

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

// TestRedirectRequestInstanceMatchesRedirectedPacket ensures that when a
// redirect hop rewrites the method (e.g. 302 + POST → GET), the
// LowhttpResponse.RequestInstance field is rebuilt from the redirected
// request packet rather than carrying the stale original request instance
// supplied via WithNativeHTTPRequestInstance.
//
// Before the fix, WithNativeHTTPRequestInstance(originalPostReq) leaked into
// every redirect hop via the shared opts slice.  HTTPWithoutRetry saw a
// non-nil NativeHTTPRequestInstance and skipped re-parsing from the new
// request packet, so RequestInstance kept reporting "POST" while RawRequest
// correctly said "GET".
func TestRedirectRequestInstanceMatchesRedirectedPacket(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/submit":
			// 302 → /done: POST must be rewritten to GET
			w.Header().Set("Location", "/done")
			w.WriteHeader(http.StatusFound)
		case "/done":
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("X-Final", "ok")
			w.WriteHeader(http.StatusOK)
		}
	})
	require.NoError(t, utils.WaitConnect(utils.HostPort(host, port), 3))

	// Build an *http.Request the same way minimartian does — this is the
	// request instance that WithNativeHTTPRequestInstance carries.
	originalReq, err := utils.ReadHTTPRequestFromBytes([]byte(
		"POST /submit HTTP/1.1\r\n" +
			"Host: " + utils.HostPort(host, port) + "\r\n" +
			"Content-Type: application/x-www-form-urlencoded\r\n" +
			"Content-Length: 3\r\n\r\na=b"))
	require.NoError(t, err)
	require.NotNil(t, originalReq)
	require.Equal(t, http.MethodPost, originalReq.Method)

	rsp, err := HTTP(
		WithRequest([]byte(
			"POST /submit HTTP/1.1\r\n"+
				"Host: "+utils.HostPort(host, port)+"\r\n"+
				"Content-Type: application/x-www-form-urlencoded\r\n"+
				"Content-Length: 3\r\n\r\na=b")),
		WithNativeHTTPRequestInstance(originalReq),
		WithHost(host),
		WithPort(port),
		WithRedirectTimes(3),
		WithTimeoutFloat(3),
	)
	require.NoError(t, err)
	require.NotNil(t, rsp)

	// The final response should be 200 OK from /done
	require.True(t, len(rsp.RedirectRawPackets) >= 2,
		"expected at least 2 redirect flows (original + 1 hop), got %d", len(rsp.RedirectRawPackets))

	// Inspect the redirect hop (index 1, the GET /done request)
	hop := rsp.RedirectRawPackets[1]

	// RawRequest should be GET (this was already correct before the fix)
	require.Equal(t, http.MethodGet, GetHTTPRequestMethod(hop.Request),
		"RawRequest method should be GET after 302 redirect of POST")

	// RequestInstance should also be GET — this is the bug being fixed.
	require.NotNil(t, hop.RespRecord.RequestInstance,
		"RequestInstance must not be nil on redirect hop")
	require.Equal(t, http.MethodGet, hop.RespRecord.RequestInstance.Method,
		"RequestInstance method must match the redirected packet (GET), not the original POST")
}

// TestRedirectRequestInstancePreservedFor307 ensures that 307 (which
// preserves the original method) also produces a RequestInstance that
// matches the redirected packet.
func TestRedirectRequestInstancePreservedFor307(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			w.Header().Set("Location", "/next")
			w.WriteHeader(http.StatusTemporaryRedirect) // 307
		case "/next":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("X-Final", "ok")
			w.WriteHeader(http.StatusOK)
		}
	})
	require.NoError(t, utils.WaitConnect(utils.HostPort(host, port), 3))

	originalReq, err := utils.ReadHTTPRequestFromBytes([]byte(
		"POST /start HTTP/1.1\r\n" +
			"Host: " + utils.HostPort(host, port) + "\r\n" +
			"Content-Length: 3\r\n\r\na=b"))
	require.NoError(t, err)

	rsp, err := HTTP(
		WithRequest([]byte(
			"POST /start HTTP/1.1\r\n"+
				"Host: "+utils.HostPort(host, port)+"\r\n"+
				"Content-Length: 3\r\n\r\na=b")),
		WithNativeHTTPRequestInstance(originalReq),
		WithHost(host),
		WithPort(port),
		WithRedirectTimes(3),
		WithTimeoutFloat(3),
	)
	require.NoError(t, err)
	require.NotNil(t, rsp)
	require.True(t, len(rsp.RedirectRawPackets) >= 2)

	hop := rsp.RedirectRawPackets[1]
	require.Equal(t, http.MethodPost, GetHTTPRequestMethod(hop.Request),
		"307 should preserve POST method in RawRequest")
	require.NotNil(t, hop.RespRecord.RequestInstance)
	require.Equal(t, http.MethodPost, hop.RespRecord.RequestInstance.Method,
		"307 should preserve POST method in RequestInstance")
}
