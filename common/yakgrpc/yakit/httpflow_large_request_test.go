package yakit

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
