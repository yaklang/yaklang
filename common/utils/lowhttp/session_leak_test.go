package lowhttp

import (
	"testing"
	"time"
)

func TestHTTPWithoutRedirectShouldNotCreateNilSessionCookiejar(t *testing.T) {
	CookiejarPool.Delete(nil)
	t.Cleanup(func() { CookiejarPool.Delete(nil) })

	// 这个请求会在解析 host/port 阶段直接失败，不会产生真实网络请求。
	_, _ = HTTPWithoutRedirect(
		WithPacketBytes([]byte("GET http:/// HTTP/1.1\r\nHost: \r\n\r\n")),
		WithTimeout(50*time.Millisecond),
	)

	if _, ok := CookiejarPool.Load(nil); ok {
		t.Fatalf("CookiejarPool should not store cookiejar for nil session")
	}
}

