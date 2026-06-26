package yakit

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

func TestSpillLargeHTTPFlowRequestIfNeeded_Small(t *testing.T) {
	packet := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\nhello")
	res, err := spillLargeHTTPFlowRequestIfNeeded(packet)
	require.NoError(t, err)
	require.False(t, res.IsTooLarge)
	require.Equal(t, packet, res.StoredPacket)
	require.Equal(t, 5, res.OriginalBodyLen)
}

func TestSpillLargeHTTPFlowRequestIfNeeded_Large(t *testing.T) {
	body := strings.Repeat("A", maxHTTPFlowRequestBodyInDBBytes+1024)
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

func TestPrepareLargeHTTPFlowRequest_Idempotent(t *testing.T) {
	body := strings.Repeat("C", maxHTTPFlowRequestBodyInDBBytes+2048)
	packet := []byte("POST /upload HTTP/1.1\r\nHost: example.com\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body)
	req, err := http.NewRequest("POST", "http://example.com/upload", strings.NewReader(body))
	require.NoError(t, err)

	first := PrepareLargeHTTPFlowRequest(req, packet)
	second := PrepareLargeHTTPFlowRequest(req, first)
	require.Equal(t, first, second)
	require.True(t, httpctx.GetRequestTooLarge(req))
	require.NotEmpty(t, httpctx.GetRequestTooLargeBodyFile(req))
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
	body := strings.Repeat("B", maxHTTPFlowRequestBodyInDBBytes+4096)
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
