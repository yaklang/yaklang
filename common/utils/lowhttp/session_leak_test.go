// 本文件测试依赖进程级 cookiejar 池；勿调用 t.Parallel()（否则包内会与其它测试并发）。

package lowhttp

import (
	"testing"
	"time"
)

func TestHTTPWithoutRedirectShouldNotCreateNilSessionCookiejar(t *testing.T) {
	baseline := CookiejarPoolCount()

	// 这个请求会在解析 host/port 阶段直接失败，不会产生真实网络请求。
	_, _ = HTTPWithoutRedirect(
		WithPacketBytes([]byte("GET http:/// HTTP/1.1\r\nHost: \r\n\r\n")),
		WithTimeout(50*time.Millisecond),
	)

	if CookiejarPoolCount()-baseline > 0 {
		t.Fatalf("cookiejar pool should not grow for failed request without session, baseline=%d now=%d", baseline, CookiejarPoolCount())
	}
}
