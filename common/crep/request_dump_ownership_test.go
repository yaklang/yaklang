package crep

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

func newRequestDumpOwnershipBenchmarkRequest(b testing.TB, bodySize int) *http.Request {
	b.Helper()
	body := bytes.Repeat([]byte("r"), bodySize)
	packet := append([]byte(fmt.Sprintf("POST /upload HTTP/1.1\r\nHost: example.com\r\nContent-Length: %d\r\n\r\n", len(body))), body...)
	req, err := utils.ReadHTTPRequestFromBytes(packet)
	require.NoError(b, err)
	return req
}

func legacyDumpRequestToBareContext(req *http.Request) ([]byte, error) {
	raw, err := utils.DumpHTTPRequest(req, true)
	if err != nil {
		return nil, err
	}
	if len(raw) > 0 {
		httpctx.SetBareRequestBytes(req, raw)
	}
	return httpctx.GetBareRequestBytes(req), nil
}

func TestDumpRequestToBareContextTransfersDumpOwnership(t *testing.T) {
	req := newRequestDumpOwnershipBenchmarkRequest(t, 1024)
	stored, err := dumpRequestToBareContext(req)
	require.NoError(t, err)
	require.NotEmpty(t, stored)
	require.Same(t, &stored[0], &httpctx.GetBareRequestBytes(req)[0])
}

func BenchmarkDumpRequestToBareContext256K(b *testing.B) {
	for _, tc := range []struct {
		name string
		fn   func(*http.Request) ([]byte, error)
	}{
		{name: "legacy-cloned-context-snapshot", fn: legacyDumpRequestToBareContext},
		{name: "owned-context-transfer", fn: dumpRequestToBareContext},
	} {
		b.Run(tc.name, func(b *testing.B) {
			req := newRequestDumpOwnershipBenchmarkRequest(b, 256*1024)
			b.ReportAllocs()
			b.SetBytes(256 * 1024)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				packet, err := tc.fn(req)
				if err != nil {
					b.Fatal(err)
				}
				if len(packet) < 256*1024 {
					b.Fatal("request body was not dumped")
				}
			}
		})
	}
}
