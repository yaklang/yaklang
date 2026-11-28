package mutate

import (
	"bytes"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

type poolCaptureServer struct {
	addr string

	mu        sync.Mutex
	requests  [][]byte
	response  []byte
	perClient time.Duration
}

func newPoolCaptureServer(t *testing.T, response []byte) *poolCaptureServer {
	t.Helper()

	s := &poolCaptureServer{response: response, perClient: 300 * time.Millisecond}
	host, port := utils.DebugMockTCPEx(func(ctx context.Context, lis net.Listener, conn net.Conn) {
		defer conn.Close()

		var received []byte
		buf := make([]byte, 8192)
		for {
			_ = conn.SetReadDeadline(time.Now().Add(s.perClient))
			n, err := conn.Read(buf)
			if n > 0 {
				received = append(received, buf[:n]...)
			}
			if err != nil {
				break
			}
		}
		if len(received) > 0 {
			s.mu.Lock()
			s.requests = append(s.requests, received)
			s.mu.Unlock()
		}
		_, _ = conn.Write(s.response)
	})
	s.addr = utils.HostPort(host, port)
	return s
}

func (s *poolCaptureServer) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = nil
}

func (s *poolCaptureServer) onlyRequest(t *testing.T) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	require.Len(t, s.requests, 1, "expect exactly one request on the wire")
	return string(s.requests[0])
}

func runPoolOnce(t *testing.T, s *poolCaptureServer, packet string, noFix bool) *HttpResult {
	t.Helper()

	host, port, err := utils.ParseStringToHostPort(s.addr)
	require.NoError(t, err)

	results, err := _httpPool(
		[][]byte{[]byte(packet)},
		WithPoolOpt_RawMode(true),
		WithPoolOpt_Addr(host, false),
		WithPoolOpt_Port(port),
		WithPoolOpt_Timeout(3),
		WithPoolOpt_RetryTimes(0),
		WithPoolOpt_noFixContentLength(noFix),
	)
	require.NoError(t, err)

	var got *HttpResult
	for result := range results {
		require.NotNil(t, result)
		require.Nil(t, result.Error)
		got = result
	}
	require.NotNil(t, got, "no result returned from http pool")
	return got
}

// TestPoolNoFixContentLengthKeepsRequestAndWireResponse 校验 http pool（WebFuzzer 引擎）在
// 「不修复数据包」时的行为：请求原样发出，响应保留服务端原始字节，且响应元信息仍然可用。
func TestPoolNoFixContentLengthKeepsRequestAndWireResponse(t *testing.T) {
	body, err := utils.GzipCompress([]byte("hello gzip world"))
	require.NoError(t, err)
	wireResponse := append([]byte("HTTP/1.1 200 OK\r\nContent-Encoding: gzip\r\nContent-Type: text/plain\r\nContent-Length: "),
		[]byte(utils.InterfaceToString(len(body))+"\r\n\r\n")...)
	wireResponse = append(wireResponse, body...)

	srv := newPoolCaptureServer(t, wireResponse)

	packet := func() string {
		return "POST /chunked HTTP/1.1\r\nHost: " + srv.addr +
			"\r\nTransfer-Encoding: chunked\r\n\r\n5;foo=bar\r\nhello\r\n0\r\n\r\n"
	}

	// 不修复：请求字节完全一致，响应保留 wire 原始字节
	t.Run("no-fix", func(t *testing.T) {
		srv.reset()
		raw := packet()
		result := runPoolOnce(t, srv, raw, true)

		require.Equal(t, raw, srv.onlyRequest(t), "不修复数据包时请求必须原样发出")
		require.Equal(t, string(wireResponse), string(result.ResponseRaw), "不修复数据包时必须保留服务端原始响应")

		// 元信息仍要可用，否则 WebFuzzer 结果表的状态码/响应头/长度会全空
		require.NotNil(t, result.Response)
		require.Equal(t, 200, result.Response.StatusCode)
		require.Equal(t, "gzip", result.Response.Header.Get("Content-Encoding"))
	})

	// 默认：请求被修复，响应被解码
	t.Run("fix", func(t *testing.T) {
		srv.reset()
		raw := packet()
		result := runPoolOnce(t, srv, raw, false)

		require.NotEqual(t, raw, srv.onlyRequest(t), "默认模式应当修复请求报文")
		require.NotContains(t, string(result.ResponseRaw), "\x1f\x8b", "默认模式应当解码 Content-Encoding 响应")
		require.Contains(t, string(result.ResponseRaw), "hello gzip world")

		require.NotNil(t, result.Response)
		require.Equal(t, 200, result.Response.StatusCode)
	})
}

// TestPoolNoFixContentLengthKeepsPlainResponse 校验「不修复数据包」不会改写普通响应报文。
func TestPoolNoFixContentLengthKeepsPlainResponse(t *testing.T) {
	// Content-Length 与 body 不一致的响应
	wireResponse := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: 100\r\n\r\nhello")
	srv := newPoolCaptureServer(t, wireResponse)

	for _, noFix := range []bool{true, false} {
		srv.reset()
		result := runPoolOnce(t, srv, "GET / HTTP/1.1\r\nHost: "+srv.addr+"\r\n\r\n", noFix)
		if noFix {
			require.Equal(t, string(wireResponse), string(result.ResponseRaw))
			continue
		}
		require.True(t, bytes.Contains(result.ResponseRaw, []byte("Content-Length: 5")),
			"默认模式应当修复响应 Content-Length, got: %q", string(result.ResponseRaw))
		require.False(t, strings.Contains(string(result.ResponseRaw), "Content-Length: 100"))
	}
}
