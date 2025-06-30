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

func TestHTTP_ForceFailureHandler(t *testing.T) {
	flag := utils.RandStringBytes(100)
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(flag))
	})
	hostport := utils.HostPort(host, port)
	packet := `GET /test HTTP/1.1
Host: ` + hostport + `
User-Agent: yaklang-test/1.0

`

	t.Run("ForceFailureHandler returns false - request should succeed", func(t *testing.T) {
		rsp, err := HTTP(WithPacketBytes([]byte(packet)),
			WithTimeout(time.Second),
			WithForceFailureHandler(func(https bool, req []byte, rsp []byte) bool {
				return false
			}))
		require.NoError(t, err)
		require.True(t, strings.Contains(string(rsp.RawPacket), flag))
	})

	t.Run("ForceFailureHandler returns true - request should fail", func(t *testing.T) {
		rsp, err := HTTP(WithPacketBytes([]byte(packet)),
			WithTimeout(time.Second),
			WithForceFailureHandler(func(https bool, req []byte, rsp []byte) bool {
				return true
			}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "request failed intentionally by force failure handler")
		require.NotNil(t, rsp)
		require.True(t, strings.Contains(string(rsp.RawPacket), flag))
	})

	t.Run("ForceFailureHandler checks response content", func(t *testing.T) {
		rsp, err := HTTP(WithPacketBytes([]byte(packet)),
			WithTimeout(time.Second),
			WithForceFailureHandler(func(https bool, req []byte, rsp []byte) bool {
				// Fail only if response contains the flag
				return strings.Contains(string(rsp), flag)
			}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "request failed intentionally by force failure handler")
		require.NotNil(t, rsp)
		require.True(t, strings.Contains(string(rsp.RawPacket), flag))
	})
}

func TestHTTP_ForceFailureHandlerWithHTTP2(t *testing.T) {
	flag := utils.RandStringBytes(50)
	host, port := utils.DebugMockHTTP2(utils.TimeoutContextSeconds(5), func(req []byte) []byte {
		return []byte(flag)
	})
	hostport := utils.HostPort(host, port)
	packet := `GET /test HTTP/1.1
Host: ` + hostport + `
User-Agent: yaklang-test/1.0

`
	t.Run("ForceFailureHandler with HTTP2 - should fail", func(t *testing.T) {
		rsp, err := HTTP(WithPacketBytes([]byte(packet)),
			WithTimeout(5*time.Second),
			WithHttp2(true),
			WithHttps(true),
			WithVerifyCertificate(false),
			WithForceFailureHandler(func(https bool, req []byte, rsp []byte) bool {
				fmt.Printf("HTTPS: %v\n", https)
				fmt.Printf("Request: %s\n", string(req))
				fmt.Printf("Response: %s\n", string(rsp))
				return true // Always fail
			}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "request failed intentionally by force failure handler")
		require.NotNil(t, rsp)
	})
}
