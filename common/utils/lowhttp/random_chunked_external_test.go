package lowhttp_test

import (
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"testing"
	"time"
)

func TestRandomChunkedHTTPExternal(t *testing.T) {
	t.Run("test simple", func(t *testing.T) {
		minLength, maxLength := 10, 25
		minDelay, maxDelay := 100*time.Millisecond, 300*time.Millisecond

		opts := []lowhttp.LowhttpOpt{
			lowhttp.WithRequest(`POST /echo HTTP/1.1
Host: 127.0.0.1:8090
Accept-Language: en-US;q=0.9,en;q=0.8
User-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36

abcdefghijklmnopqrstlv`),
			lowhttp.WithForceChunked(true),
			lowhttp.WithChunkDelayTime(minDelay, maxDelay),
			lowhttp.WithChunkedLength(minLength, maxLength),
			lowhttp.WithProxy("http://127.0.0.1:8083"),
		}
		rsp, err := lowhttp.HTTP(opts...)
		if err != nil {
			return
		}
		t.Log(string(rsp.GetBody()))
	})
}
