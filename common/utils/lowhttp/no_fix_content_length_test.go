package lowhttp

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

// rawCaptureServer 是一个记录“实际收到字节”的原始 TCP mock 服务。
type rawCaptureServer struct {
	addr string

	mu      sync.Mutex
	packets [][]byte
}

func newRawCaptureServer(t *testing.T) *rawCaptureServer {
	t.Helper()

	s := &rawCaptureServer{}
	host, port := utils.DebugMockTCPEx(func(ctx context.Context, lis net.Listener, conn net.Conn) {
		defer conn.Close()

		var received []byte
		buf := make([]byte, 8192)
		for {
			_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
			n, err := conn.Read(buf)
			if n > 0 {
				received = append(received, buf[:n]...)
			}
			if err != nil {
				break
			}
		}

		s.mu.Lock()
		if len(received) > 0 {
			// 健康检查/探活会建立空连接，不应计入“实际发出的报文”。
			s.packets = append(s.packets, received)
		}
		s.mu.Unlock()

		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"))
	})
	s.addr = utils.HostPort(host, port)
	return s
}

func (s *rawCaptureServer) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.packets = nil
}

func (s *rawCaptureServer) only(t *testing.T) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	require.Len(t, s.packets, 1, "expect exactly one request on the wire, got %d", len(s.packets))
	return string(s.packets[0])
}

// sendPacket 直接用 lowhttp.HTTP 发一包，并返回服务端实际收到的字节。
func (s *rawCaptureServer) sendPacket(t *testing.T, packet string, noFix bool) string {
	t.Helper()
	s.reset()

	host, port, err := utils.ParseStringToHostPort(s.addr)
	require.NoError(t, err)

	rsp, err := HTTP(
		WithHost(host),
		WithPort(port),
		WithPacketBytes([]byte(packet)),
		WithTimeout(3*time.Second),
		WithNoReadMultiResponse(true),
		WithNoFixContentLength(noFix),
	)
	require.NoError(t, err)
	require.NotNil(t, rsp)
	return s.only(t)
}

// TestHTTPNoFixContentLengthKeepsWirePacket 校验「不修复数据包」的请求侧语义：
// 开启后服务端收到的字节必须和调用方构造的报文完全一致；关闭时保持原有自动修复行为。
func TestHTTPNoFixContentLengthKeepsWirePacket(t *testing.T) {
	srv := newRawCaptureServer(t)

	tests := []struct {
		name string
		raw  string
		// wantWhenFixed 是「修复数据包」（NoFixContentLength=false）时服务端应收到的字节。
		wantWhenFixed string
	}{
		{
			name:          "content-length-mismatch",
			raw:           "POST /cl HTTP/1.1\r\nHost: %s\r\nContent-Length: 100\r\n\r\nhello world",
			wantWhenFixed: "POST /cl HTTP/1.1\r\nHost: %s\r\nContent-Length: 11\r\n\r\nhello world",
		},
		{
			name:          "content-length-duplicate",
			raw:           "POST /dupcl HTTP/1.1\r\nHost: %s\r\nContent-Length: 7\r\nContent-Length: 7\r\n\r\nhello",
			wantWhenFixed: "POST /dupcl HTTP/1.1\r\nHost: %s\r\nContent-Length: 5\r\nContent-Length: 7\r\n\r\nhello",
		},
		{
			// chunk extension 是典型会被“修复”改写的结构：重新编码后会重复拼接 extension
			name:          "chunked-with-extension",
			raw:           "POST /chunked HTTP/1.1\r\nHost: %s\r\nTransfer-Encoding: chunked\r\n\r\n5;foo=bar\r\nhello\r\n0\r\n\r\n",
			wantWhenFixed: "POST /chunked HTTP/1.1\r\nHost: %s\r\nTransfer-Encoding: chunked\r\n\r\n5;foo=bar;foo=bar\r\nhello\r\n0\r\n\r\n",
		},
		{
			// 缺少 boundary 的 multipart 会被补上 boundary 并改写 Content-Type
			name:          "multipart-without-boundary",
			raw:           "POST /mp HTTP/1.1\r\nHost: %s\r\nContent-Type: multipart/form-data\r\nContent-Length: 72\r\n\r\n--X\r\nContent-Disposition: form-data; name=\"a\"\r\n\r\n1\r\n--X--\r\n",
			wantWhenFixed: "POST /mp HTTP/1.1\r\nHost: %s\r\nContent-Type: multipart/form-data; boundary=X\r\nContent-Length: 59\r\n\r\n--X\r\nContent-Disposition: form-data; name=\"a\"\r\n\r\n1\r\n--X--\r\n",
		},
		{
			// 只有 LF 的行分隔符
			name:          "lf-only-packet",
			raw:           "POST /lf HTTP/1.1\nHost: %s\nContent-Length: 99\n\nhello",
			wantWhenFixed: "POST /lf HTTP/1.1\r\nHost: %s\r\nContent-Length: 5\r\n\r\nhello",
		},
		{
			// Host 与目标不一致时，即使修复模式也不应该改写 Host
			name:          "host-mismatch-not-rewritten",
			raw:           "POST /host HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\n\r\nhello",
			wantWhenFixed: "POST /host HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\n\r\nhello",
		},
		{
			// 没有 Content-Length 但有 body：修复模式会补一个 CL
			name:          "no-content-length-with-body",
			raw:           "POST /nocl HTTP/1.1\r\nHost: %s\r\n\r\nhello",
			wantWhenFixed: "POST /nocl HTTP/1.1\r\nHost: %s\r\nContent-Length: 5\r\n\r\nhello",
		},
		{
			// 绝对 URL 不会被改写
			name:          "absolute-url-not-rewritten",
			raw:           "GET http://%s/abs HTTP/1.1\r\nHost: %s\r\n\r\n",
			wantWhenFixed: "GET http://%s/abs HTTP/1.1\r\nHost: %s\r\n\r\n",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			packet := strings.ReplaceAll(tt.raw, "%s", srv.addr)
			wantFixed := strings.ReplaceAll(tt.wantWhenFixed, "%s", srv.addr)

			// 不修复：服务端收到的字节必须与输入完全一致
			noFixWire := srv.sendPacket(t, packet, true)
			require.Equal(t, packet, noFixWire,
				"「不修复数据包」下请求必须原样发出\nwant: %q\ngot : %q", packet, noFixWire)

			// 修复（默认行为）：服务端收到修复后的报文
			fixedWire := srv.sendPacket(t, packet, false)
			require.Equal(t, wantFixed, fixedWire,
				"默认模式下的修复行为不应改变\nwant: %q\ngot : %q", wantFixed, fixedWire)
		})
	}
}
