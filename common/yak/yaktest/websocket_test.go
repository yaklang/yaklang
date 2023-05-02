package yaktest

import (
	"fmt"
	"testing"
	"github.com/yaklang/yaklang/common/utils"
)

func TestMisc_WS(t *testing.T) {
	randomPort := utils.GetRandomAvailableTCPPort()
	utils.NewWebHookServer(randomPort, func(data interface{}) {

	})

	cases := []YakTestCase{
		{
			Name: "测试 poc.Websocket",
			Src: fmt.Sprintf(`
rsp, req, err = poc.Websocket(%v, poc.timeout(10), poc.websocketFromServer(func(i, cancel){
    dump(i)
	if str.MatchAllOfSubString(i, "uid=", "gid=", "groups=") { cancel() }
}), poc.websocketOnClient(func(wsClient) {
	wsClient.WriteText("[\"stdin\", \"id\\r\\n\"]")
}))
die(err)

println(rsp)
str.SplitAndTrim([]byte("  adfasdfad asdfasdfasdfasdf asdfasdf  "), " ")

`, "`"+`GET /terminals/websocket/1 HTTP/1.1
Host: 104.198.70.204
Accept-Encoding: gzip, deflate
Accept-Language: zh-CN,zh;q=0.9
Cache-Control: no-cache
Connection: Upgrade
Cookie: _xsrf=2|b4ec40bd|66f9830ab3b36fd2a0cfc32614cb1145|1662968307
Origin: http://104.198.70.204
Pragma: no-cache
Sec-WebSocket-Extensions: permessage-deflate; client_max_window_bits
Sec-WebSocket-Key: LIb4U+i+y+phoP4B2y6uoA==
Sec-WebSocket-Version: 13
Upgrade: websocket
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/104.0.0.0 Safari/537.36

`+"`"),
		},
	}

	Run("poc.Websocket", t, cases...)
}

func TestMisc_AutoType(t *testing.T) {
	cases := []YakTestCase{
		{
			Name: "测试 poc.Websocket",
			Src: fmt.Sprintf(`

a = str.SplitAndTrim([]byte("  adfasdfad asdfasdfasdfasdf asdfasdf  "), " ")
dump(a)

`),
		},
	}

	Run("poc.Websocket", t, cases...)
}

func TestWebsocket(t *testing.T) {
	cases := []YakTestCase{
		{Name: "测试 3syua", Src: `

`},
	}
	Run("poc.Websocket", t, cases...)
}
