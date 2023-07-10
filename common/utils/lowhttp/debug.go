package lowhttp

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
)

func DebugEchoServer() (string, int) {
	return utils.DebugMockHTTPEx(func(req []byte) []byte {
		return ReplaceHTTPPacketBodyFast([]byte(`HTTP/1.1 200 OK
Content-Type: text/html
`), req)
	})
}

func DebugEchoServerContext(ctx context.Context) (string, int) {
	return utils.DebugMockHTTPExContext(ctx, func(req []byte) []byte {
		return ReplaceHTTPPacketBodyFast([]byte(`HTTP/1.1 200 OK
Content-Type: text/html
`), req)
	})
}
