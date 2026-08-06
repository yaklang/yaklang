//go:build !yakit_exclude

package yakgrpc

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

var benchmarkMITMRequestInstance *http.Request

func BenchmarkMITMRequestInstancePreparation256K(b *testing.B) {
	body := bytes.Repeat([]byte("0123456789abcdef"), 16*1024)
	packet := append(
		[]byte(fmt.Sprintf("POST /upload HTTP/1.1\r\nHost: example.test\r\nContent-Length: %d\r\n\r\n", len(body))),
		body...,
	)
	originRequest, err := lowhttp.ParseBytesToHttpRequest(packet)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("origin-request", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkMITMRequestInstance = originRequest
		}
	})

	b.Run("legacy-eager-fix-and-parse", func(b *testing.B) {
		b.SetBytes(int64(len(packet)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			fixed := lowhttp.FixHTTPRequest(packet)
			benchmarkMITMRequestInstance, _ = lowhttp.ParseBytesToHttpRequest(fixed)
		}
	})
}
