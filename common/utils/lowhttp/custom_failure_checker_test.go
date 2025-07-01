package lowhttp

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestHTTP_CustomFailureChecker(t *testing.T) {
	flag := utils.RandStringBytes(100)
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(flag))
	})
	hostport := utils.HostPort(host, port)
	packet := `GET /test HTTP/1.1
Host: ` + hostport + `
User-Agent: yaklang-test/1.0

`

	t.Run("CustomFailureChecker no fail call - request should succeed", func(t *testing.T) {
		rsp, err := HTTP(WithPacketBytes([]byte(packet)),
			WithTimeout(time.Second),
			WithCustomFailureChecker(func(https bool, req []byte, rsp []byte, fail func(string)) {
				// Do not call fail function
			}))
		require.NoError(t, err)
		require.True(t, strings.Contains(string(rsp.RawPacket), flag))
	})

	t.Run("CustomFailureChecker with fail call - request should fail", func(t *testing.T) {
		rsp, err := HTTP(WithPacketBytes([]byte(packet)),
			WithTimeout(time.Second),
			WithCustomFailureChecker(func(https bool, req []byte, rsp []byte, fail func(string)) {
				fail("intentional failure for testing")
			}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "request failed intentionally by custom failure checker")
		require.Contains(t, err.Error(), "intentional failure for testing")
		require.NotNil(t, rsp)
		require.True(t, strings.Contains(string(rsp.RawPacket), flag))
	})

	t.Run("CustomFailureChecker checks response content", func(t *testing.T) {
		rsp, err := HTTP(WithPacketBytes([]byte(packet)),
			WithTimeout(time.Second),
			WithCustomFailureChecker(func(https bool, req []byte, rsp []byte, fail func(string)) {
				// Fail only if response contains the flag
				if strings.Contains(string(rsp), flag) {
					fail("Response contains flag: " + flag)
				}
			}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "request failed intentionally by custom failure checker")
		require.Contains(t, err.Error(), "Response contains flag:")
		require.NotNil(t, rsp)
		require.True(t, strings.Contains(string(rsp.RawPacket), flag))
	})
}

func TestHTTP_CustomFailureCheckerWithHTTP2(t *testing.T) {
	flag := utils.RandStringBytes(50)
	host, port := utils.DebugMockHTTP2(utils.TimeoutContextSeconds(5), func(req []byte) []byte {
		return []byte(flag)
	})
	hostport := utils.HostPort(host, port)
	packet := `GET /test HTTP/1.1
Host: ` + hostport + `
User-Agent: yaklang-test/1.0

`
	t.Run("CustomFailureChecker with HTTP2 - should fail", func(t *testing.T) {
		rsp, err := HTTP(WithPacketBytes([]byte(packet)),
			WithTimeout(5*time.Second),
			WithHttp2(true),
			WithHttps(true),
			WithVerifyCertificate(false),
			WithCustomFailureChecker(func(https bool, req []byte, rsp []byte, fail func(string)) {
				fmt.Printf("HTTPS: %v\n", https)
				fmt.Printf("Request: %s\n", string(req))
				fmt.Printf("Response: %s\n", string(rsp))
				fail("Always fail for HTTP2 testing")
			}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "request failed intentionally by custom failure checker")
		require.Contains(t, err.Error(), "Always fail for HTTP2 testing")
		require.NotNil(t, rsp)
	})
}
