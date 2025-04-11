package lowhttp

import (
	"context"
	"github.com/yaklang/yaklang/common/utils/testutils"
)

func DebugEchoServer() (string, int) {
	return testutils.DebugMockHTTPEx(func(req []byte) []byte {
		return ReplaceHTTPPacketBodyFast([]byte(`HTTP/1.1 200 OK
Content-Type: text/html
`), req)
	})
}

func DebugEchoServerContext(ctx context.Context) (string, int) {
	return testutils.DebugMockHTTPExContext(ctx, func(req []byte) []byte {
		return ReplaceHTTPPacketBodyFast([]byte(`HTTP/1.1 200 OK
Content-Type: text/html
`), req)
	})
}
