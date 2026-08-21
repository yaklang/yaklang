//go:build !yakit_exclude

package yakgrpc

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestTruncateMITMBareRequestForStorage(t *testing.T) {
	previous := consts.GetGlobalMaxContentLength()
	const limit = 1024 * 1024
	consts.SetGlobalMaxContentLength(limit)
	t.Cleanup(func() {
		consts.SetGlobalMaxContentLength(previous)
	})

	t.Run("body_over_global_limit", func(t *testing.T) {
		body := strings.Repeat("A", limit+4096)
		packet := []byte("POST /upload HTTP/1.1\r\nHost: example.com\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body)

		stored, truncated, originalBodyLen, gotLimit := truncateMITMBareRequestForStorage(packet)
		require.True(t, truncated)
		require.Equal(t, len(body), originalBodyLen)
		require.Equal(t, limit, gotLimit)

		_, storedBody := lowhttp.SplitHTTPHeadersAndBodyFromPacketView(stored)
		require.Len(t, storedBody, limit)
		require.Contains(t, string(storedBody), "original request body truncated for storage")
		require.Contains(t, string(storedBody), "original=1M")
		require.Equal(t, strconv.Itoa(limit), lowhttp.GetHTTPPacketHeader(stored, "Content-Length"))
	})

	t.Run("body_at_global_limit", func(t *testing.T) {
		body := strings.Repeat("B", limit)
		packet := []byte("POST /upload HTTP/1.1\r\nHost: example.com\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body)

		stored, truncated, originalBodyLen, gotLimit := truncateMITMBareRequestForStorage(packet)
		require.False(t, truncated)
		require.Equal(t, len(body), originalBodyLen)
		require.Equal(t, limit, gotLimit)
		require.Equal(t, packet, stored)
	})
}
