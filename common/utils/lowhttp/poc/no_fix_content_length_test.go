package poc

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

type rawCapture struct {
	addr    string
	mu      sync.Mutex
	packets [][]byte
}

func newRawCapture(t *testing.T) *rawCapture {
	t.Helper()
	s := &rawCapture{}
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
		if len(received) > 0 {
			s.mu.Lock()
			s.packets = append(s.packets, received)
			s.mu.Unlock()
		}
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"))
	})
	s.addr = utils.HostPort(host, port)
	return s
}

func (s *rawCapture) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.packets = nil
}

func (s *rawCapture) only(t *testing.T) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	require.Len(t, s.packets, 1)
	return string(s.packets[0])
}

// TestPocHTTPNoFixContentLengthReturnsWireRequest 校验 poc.HTTP 在「不修复数据包」时
// 返回的原始请求报文与实际上线的字节完全一致（不做 CRLF 归一化/chunked 重编码）。
func TestPocHTTPNoFixContentLengthReturnsWireRequest(t *testing.T) {
	srv := newRawCapture(t)

	raw := "POST /chunked HTTP/1.1\nHost: %s\nTransfer-Encoding: chunked\n\n5;foo=bar\nhello\n0\n\n"
	packet := strings.ReplaceAll(raw, "%s", srv.addr)

	for _, noFix := range []bool{true, false} {
		srv.reset()
		rsp, req, err := HTTP(packet, WithNoFixContentLength(noFix), WithTimeout(3))
		require.NoError(t, err)
		require.NotEmpty(t, rsp)

		wire := srv.only(t)
		if noFix {
			require.Equal(t, packet, wire, "不修复数据包时实际发出的字节必须与输入一致")
			require.Equal(t, packet, string(req), "不修复数据包时返回的请求报文必须与输入一致")
			continue
		}
		require.NotEqual(t, packet, wire, "默认模式应当修复报文")
		require.Equal(t, wire, string(req), "默认模式返回的请求报文应与实际发出的一致")
	}
}
