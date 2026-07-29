package lowhttp

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

func TestDiscardIntermediateResponseBodyPreservesPackets(t *testing.T) {
	body := bytes.Repeat([]byte("yak-response-body"), 4*1024)
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/octet-stream")
		writer.Header().Set("Content-Length", strconv.Itoa(len(body)))
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(body)
	})
	require.NoError(t, utils.WaitConnect(utils.HostPort(host, port), 3))

	for _, withConnPool := range []bool{false, true} {
		t.Run(fmt.Sprintf("conn_pool_%t", withConnPool), func(t *testing.T) {
			req := &http.Request{Method: http.MethodGet}
			response, err := HTTPWithoutRedirect(
				WithRequest("GET / HTTP/1.1\r\nHost: "+utils.HostPort(host, port)+"\r\n\r\n"),
				WithNativeHTTPRequestInstance(req),
				WithConnPool(withConnPool),
				WithDiscardIntermediateResponseBody(true),
				WithMaxContentLength(1<<20),
				WithSaveHTTPFlow(false),
				WithTimeout(3*time.Second),
			)
			require.NoError(t, err)
			require.NotNil(t, response)
			if !withConnPool {
				require.NotEmpty(t, response.MultiResponseInstances)
				require.True(t, response.MultiResponseInstances[0].Body == http.NoBody)
				require.Equal(t, int64(len(body)), response.ResponseBodySize)
			}
			require.Equal(t, int64(len(body)), httpctx.GetResponseBodySize(req))
			require.Equal(t, body, GetHTTPPacketBody(response.BareResponse))
			require.Equal(t, body, GetHTTPPacketBody(response.RawPacket))

			bareContext := httpctx.GetBareResponseBytes(req)
			require.Equal(t, response.BareResponse, bareContext)
			_, bareBody := SplitHTTPHeadersAndBodyFromPacketView(response.BareResponse)
			_, contextBody := SplitHTTPHeadersAndBodyFromPacketView(bareContext)
			require.NotEmpty(t, bareBody)
			bareBody[0] ^= 0xff
			require.Equal(t, body[0], contextBody[0], "context bare packet aliases LowhttpResponse.BareResponse")
		})
	}
}

func TestBorrowConnPoolResponsePacketUsesImmutableTransportView(t *testing.T) {
	body := bytes.Repeat([]byte("borrowed-response-body"), 4*1024)
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/octet-stream")
		writer.Header().Set("Content-Length", strconv.Itoa(len(body)))
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(body)
	})
	require.NoError(t, utils.WaitConnect(utils.HostPort(host, port), 3))

	req := &http.Request{Method: http.MethodGet}
	response, err := HTTPWithoutRedirect(
		WithRequest("GET / HTTP/1.1\r\nHost: "+utils.HostPort(host, port)+"\r\n\r\n"),
		WithNativeHTTPRequestInstance(req),
		WithConnPool(true),
		WithDiscardIntermediateResponseBody(true),
		WithBorrowConnPoolResponsePacket(true),
		WithMaxContentLength(1<<20),
		WithSaveHTTPFlow(false),
		WithTimeout(3*time.Second),
	)
	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, body, GetHTTPPacketBody(response.BareResponse))
	require.Equal(t, body, GetHTTPPacketBody(response.RawPacket))

	bareContext := httpctx.GetBareResponseBytes(req)
	require.Equal(t, response.BareResponse, bareContext)
	require.Same(t, &response.BareResponse[0], &bareContext[0])
	require.NotSame(t, &response.RawPacket[0], &bareContext[0], "fixed public packet must retain independent ownership")
}

func TestBorrowConnPoolResponsePacketFallsBackForLFOnlyResponse(t *testing.T) {
	body := []byte("lf-only-response")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	host, port := utils.DebugMockHTTPExContext(ctx, func([]byte) []byte {
		return []byte(fmt.Sprintf("HTTP/1.1 200 OK\nContent-Length: %d\nX-Test: yak\n\n%s", len(body), body))
	})
	require.NoError(t, utils.WaitConnect(utils.HostPort(host, port), 3))

	req := &http.Request{Method: http.MethodGet}
	response, err := HTTPWithoutRedirect(
		WithRequest("GET / HTTP/1.1\r\nHost: "+utils.HostPort(host, port)+"\r\n\r\n"),
		WithNativeHTTPRequestInstance(req),
		WithConnPool(true),
		WithDiscardIntermediateResponseBody(true),
		WithBorrowConnPoolResponsePacket(true),
		WithMaxContentLength(1<<20),
		WithSaveHTTPFlow(false),
		WithTimeout(3*time.Second),
	)
	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, body, GetHTTPPacketBody(response.BareResponse))
	require.Equal(
		t,
		[]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\nX-Test: yak\r\n\r\n%s", len(body), body)),
		httpctx.GetBareResponseBytes(req),
	)
}

func TestBorrowConnPoolResponsePacketDoesNotChangeNonPoolOwnership(t *testing.T) {
	body := bytes.Repeat([]byte("non-pool-response"), 64)
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = writer.Write(body)
	})
	require.NoError(t, utils.WaitConnect(utils.HostPort(host, port), 3))

	req := &http.Request{Method: http.MethodGet}
	response, err := HTTPWithoutRedirect(
		WithRequest("GET / HTTP/1.1\r\nHost: "+utils.HostPort(host, port)+"\r\n\r\n"),
		WithNativeHTTPRequestInstance(req),
		WithConnPool(false),
		WithDiscardIntermediateResponseBody(true),
		WithBorrowConnPoolResponsePacket(true),
		WithMaxContentLength(1<<20),
		WithSaveHTTPFlow(false),
		WithTimeout(3*time.Second),
	)
	require.NoError(t, err)
	bareContext := httpctx.GetBareResponseBytes(req)
	require.Equal(t, response.BareResponse, bareContext)
	require.NotSame(t, &response.BareResponse[0], &bareContext[0])
}
