package yakit

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

func withGlobalMaxContentLength(t *testing.T, limit uint64) {
	t.Helper()
	prev := consts.GetGlobalMaxContentLength()
	consts.SetGlobalMaxContentLength(limit)
	t.Cleanup(func() {
		consts.SetGlobalMaxContentLength(prev)
	})
}

func TestGetMaxHTTPFlowRequestBodyInDBBytes_FollowsGlobal(t *testing.T) {
	t.Run("follows_global", func(t *testing.T) {
		withGlobalMaxContentLength(t, 50*1024*1024)
		require.Equal(t, 50*1024*1024, GetMaxHTTPFlowRequestBodyInDBBytes())
	})
	t.Run("fallback_when_unset", func(t *testing.T) {
		withGlobalMaxContentLength(t, 0)
		require.Equal(t, defaultHTTPFlowRequestBodyInDBBytes, GetMaxHTTPFlowRequestBodyInDBBytes())
	})
	t.Run("follows_lower_global", func(t *testing.T) {
		withGlobalMaxContentLength(t, 64*1024)
		require.Equal(t, 64*1024, GetMaxHTTPFlowRequestBodyInDBBytes())
	})
}

func TestSpillLargeHTTPFlowRequestIfNeeded_Small(t *testing.T) {
	withGlobalMaxContentLength(t, 10*1024*1024)
	packet := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\nhello")
	res, err := spillLargeHTTPFlowRequestIfNeeded(packet)
	require.NoError(t, err)
	require.False(t, res.IsTooLarge)
	require.Equal(t, packet, res.StoredPacket)
	require.Equal(t, 5, res.OriginalBodyLen)
}

func TestSpillLargeHTTPFlowRequestIfNeeded_Large(t *testing.T) {
	const limit = 64 * 1024
	withGlobalMaxContentLength(t, limit)
	body := strings.Repeat("A", limit+1024)
	packet := []byte("POST /upload HTTP/1.1\r\nHost: example.com\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body)
	res, err := spillLargeHTTPFlowRequestIfNeeded(packet)
	require.NoError(t, err)
	require.True(t, res.IsTooLarge)
	require.NotEmpty(t, res.HeaderFile)
	require.NotEmpty(t, res.BodyFile)
	defer os.Remove(res.HeaderFile)
	defer os.Remove(res.BodyFile)

	require.Less(t, len(res.StoredPacket), len(packet))
	require.Contains(t, string(res.StoredPacket), "request too large")
	require.Contains(t, string(res.StoredPacket), "POST /upload")

	rawBody, err := os.ReadFile(res.BodyFile)
	require.NoError(t, err)
	require.Equal(t, body, string(rawBody))
}

func TestRebuildFlatSpillRequestPacket(t *testing.T) {
	const limit = 64 * 1024
	withGlobalMaxContentLength(t, limit)
	body := strings.Repeat("A", limit+1024)
	packet := []byte("PUT /upload HTTP/1.1\r\nHost: example.com\r\nContent-Type: application/octet-stream\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\nX-Edit: before\r\n\r\n" + body)
	spill, err := spillLargeHTTPFlowRequestIfNeeded(packet)
	require.NoError(t, err)
	require.True(t, spill.IsTooLarge)
	require.True(t, IsFlatSpillRequestPacket(spill.StoredPacket))
	require.False(t, IsFlatSpillRequestPacket([]byte("prefix [[request too large(1MB), truncated]]")))
	t.Cleanup(func() {
		_ = os.Remove(spill.HeaderFile)
		_ = os.Remove(spill.BodyFile)
	})

	replacement := []byte("replacement raw body")
	replacementPath := filepath.Join(t.TempDir(), "replacement.bin")
	require.NoError(t, os.WriteFile(replacementPath, replacement, 0o644))
	edited := []byte(strings.Replace(string(spill.StoredPacket), "X-Edit: before", "X-Edit: after", 1))
	rebuilt, err := RebuildFlatSpillRequestPacket(edited, spill.BodyFile, replacementPath)
	require.NoError(t, err)
	require.False(t, IsFlatSpillRequestPacket(rebuilt))
	require.Equal(t, "after", lowhttp.GetHTTPPacketHeader(rebuilt, "X-Edit"))
	_, rebuiltBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rebuilt)
	require.Equal(t, replacement, rebuiltBody)
	require.Equal(t, strconv.Itoa(len(replacement)), lowhttp.GetHTTPPacketHeader(rebuilt, "Content-Length"))

	restored, err := RebuildFlatSpillRequestPacket(edited, spill.BodyFile, "")
	require.NoError(t, err)
	_, restoredBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(restored)
	require.Equal(t, body, string(restoredBody))

	emptyPath := filepath.Join(t.TempDir(), "empty.bin")
	require.NoError(t, os.WriteFile(emptyPath, nil, 0o644))
	empty, err := RebuildFlatSpillRequestPacket(edited, spill.BodyFile, emptyPath)
	require.NoError(t, err)
	_, emptyBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(empty)
	require.Empty(t, emptyBody)
	require.Equal(t, "0", lowhttp.GetHTTPPacketHeader(empty, "Content-Length"))
}

