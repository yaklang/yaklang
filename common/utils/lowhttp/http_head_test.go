package lowhttp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestHTTPServer_CheckChunkedDecoder(t *testing.T) {
	// 使用 DebugMockHTTPEx 创建一个 mock HTTP 服务器
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		reqStr := string(req)

		// 判断是否为 HEAD 请求
		isHead := strings.HasPrefix(reqStr, "HEAD")

		if isHead {
			// HEAD 请求：返回带有 Transfer-Encoding: chunked 的响应头
			// 有body但不能有content-length，这样才能到达errors.Wrap(err, "chunked decoder error")
			return []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Encoding: identity\r\nTransfer-Encoding: chunked\r\n\r\n")
		} else {
			if !strings.Contains(reqStr, "GET /xx HTTP/1.1") {
				t.Errorf("expected request to contain 'GET /xx HTTP/1.1', got: %s", reqStr)
			}
			if !strings.Contains(reqStr, "Host:") {
				t.Errorf("expected request to contain 'Host:', got: %s", reqStr)
			}

			// 返回响应
			return []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 5\r\n\r\nhello")
		}
	})

	// 测试 HEAD 请求（带 chunked encoding）
	t.Run("HEAD request with chunked encoding", func(t *testing.T) {
		rawRequest := fmt.Sprintf("HEAD /xx HTTP/1.1\r\nHost: %s:%d\r\n\r\n", host, port)

		// 使用 lowhttp.HTTP 发送 HEAD 请求
		rsp, err := HTTP(WithHost(host), WithPort(port), WithPacketBytes([]byte(rawRequest)))
		if err != nil {
			t.Fatalf("HTTP failed: %v", err)
		}

		// HEAD 请求的响应不应该包含 body
		// 即使服务器发送了 body（"aaaaa"），也应该被丢弃
		body := string(rsp.GetBody())
		raw := string(rsp.RawPacket)

		require.Equal(t, body, "")
		require.Equal(t, raw, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Encoding: identity\r\n\r\n")
	})
}
