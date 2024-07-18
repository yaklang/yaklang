package poc

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestPocRequestWithSession(t *testing.T) {
	token, token2, token3 := utils.RandStringBytes(10), utils.RandStringBytes(10), utils.RandStringBytes(10)
	cookieStr := fmt.Sprintf("%s=%s", token, token2)

	host, port := utils.DebugMockHTTP([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nConnection: close\r\nSet-Cookie: %s\r\n\r\n", cookieStr)))

	// get cookie from server
	_, _, err := HTTP(fmt.Sprintf(`GET / HTTP/1.1
Host: %s
`, utils.HostPort(host, port)), WithSession(token3))
	require.NoError(t, err)

	// test HTTP / DO
	// if request has cookie
	_, req, err := HTTP(fmt.Sprintf(`GET / HTTP/1.1
Host: %s
`, utils.HostPort(host, port)), WithSession(token3))
	require.NoError(t, err)
	require.Contains(t, string(req), cookieStr)

	_, req2, err := Do(http.MethodGet, fmt.Sprintf("http://%s", utils.HostPort(host, port)), WithSession(token3))
	require.NoError(t, err)
	cookie, err := req2.Cookie(token)
	require.NoError(t, err)
	require.Equal(t, token2, cookie.Value)
}