func TestSpillLargeHTTPFlowRequestIfNeeded_RespectsGlobalMaxContentLength(t *testing.T) {
	withGlobalMaxContentLength(t, 64*1024)
	body := strings.Repeat("D", 100*1024)
	packet := []byte("POST /upload HTTP/1.1\r\nHost: example.com\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body)
	res, err := spillLargeHTTPFlowRequestIfNeeded(packet)
	require.NoError(t, err)
	require.True(t, res.IsTooLarge)
	defer os.Remove(res.HeaderFile)
	defer os.Remove(res.BodyFile)
}

func TestSpillLargeHTTPFlowRequestIfNeeded_UnderGlobalNotSpilled(t *testing.T) {
	withGlobalMaxContentLength(t, 10*1024*1024)
	body := strings.Repeat("E", 300*1024) // 300KB < 10MB default dump size
	packet := []byte("POST /upload HTTP/1.1\r\nHost: example.com\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body)
	res, err := spillLargeHTTPFlowRequestIfNeeded(packet)
	require.NoError(t, err)
	require.False(t, res.IsTooLarge)
	require.Equal(t, packet, res.StoredPacket)
}

func TestPrepareLargeHTTPFlowRequest_Idempotent(t *testing.T) {
	const limit = 64 * 1024
	withGlobalMaxContentLength(t, limit)
	body := strings.Repeat("C", limit+2048)
	packet := []byte("POST /upload HTTP/1.1\r\nHost: example.com\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body)
	req, err := http.NewRequest("POST", "http://example.com/upload", strings.NewReader(body))
	require.NoError(t, err)

	first := PrepareLargeHTTPFlowRequest(req, packet)
	second := PrepareLargeHTTPFlowRequest(req, first)
	require.Equal(t, first, second)
	require.True(t, httpctx.GetRequestTooLarge(req))
	require.NotEmpty(t, httpctx.GetRequestTooLargeBodyFile(req))
	require.Contains(t, string(first), "request too large")
	require.Empty(t, httpctx.GetPlainRequestBytes(req), "plain request must not hold truncated display bytes for wire forwarding")
	defer os.Remove(httpctx.GetRequestTooLargeHeaderFile(req))
	defer os.Remove(httpctx.GetRequestTooLargeBodyFile(req))

	flow, err := CreateHTTPFlow(
		CreateHTTPFlowWithURL("http://example.com/upload"),
		CreateHTTPFlowWithRequestRaw(packet),
		CreateHTTPFlowWithRequestIns(req),
		CreateHTTPFlowWithResponseRaw([]byte("HTTP/1.1 200 OK\r\n\r\nok")),
	)
	require.NoError(t, err)
	require.True(t, flow.IsTooLargeRequest)
	require.Equal(t, httpctx.GetRequestTooLargeBodyFile(req), flow.TooLargeRequestBodyFile)
}

func TestSyncLargeHTTPFlowFlagsFromStoredPacket(t *testing.T) {
	bodyLen := int64(531374322)
	req := fmt.Sprintf("POST /upload HTTP/1.1\r\nHost: 127.0.0.1:8765\r\nContent-Length: %d\r\n\r\n[[request too large(506.8MB), truncated]] use GetHTTPFlowBodyById(IsRequest=true) for full body", bodyLen)
	flow := &schema.HTTPFlow{
		Request: strconv.Quote(req),
	}
	SyncLargeHTTPFlowFlagsFromStoredPacket(flow, 0, 0)
	require.True(t, flow.IsTooLargeRequest)
	require.Equal(t, bodyLen, flow.RequestLength)

	flow2 := &schema.HTTPFlow{
		Request: "POST / HTTP/1.1\r\nHost: a\r\nContent-Length: 100\r\n\r\n[[request-too-large(1MB), truncated]]",
	}
	SyncLargeHTTPFlowFlagsFromStoredPacket(flow2, 100, 0)
	require.True(t, flow2.IsTooLargeRequest)
	require.Equal(t, int64(100), flow2.RequestLength)
}

func TestCreateHTTPFlow_LargeRequestSpill(t *testing.T) {
	const limit = 64 * 1024
	withGlobalMaxContentLength(t, limit)
	body := strings.Repeat("B", limit+4096)
	reqRaw := []byte("POST /big HTTP/1.1\r\nHost: test.local\r\n\r\n" + body)
	flow, err := CreateHTTPFlow(
		CreateHTTPFlowWithURL("http://test.local/big"),
		CreateHTTPFlowWithRequestRaw(reqRaw),
		CreateHTTPFlowWithResponseRaw([]byte("HTTP/1.1 200 OK\r\n\r\nok")),
	)
	require.NoError(t, err)
	require.True(t, flow.IsTooLargeRequest)
	require.NotEmpty(t, flow.TooLargeRequestBodyFile)
	require.NotEmpty(t, flow.TooLargeRequestHeaderFile)
	defer os.Remove(flow.TooLargeRequestBodyFile)
	defer os.Remove(flow.TooLargeRequestHeaderFile)
	require.Equal(t, int64(len(body)), flow.RequestLength)
	require.Less(t, len(flow.GetRequest()), len(body))
}
