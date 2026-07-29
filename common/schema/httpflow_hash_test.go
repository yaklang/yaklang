package schema

import (
	"bytes"
	"math/rand"
	"strconv"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
)

func legacyHTTPFlowHash(flow *HTTPFlow) string {
	return utils.CalcSha1(
		flow.IsHTTPS,
		flow.Url,
		flow.Path,
		flow.Method,
		flow.BodyLength,
		flow.ContentType,
		flow.StatusCode,
		flow.SourceType,
		flow.Tags,
		flow.Request,
		flow.HiddenIndex,
		flow.RuntimeId,
		flow.FromPlugin,
	)
}

func TestHTTPFlowCalcHashPreservesLegacyFormat(t *testing.T) {
	random := rand.New(rand.NewSource(1))
	for index := 0; index < 256; index++ {
		randomString := func(maxLength int) string {
			alphabet := []byte("abc XYZ[]\\\n\r\t\x00\xff")
			value := make([]byte, random.Intn(maxLength+1))
			for offset := range value {
				value[offset] = alphabet[random.Intn(len(alphabet))]
			}
			return string(value)
		}
		flow := &HTTPFlow{
			IsHTTPS:     random.Intn(2) == 1,
			Url:         randomString(64),
			Path:        randomString(64),
			Method:      randomString(16),
			BodyLength:  random.Int63() - random.Int63(),
			ContentType: randomString(48),
			StatusCode:  random.Int63() - random.Int63(),
			SourceType:  randomString(32),
			Tags:        randomString(64),
			Request:     randomString(1024),
			HiddenIndex: randomString(64),
			RuntimeId:   randomString(64),
			FromPlugin:  randomString(64),
		}
		if got, want := flow.CalcHash(), legacyHTTPFlowHash(flow); got != want {
			t.Fatalf("case %d hash mismatch: got %s, want %s", index, got, want)
		}
	}
}

func BenchmarkHTTPFlowCalcHash64KRequest(b *testing.B) {
	request := append([]byte("POST /upload HTTP/1.1\r\nContent-Length: 65536\r\n\r\n"), bytes.Repeat([]byte("a"), 64*1024)...)
	flow := &HTTPFlow{
		IsHTTPS:     true,
		Url:         "https://example.test/upload",
		Path:        "/upload",
		Method:      "POST",
		BodyLength:  int64(len(request)),
		ContentType: "application/octet-stream",
		StatusCode:  200,
		SourceType:  HTTPFlow_SourceType_MITM,
		Tags:        "e2e|large-body",
		Request:     strconv.Quote(string(request)),
		HiddenIndex: "benchmark-hidden-index",
		RuntimeId:   "benchmark-runtime",
	}
	b.SetBytes(int64(len(flow.Request)))
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		_ = flow.CalcHash()
	}
}
