package browser

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestSimpleAuthorizationPairFindsAndSwapsAResourceID(t *testing.T) {
	left := []byte("GET /users/10?project=100 HTTP/1.1\r\nHost: example.test\r\n\r\n")
	right := []byte("GET /users/20?project=200 HTTP/1.1\r\nHost: example.test\r\n\r\n")
	exported := func(id, rawURL string, packet []byte) extensionAuthorizationRequestExport {
		return extensionAuthorizationRequestExport{
			ID: id, URL: rawURL, RawRequestBase64: base64.StdEncoding.EncodeToString(packet),
		}
	}
	inspection, candidates, err := extensionAuthorizationCandidateValues(
		exported("left", "http://example.test/users/10?project=100", left),
		exported("right", "http://example.test/users/20?project=200", right),
		left,
		right,
	)
	require.NoError(t, err)
	require.Equal(t, "GET", inspection.Method)
	require.Len(t, candidates, 2)
	rewrittenPath, err := extensionAuthorizationReplace(left, "http://example.test/users/10?project=100", candidates[0], candidates[0].right)
	require.NoError(t, err)
	_, rewrittenURI, _ := lowhttp.GetHTTPPacketFirstLine(rewrittenPath)
	require.Equal(t, "/users/20?project=100", rewrittenURI)

	rewritten, err := extensionAuthorizationReplace(left, "http://example.test/users/10?project=100", candidates[1], candidates[1].right)
	require.NoError(t, err)
	require.Equal(t, "200", lowhttp.GetHTTPRequestQueryParam(rewritten, "project"))

	var task struct {
		extensionAuthorizationPairInput
		Side string `json:"side"`
	}
	require.NoError(t, decodeExtensionAuthorizationJSON(json.RawMessage(
		`{"left":{"deviceId":"a","tabId":1},"right":{"deviceId":"b","tabId":2},"side":"left"}`,
	), &task))
	require.Equal(t, "b", task.Right.DeviceID)
}
