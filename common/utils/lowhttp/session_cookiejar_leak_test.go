// 本文件测试依赖进程级 cookiejar 池；勿调用 t.Parallel()（否则包内会与其它测试并发）。

package lowhttp

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

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

func resetCookiejarPoolForTest(t *testing.T) {
	t.Helper()
	cookiejarPool.Purge()
	t.Cleanup(cookiejarPool.Purge)
}

// 调用方可控的 session 名不能让进程级 cookiejar 池无限增长。
func TestCookiejarPool_persistentSessionsAreBounded(t *testing.T) {
	resetCookiejarPoolForTest(t)

	const extra = 64
	for i := 0; i < cookiejarPoolCapacity+extra; i++ {
		GetCookiejar(fmt.Sprintf("session-%d", i))
	}

	require.Equal(t, cookiejarPoolCapacity, CookiejarPoolCount())
	_, oldestStillPresent := cookiejarPool.Get("session-0")
	require.False(t, oldestStillPresent, "least-recently-used session should be evicted")
}

func TestCookiejarPool_concurrentSameSessionUsesOneJar(t *testing.T) {
	resetCookiejarPoolForTest(t)

	const workers = 64
	jars := make([]http.CookieJar, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range jars {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			jars[index] = GetCookiejar("shared")
		}(i)
	}
	close(start)
	wg.Wait()

	for i := 1; i < len(jars); i++ {
		require.True(t, jars[i] == jars[0], "same session created more than one cookie jar")
	}
	require.Equal(t, 1, CookiejarPoolCount())
}

// 未传 session：HTTP() 内 session 为空时自动分配并在结束后清理。
func TestCookiejarPool_ephemeralSessionWithoutExplicitSession(t *testing.T) {
	resetCookiejarPoolForTest(t)
	addr := cookiejarLeakMockAddr(t)

	const n = 30
	for i := 0; i < n; i++ {
		httpGetSetCookie(t, addr)
	}

	require.Zero(t, CookiejarPoolCount())
}

// DisableSession：不分配 session，cookie jar 池不增长。
func TestCookiejarPool_disableSessionDoesNotUseCookiejar(t *testing.T) {
	resetCookiejarPoolForTest(t)
	addr := cookiejarLeakMockAddr(t)

	const n = 30
	for i := 0; i < n; i++ {
		httpGetSetCookie(t, addr, WithDisableSession(true))
	}

	require.Zero(t, CookiejarPoolCount())
}
