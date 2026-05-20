// 本文件测试依赖进程级 cookiejar 池；勿调用 t.Parallel()（否则包内会与其它测试并发）。

package lowhttp

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func cookiejarLeakMockAddr(t *testing.T) string {
	t.Helper()
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: strings.Repeat("x", 512)})
		_, _ = w.Write([]byte("ok"))
	})
	return utils.HostPort(host, port)
}

func httpGetSetCookie(t *testing.T, addr string, opts ...LowhttpOpt) {
	t.Helper()
	raw := fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\n\r\n", addr)
	_, err := HTTP(append([]LowhttpOpt{
		WithPacketBytes([]byte(raw)),
		WithTimeout(3 * time.Second),
		WithSaveHTTPFlow(false),
	}, opts...)...)
	require.NoError(t, err)
}

// 显式 WithSession 且未 RemoveCookiejar：jar 留在池中（持久 session）。
func TestCookiejarPool_persistentSessionWithoutCleanup(t *testing.T) {
	baseline := CookiejarPoolCount()
	addr := cookiejarLeakMockAddr(t)

	const n = 30
	for i := 0; i < n; i++ {
		httpGetSetCookie(t, addr, WithSession(uuid.NewString()))
	}

	require.GreaterOrEqual(t, CookiejarPoolCount()-baseline, n-3)
}

// 未传 session：HTTP() 内 session 为空时自动分配并在结束后清理。
func TestCookiejarPool_ephemeralSessionWithoutExplicitSession(t *testing.T) {
	baseline := CookiejarPoolCount()
	addr := cookiejarLeakMockAddr(t)

	const n = 30
	for i := 0; i < n; i++ {
		httpGetSetCookie(t, addr)
	}

	require.LessOrEqual(t, CookiejarPoolCount()-baseline, 2)
}

// DisableSession：不分配 session，cookie jar 池不增长。
func TestCookiejarPool_disableSessionDoesNotUseCookiejar(t *testing.T) {
	baseline := CookiejarPoolCount()
	addr := cookiejarLeakMockAddr(t)

	const n = 30
	for i := 0; i < n; i++ {
		httpGetSetCookie(t, addr, WithDisableSession(true))
	}

	require.LessOrEqual(t, CookiejarPoolCount()-baseline, 0)
}
