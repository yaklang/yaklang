package yakgrpc

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
)

func TestH2Hijack(t *testing.T) {
	count := 0
	h2Host, h2Port := utils.DebugMockHTTP2(context.Background(), func(req []byte) []byte {
		count++
		return req
	})
	h2Addr := utils.HostPort(h2Host, h2Port)
	_, err := yak.NewScriptEngine(10).ExecuteEx(`
rsp,req = poc.HTTP(getParam("packet"), poc.http2(true), poc.https(true))~
`, map[string]any{
		"packet": `GET / HTTP/2.0
User-Agent: 111
Host: ` + h2Addr,
	})
	if err != nil {
		panic(err)
	}

	if count != 1 {
		t.Fatal("no recv h2 request")
	}
}
