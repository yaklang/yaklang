package utils_test

import (
	"context"
	"crypto/tls"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	_ "github.com/yaklang/yaklang/common/utils/tlsutils"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestDebugMockHTTPClosesCompletedResponse(t *testing.T) {
	for _, https := range []bool{false, true} {
		name := "HTTP"
		if https {
			name = "HTTPS"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			body := strings.Repeat("complete local response", 64)
			host, port := utils.DebugMockHTTPServerWithContext(ctx, https, false, false, false, false, func([]byte) []byte {
				// A close-delimited body requires EOF, so a hidden post-write
				// sleep would cause the read deadline to expire.
				return []byte("HTTP/1.1 200 OK\r\nConnection: close\r\n\r\n" + body)
			})
			var conn net.Conn
			var err error
			if https {
				conn, err = tls.Dial("tcp", utils.HostPort(host, port), &tls.Config{InsecureSkipVerify: true})
			} else {
				conn, err = net.Dial("tcp", utils.HostPort(host, port))
			}
			require.NoError(t, err)
			defer conn.Close()
			require.NoError(t, conn.SetDeadline(time.Now().Add(250*time.Millisecond)))
			_, err = io.WriteString(conn, "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n")
			require.NoError(t, err)
			response, err := io.ReadAll(conn)
			require.NoError(t, err)
			require.Equal(t, "HTTP/1.1 200 OK\r\nConnection: close\r\n\r\n"+body, string(response))
		})
	}
}
