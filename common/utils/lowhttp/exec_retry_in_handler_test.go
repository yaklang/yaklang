package lowhttp

import (
	"bytes"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestHTTP_RetryWithHandler(t *testing.T) {
	flag := utils.RandStringBytes(100)
	flag2 := utils.RandStringBytes(50)
	count := 0
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		count++
		if count < 3 {
			writer.Write([]byte("not ready"))
			return
		}
		if rand.Intn(999) > 600 {
			writer.Write([]byte(flag))
		}
	})
	hostport := utils.HostPort(host, port)
	packet := `GET /` + flag2 + ` HTTP/1.1
Host: ` + hostport + `
User-Agent: yaklang-test/1.0

`

	checkReq := false
	rsp, err := HTTP(WithPacketBytes(
		[]byte(packet)),
		WithTimeout(time.Second),
		WithRetryWaitTime(20*time.Millisecond),
		WithRetryHandler(func(https bool, req []byte, rsp []byte) bool {
			if strings.Contains(string(req), flag2) {
				checkReq = true
			}
			fmt.Println(string(rsp))
			if bytes.Contains(rsp, []byte(flag)) {
				return false
			}
			return true
		}))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(rsp.RawPacket))
	require.True(t, checkReq)
	require.True(t, strings.Contains(string(rsp.RawPacket), string(flag)))
}

func TestHTTP_RetryWithHandler_StopImmediately(t *testing.T) {
	responseBody := "first response"
	count := 0
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		count++
		writer.Write([]byte(responseBody))
	})
	hostport := utils.HostPort(host, port)
	packet := `GET / HTTP/1.1
Host: ` + hostport + `
User-Agent: yaklang-test/1.0

`

	retryCount := 0
	rsp, err := HTTP(WithPacketBytes(
		[]byte(packet)),
		WithTimeout(time.Second),
		WithRetryWaitTime(20*time.Millisecond),
		WithRetryHandler(func(https bool, req []byte, rsp []byte) bool {
			retryCount++
			return false // stop immediately
		}))
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Equal(t, 1, retryCount)
	require.True(t, bytes.Contains(rsp.RawPacket, []byte(responseBody)))
}

func TestHTTPS_RetryWithHandler(t *testing.T) {
	flag := utils.RandStringBytes(100)
	count := 0

	host, port := utils.DebugMockHTTPSEx(func(req []byte) []byte {
		count++
		var body string
		if count < 3 {
			body = "not ready"
		} else {
			body = flag
		}
		return []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(body), body))
	})

	hostport := utils.HostPort(host, port)
	packet := `GET / HTTP/1.1
Host: ` + hostport + `
User-Agent: yaklang-test/1.0

`

	httpsParamCorrect := false
	rsp, err := HTTP(WithPacketBytes(
		[]byte(packet)),
		WithTimeout(time.Second),
		WithRetryWaitTime(20*time.Millisecond),
		WithHttps(true),
		WithVerifyCertificate(false),
		WithRetryHandler(func(https bool, req []byte, rsp []byte) bool {
			if https {
				httpsParamCorrect = true
			}
			if bytes.Contains(rsp, []byte(flag)) {
				return false
			}
			return true
		}))
	require.NoError(t, err)
	require.True(t, httpsParamCorrect, "https parameter in handler should be true")
	require.GreaterOrEqual(t, count, 3, "server should be called at least 3 times")
	require.True(t, strings.Contains(string(rsp.RawPacket), string(flag)), "final response should contain the flag")
}
