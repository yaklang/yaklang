package browser

import (
	"testing"

	"github.com/go-rod/rod/lib/proto"
	"github.com/stretchr/testify/require"
	"github.com/ysmood/gson"
)

func TestClipNetworkBody(t *testing.T) {
	t.Parallel()
	require.Equal(t, "short", clipNetworkBody("short"))
	long := stringsRepeat("a", networkTapBodyLimit+10)
	clipped := clipNetworkBody(long)
	require.Equal(t, networkTapBodyLimit, len(clipped))
}

func TestSkipNetworkURL(t *testing.T) {
	t.Parallel()
	require.True(t, skipNetworkURL("data:text/plain,hi"))
	require.True(t, skipNetworkURL("about:blank"))
	require.True(t, skipNetworkURL("chrome-extension://abc"))
	require.True(t, skipNetworkURL(""))
	require.False(t, skipNetworkURL("http://127.0.0.1:8181/api/me"))
}

func TestSkipNetworkResource(t *testing.T) {
	t.Parallel()
	require.True(t, skipNetworkResource(proto.NetworkResourceTypeImage))
	require.True(t, skipNetworkResource(proto.NetworkResourceTypeScript))
	require.False(t, skipNetworkResource(proto.NetworkResourceTypeXHR))
	require.False(t, skipNetworkResource(proto.NetworkResourceTypeFetch))
	require.False(t, skipNetworkResource(proto.NetworkResourceTypeDocument))
}

func TestNetworkHeadersToMap(t *testing.T) {
	t.Parallel()
	h := proto.NetworkHeaders{
		"Cookie":       gson.New("sid=1"),
		"Content-Type": gson.New("application/json"),
	}
	got := networkHeadersToMap(h)
	require.Equal(t, "sid=1", got["Cookie"])
	require.Equal(t, "application/json", got["Content-Type"])
}

func TestNetworkTapDrainEmpty(t *testing.T) {
	t.Parallel()
	p := &BrowserPage{}
	require.Empty(t, p.DrainNetworkTap())
}

func TestNetworkTapFlushRedirectKeepsOriginalPOST(t *testing.T) {
	t.Parallel()
	tap := &networkTap{pending: map[proto.NetworkRequestID]*pendingNetworkReq{}}
	id := proto.NetworkRequestID("req-1")
	tap.onRequest(&proto.NetworkRequestWillBeSent{
		RequestID: id,
		Type:      proto.NetworkResourceTypeDocument,
		Request: &proto.NetworkRequest{
			URL:      "http://app.example/login",
			Method:   "POST",
			PostData: "username=admin",
			Headers:  proto.NetworkHeaders{"Content-Type": gson.New("application/x-www-form-urlencoded")},
		},
	})
	tap.onRequest(&proto.NetworkRequestWillBeSent{
		RequestID: id,
		Type:      proto.NetworkResourceTypeDocument,
		Request: &proto.NetworkRequest{
			URL:    "http://app.example/app",
			Method: "GET",
		},
		RedirectResponse: &proto.NetworkResponse{Status: 302, URL: "http://app.example/login"},
	})
	got := tap.drain()
	require.GreaterOrEqual(t, len(got), 1)
	require.Equal(t, "POST", got[0]["method"])
	require.Equal(t, "http://app.example/login", got[0]["url"])
	require.Equal(t, 302, got[0]["status"])
	require.Contains(t, got[0]["body"], "username=admin")
}

func stringsRepeat(s string, n int) string {
	b := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		b = append(b, s...)
	}
	return string(b)
}
