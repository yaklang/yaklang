package yakgrpc

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/netx"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/facades"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_MITM_WITH_REPLACE_RULE_GZIP_NCHUNKED(t *testing.T) {
	var mockHost, mockPort = utils.DebugMockHTTPEx(func(req []byte) []byte {
		rsp, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 OK
Content-Type: text/html
Content-Length: 3

111`))
		_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(req)
		req = lowhttp.FixHTTPRequest(req)
		var reqIsGzip = lowhttp.GetHTTPPacketHeader(req, "Content-Encoding") == "gzip"
		var reqIsChunked = lowhttp.GetHTTPPacketHeader(req, "Transfer-Encoding") == "chunked"

		if reqIsChunked {
			body, _ = codec.HTTPChunkedDecode(body)
		}
		if reqIsGzip {
			body, _ = utils.GzipDeCompress(body)
		}

		if reqIsGzip {
			body, _ = utils.GzipCompress(body)
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Content-Encoding", "gzip")
		}
		rsp = lowhttp.ReplaceHTTPPacketBodyEx(rsp, body, reqIsChunked, false)
		return rsp
	})

	client, err := NewLocalClient() // 新建一个 yakit client
	if err != nil {
		t.Fatal(err)
	}

	rPort := utils.GetRandomAvailableTCPPort()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动MITM服务器
	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host:             "127.0.0.1",
		Port:             uint32(rPort),
		Recover:          true,
		Forward:          true,
		SetAutoForward:   true,
		AutoForwardValue: true,
		EnableHttp2:      true,
	})
	var wg sync.WaitGroup
	wg.Add(1)
	var started = false
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if strings.Contains(spew.Sdump(rsp), `starting mitm server`) && !started {
			started = true

			token := ksuid.New().String()
			body, _ := utils.GzipCompress(token)
			var packet = "GET / HTTP/1.1\r\nHost: " + utils.HostPort(mockHost, mockPort) + "\r\n\r\n" + string(body)
			var packetBytes = lowhttp.ReplaceHTTPPacketHeader([]byte(packet), "Content-Encoding", "gzip")
			packetBytes = lowhttp.ReplaceHTTPPacketHeader(packetBytes, "Transfer-Encoding", "chunked")
			_, err := yak.Execute(`
proxy = "http://"+str.HostPort(mitmHost, mitmPort)
rsp, req = poc.HTTP(packet, poc.proxy(proxy), poc.host(mockHost), poc.port(mockPort))~
assert rsp.Contains(token), "gzip + chunk failed"
`, map[string]any{
				"mitmHost": "127.0.0.1", "mitmPort": rPort,
				"mockHost": "127.0.0.1", "mockPort": mockPort,
				"packet": packetBytes, "token": token,
			})
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
}

func TestGRPCMUSTPASS_MITM(t *testing.T) {
	client, err := NewLocalClient() // 新建一个 yakit client
	if err != nil {
		t.Fatal(err)
	}

	var (
		started           bool // MITM正常启动（此时MITM开启HTTP2支持）
		passthroughTested bool // Mock的普通HTTP服务器正常工作
		echoTested        bool // 将MITM作为代理向mock的http服务器发包 这个过程成功说明 MITM开启H2支持的情况下 能够正确处理H1请求
		gzipAutoDecode    bool // 将MITM作为代理向mock的http服务器发包 同时客户端发包被gzip编码 mitm正常处理 mock服务器正常处理 说明整个流程正确处理了gzip编码的情况
		chunkDecode       bool // 将MITM作为代理向mock的http服务器发包 同时客户端发包被gzip编码 且使用chunk编码 mitm正常处理 mock服务器正常处理 说明整个流程正确处理了gzip+chunk编码的情况
		h2Test            bool // 将MITM作为代理向mock的http2服务器发包 这个过程成功说明 MITM开启H2支持的情况下 能够正确处理H2请求和响应
	)

	var mockHost, mockPort = utils.DebugMockHTTPEx(func(req []byte) []byte {
		passthroughTested = true // 测试标识位 收到了http请求
		rsp, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 OK
Content-Type: text/html
Content-Length: 3

111`))
		_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(req)
		req = lowhttp.FixHTTPRequest(req)
		var reqIsGzip = lowhttp.GetHTTPPacketHeader(req, "Content-Encoding") == "gzip"
		var reqIsChunked = lowhttp.GetHTTPPacketHeader(req, "Transfer-Encoding") == "chunked"

		if reqIsChunked {
			body, err = codec.HTTPChunkedDecode(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		if reqIsGzip {
			body, err = utils.GzipDeCompress(body)
			if err != nil {
				t.Fatal(err)
			}
		}

		if reqIsGzip {
			body, err = utils.GzipCompress(body)
			if err != nil {
				t.Fatal(err)
			}
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Content-Encoding", "gzip")
		}
		rsp = lowhttp.ReplaceHTTPPacketBodyEx(rsp, body, reqIsChunked, false)
		return rsp
	})

	log.Infof("start to mock server: %v", utils.HostPort(mockHost, mockPort))
	var rPort = utils.GetRandomAvailableTCPPort()
	var proxy = "http://127.0.0.1:" + fmt.Sprint(rPort)
	_ = proxy

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	/* H2 */
	h2Host, h2Port := utils.DebugMockHTTP2(ctx, func(req []byte) []byte {
		return req
	})
	h2Addr := utils.HostPort(h2Host, h2Port)
	// 测试我们的h2 mock服务器是否正常工作
	_, err = yak.NewScriptEngine(10).ExecuteEx(`
rsp,req = poc.HTTP(getParam("packet"), poc.http2(true), poc.https(true))~
`, map[string]any{
		"packet": `GET / HTTP/2.0
User-Agent: 111
Host: ` + h2Addr,
	})
	if err != nil {
		t.Fatal(err)
	}

	rule := `W3siUnVsZSI6Iig/aSkoanNvbnBfW2EtejAtOV0rKXwoKF8/Y2FsbGJhY2t8X2NifF9jYWxsfF8/anNvbnBfPyk9KSIsIkNvbG9yIjoieWVsbG93IiwiRW5hYmxlRm9yUmVxdWVzdCI6dHJ1ZSwiRW5hYmxlRm9ySGVhZGVyIjp0cnVlLCJJbmRleCI6MSwiRXh0cmFUYWciOlsi55aR5Ly8SlNPTlAiXSwiTm9SZXBsYWNlIjp0cnVlfSx7IlJ1bGUiOiIoP2kpKChwYXNzd29yZCl8KHBhc3MpfChzZWNyZXQpfChtaW1hKSlbJ1wiXT9cXHMqW1xcOlxcPV0iLCJDb2xvciI6InJlZCIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvckhlYWRlciI6dHJ1ZSwiRW5hYmxlRm9yQm9keSI6dHJ1ZSwiSW5kZXgiOjIsIkV4dHJhVGFnIjpbIueZu+mZhi/lr4bnoIHkvKDovpMiXSwiTm9SZXBsYWNlIjp0cnVlfSx7IlJ1bGUiOiIoP2kpKChhY2Nlc3N8YWRtaW58YXBpfGRlYnVnfGF1dGh8YXV0aG9yaXphdGlvbnxncGd8b3BzfHJheXxkZXBsb3l8czN8Y2VydGlmaWNhdGV8YXdzfGFwcHxhcHBsaWNhdGlvbnxkb2NrZXJ8ZXN8ZWxhc3RpY3xlbGFzdGljc2VhcmNofHNlY3JldClbLV9dezAsNX0oa2V5fHRva2VufHNlY3JldHxzZWNyZXRrZXl8cGFzc3xwYXNzd29yZHxzaWR8ZGVidWcpKXwoc2VjcmV0fHBhc3N3b3JkKShbXCInXT9cXHMqOlxccyp8XFxzKj1cXHMqKSIsIkNvbG9yIjoicmVkIiwiRW5hYmxlRm9yUmVxdWVzdCI6dHJ1ZSwiRW5hYmxlRm9yUmVzcG9uc2UiOnRydWUsIkVuYWJsZUZvckhlYWRlciI6dHJ1ZSwiRW5hYmxlRm9yQm9keSI6dHJ1ZSwiSW5kZXgiOjMsIkV4dHJhVGFnIjpbIuaVj+aEn+S/oeaBryJdLCJOb1JlcGxhY2UiOnRydWV9LHsiUnVsZSI6IihCRUdJTiBQVUJMSUMgS0VZKS4qPyhFTkQgUFVCTElDIEtFWSkiLCJDb2xvciI6InB1cnBsZSIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvclJlc3BvbnNlIjp0cnVlLCJFbmFibGVGb3JIZWFkZXIiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4Ijo0LCJFeHRyYVRhZyI6WyLlhazpkqXkvKDovpMiXSwiTm9SZXBsYWNlIjp0cnVlfSx7IlJ1bGUiOiIoP2lzKSg8Zm9ybS4qdHlwZT0uKj90ZXh0Lio/dHlwZT0uKj9wYXNzd29yZC4qPzwvZm9ybS4qPz4pIiwiQ29sb3IiOiJncmVlbiIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvclJlc3BvbnNlIjp0cnVlLCJFbmFibGVGb3JIZWFkZXIiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4Ijo1LCJFeHRyYVRhZyI6WyLnmbvpmYbngrkiXSwiVmVyYm9zZU5hbWUiOiLnmbvpmYbngrkiLCJOb1JlcGxhY2UiOnRydWV9LHsiUnVsZSI6Iig/aXMpKDxmb3JtLip0eXBlPS4qP3RleHQuKj90eXBlPS4qP3Bhc3N3b3JkLio/b25jbGljaz0uKj88L2Zvcm0uKj8+KSIsIkNvbG9yIjoiZ3JlZW4iLCJFbmFibGVGb3JSZXF1ZXN0Ijp0cnVlLCJFbmFibGVGb3JSZXNwb25zZSI6dHJ1ZSwiRW5hYmxlRm9ySGVhZGVyIjp0cnVlLCJFbmFibGVGb3JCb2R5Ijp0cnVlLCJJbmRleCI6NiwiRXh0cmFUYWciOlsi55m76ZmG77yI6aqM6K+B56CB77yJIl0sIlZlcmJvc2VOYW1lIjoi55m76ZmG77yI6aqM6K+B56CB77yJIiwiTm9SZXBsYWNlIjp0cnVlfSx7IlJ1bGUiOiIoP2lzKTxmb3JtLiplbmN0eXBlPS4qP211bHRpcGFydC9mb3JtLWRhdGEuKj90eXBlPS4qP2ZpbGUuKj88L2Zvcm0+IiwiQ29sb3IiOiJncmVlbiIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvclJlc3BvbnNlIjp0cnVlLCJFbmFibGVGb3JIZWFkZXIiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4Ijo3LCJFeHRyYVRhZyI6WyLmlofku7bkuIrkvKDngrkiXSwiVmVyYm9zZU5hbWUiOiLmlofku7bkuIrkvKDngrkiLCJOb1JlcGxhY2UiOnRydWV9LHsiUnVsZSI6IihmaWxlPXxwYXRoPXx1cmw9fGxhbmc9fHNyYz18bWVudT18bWV0YS1pbmY9fHdlYi1pbmY9fGZpbGVuYW1lPXx0b3BpYz18cGFnZT3vvZxfRmlsZVBhdGg9fHRhcmdldD0pIiwiQ29sb3IiOiJncmVlbiIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvclJlc3BvbnNlIjp0cnVlLCJFbmFibGVGb3JIZWFkZXIiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4Ijo4LCJFeHRyYVRhZyI6WyLmlofku7bljIXlkKvlj4LmlbAiXSwiVmVyYm9zZU5hbWUiOiLmlofku7bljIXlkKvlj4LmlbAiLCJOb1JlcGxhY2UiOnRydWV9LHsiUnVsZSI6IigoY21kPSl8KGV4ZWM9KXwoY29tbWFuZD0pfChleGVjdXRlPSl8KHBpbmc9KXwocXVlcnk9KXwoanVtcD0pfChjb2RlPSl8KHJlZz0pfChkbz0pfChmdW5jPSl8KGFyZz0pfChvcHRpb249KXwobG9hZD0pfChwcm9jZXNzPSl8KHN0ZXA9KXwocmVhZD0pfChmdW5jdGlvbj0pfChmZWF0dXJlPSl8KGV4ZT0pfChtb2R1bGU9KXwocGF5bG9hZD0pfChydW49KXwoZGFlbW9uPSl8KHVwbG9hZD0pfChkaXI9KXwoZG93bmxvYWQ9KXwobG9nPSl8KGlwPSl8KGNsaT0pKXwoaXBhZGRyZXNzPSl8KHR4dD0pfChjYXNlPSl8KGNvdW50PSkiLCJDb2xvciI6ImdyZWVuIiwiRW5hYmxlRm9yUmVxdWVzdCI6dHJ1ZSwiRW5hYmxlRm9yUmVzcG9uc2UiOnRydWUsIkVuYWJsZUZvckhlYWRlciI6dHJ1ZSwiRW5hYmxlRm9yQm9keSI6dHJ1ZSwiSW5kZXgiOjksIkV4dHJhVGFnIjpbIuWRveS7pOazqOWFpeWPguaVsCJdLCJWZXJib3NlTmFtZSI6IuWRveS7pOazqOWFpeWPguaVsCIsIk5vUmVwbGFjZSI6dHJ1ZX0seyJSdWxlIjoiXFxiKChbXjw+KClbXFxdXFxcXC4sOzpcXHNAXCJdKyhcXC5bXjw+KClbXFxdXFxcXC4sOzpcXHNAXCJdKykqKXwoXCIuK1wiKSlAKChcXFtbMC05XXsxLDN9XFwuWzAtOV17MSwzfVxcLlswLTldezEsM31cXC5bMC05XXsxLDN9XFxdKXwoKFthLXpBLVpcXC0wLTldK1xcLikrKGNufGNvbXxlZHV8Z292fGludHxtaWx8bmV0fG9yZ3xiaXp8aW5mb3xwcm98bmFtZXxtdXNldW18Y29vcHxhZXJvfHh4eHxpZHYpKSlcXGIiLCJDb2xvciI6ImdyZWVuIiwiRW5hYmxlRm9yUmVzcG9uc2UiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4IjoxMCwiRXh0cmFUYWciOlsiZW1haWzms4TmvI8iXSwiVmVyYm9zZU5hbWUiOiJlbWFpbOazhOa8jyIsIk5vUmVwbGFjZSI6dHJ1ZX0seyJSdWxlIjoiXFxiKD86KD86XFwrfDAwKTg2KT8xKD86KD86M1tcXGRdKXwoPzo0WzUtNzldKXwoPzo1WzAtMzUtOV0pfCg/OjZbNS03XSl8KD86N1swLThdKXwoPzo4W1xcZF0pfCg/OjlbMTg5XSkpXFxkezh9XFxiIiwiQ29sb3IiOiJncmVlbiIsIkVuYWJsZUZvclJlc3BvbnNlIjp0cnVlLCJFbmFibGVGb3JCb2R5Ijp0cnVlLCJJbmRleCI6MTEsIkV4dHJhVGFnIjpbIuaJi+acuuWPt+azhOa8jyJdLCJWZXJib3NlTmFtZSI6IuaJi+acuuWPt+azhOa8jyIsIk5vUmVwbGFjZSI6dHJ1ZX0seyJSdWxlIjoiKChcXFtjbGllbnRcXF0pfFxcWyhteXNxbFxcXSl8KFxcW215c3FsZFxcXSkpIiwiQ29sb3IiOiJncmVlbiIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvclJlc3BvbnNlIjp0cnVlLCJFbmFibGVGb3JIZWFkZXIiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4IjoxMiwiRXh0cmFUYWciOlsiTXlTUUzphY3nva4iXSwiVmVyYm9zZU5hbWUiOiJNeVNRTOmFjee9riIsIk5vUmVwbGFjZSI6dHJ1ZX0seyJSdWxlIjoiXFxiWzEtOV1cXGR7NX0oPzoxOHwxOXwyMClcXGR7Mn0oPzowWzEtOV18MTB8MTF8MTIpKD86MFsxLTldfFsxLTJdXFxkfDMwfDMxKVxcZHszfVtcXGRYeF1cXGIiLCJDb2xvciI6ImdyZWVuIiwiRW5hYmxlRm9yUmVzcG9uc2UiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4IjoxMywiRXh0cmFUYWciOlsi6Lqr5Lu96K+BIl0sIlZlcmJvc2VOYW1lIjoi6Lqr5Lu96K+BIiwiTm9SZXBsYWNlIjp0cnVlfSx7IlJ1bGUiOiJbLV0rQkVHSU4gW15cXHNdKyBQUklWQVRFIEtFWVstXSIsIkNvbG9yIjoicmVkIiwiRW5hYmxlRm9yUmVxdWVzdCI6dHJ1ZSwiRW5hYmxlRm9yUmVzcG9uc2UiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4IjoxNCwiRXh0cmFUYWciOlsiUlNB56eB6ZKlIl0sIlZlcmJvc2VOYW1lIjoiUlNB56eB6ZKlIiwiTm9SZXBsYWNlIjp0cnVlfSx7IlJ1bGUiOiIoW0F8YV1jY2Vzc1tLfGtdZXlbU3xzXWVjcmV0KXwoW0F8YV1jY2Vzc1tLfGtdZXlbSXxpXVtkfERdKXwoW0FhXShjY2Vzc3xDQ0VTUylfP1tLa10oZXl8RVkpKXwoW0FhXShjY2Vzc3xDQ0VTUylfP1tzU10oZWNyZXR8RUNSRVQpKXwoKFtBYV0oY2Nlc3N8Q0NFU1MpXz8oaWR8SUR8SWQpKSl8KFtTc10oZWNyZXR8RUNSRVQpXz9bS2tdKGV5fEVZKSkiLCJDb2xvciI6InllbGxvdyIsIkVuYWJsZUZvclJlc3BvbnNlIjp0cnVlLCJFbmFibGVGb3JCb2R5Ijp0cnVlLCJJbmRleCI6MTUsIkV4dHJhVGFnIjpbIk9TUyBLZXkiXSwiVmVyYm9zZU5hbWUiOiJPU1MgS2V5IiwiTm9SZXBsYWNlIjp0cnVlfSx7IlJ1bGUiOiJbXFx3LS5dK1xcLm9zc1xcLmFsaXl1bmNzXFwuY29tIiwiQ29sb3IiOiJyZWQiLCJFbmFibGVGb3JSZXNwb25zZSI6dHJ1ZSwiRW5hYmxlRm9ySGVhZGVyIjp0cnVlLCJFbmFibGVGb3JCb2R5Ijp0cnVlLCJJbmRleCI6MTYsIkV4dHJhVGFnIjpbIkFsaXl1bk9TUyJdLCJWZXJib3NlTmFtZSI6IkFsaXl1bk9TUyIsIk5vUmVwbGFjZSI6dHJ1ZX0seyJSdWxlIjoiXFxiKCgxMjdcXC4wXFwuMFxcLjEpfChsb2NhbGhvc3QpfCgxMFxcLlxcZHsxLDN9XFwuXFxkezEsM31cXC5cXGR7MSwzfSl8KDE3MlxcLigoMVs2LTldKXwoMlxcZCl8KDNbMDFdKSlcXC5cXGR7MSwzfVxcLlxcZHsxLDN9KXwoMTkyXFwuMTY4XFwuXFxkezEsM31cXC5cXGR7MSwzfSkpXFxiIiwiQ29sb3IiOiJyZWQiLCJFbmFibGVGb3JSZXF1ZXN0Ijp0cnVlLCJFbmFibGVGb3JSZXNwb25zZSI6dHJ1ZSwiRW5hYmxlRm9ySGVhZGVyIjp0cnVlLCJFbmFibGVGb3JCb2R5Ijp0cnVlLCJJbmRleCI6MTcsIkV4dHJhVGFnIjpbIklQ5Zyw5Z2AIl0sIlZlcmJvc2VOYW1lIjoiSVDlnLDlnYAiLCJOb1JlcGxhY2UiOnRydWV9LHsiUnVsZSI6Iig9ZGVsZXRlTWV8cmVtZW1iZXJNZT0pIiwiQ29sb3IiOiJncmVlbiIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvclJlc3BvbnNlIjp0cnVlLCJFbmFibGVGb3JIZWFkZXIiOnRydWUsIkluZGV4IjoxOCwiRXh0cmFUYWciOlsiU2hpcm8iXSwiVmVyYm9zZU5hbWUiOiJTaGlybyIsIk5vUmVwbGFjZSI6dHJ1ZX0seyJSdWxlIjoiKD9pcyleey4qfSQiLCJDb2xvciI6ImdyZWVuIiwiRW5hYmxlRm9yUmVxdWVzdCI6dHJ1ZSwiRW5hYmxlRm9yUmVzcG9uc2UiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4IjoxOSwiRXh0cmFUYWciOlsiSlNPTuS8oOi+kyJdLCJWZXJib3NlTmFtZSI6IkpTT07kvKDovpMiLCJOb1JlcGxhY2UiOnRydWV9LHsiUnVsZSI6Iig/aXMpXjxcXD94bWwuKjxzb2FwOkJvZHk+IiwiQ29sb3IiOiJncmVlbiIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4IjoyMCwiRXh0cmFUYWciOlsiU09BUOivt+axgiJdLCJWZXJib3NlTmFtZSI6IlNPQVDor7fmsYIiLCJOb1JlcGxhY2UiOnRydWV9LHsiUnVsZSI6Iig/aXMpXjxcXD94bWwuKj4kIiwiQ29sb3IiOiJncmVlbiIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4IjoyMSwiRXh0cmFUYWciOlsiWE1M6K+35rGCIl0sIlZlcmJvc2VOYW1lIjoiWE1M6K+35rGCIiwiTm9SZXBsYWNlIjp0cnVlfSx7IlJ1bGUiOiIoP2kpKEF1dGhvcml6YXRpb246IC4qKXwod3d3LUF1dGhlbnRpY2F0ZTogKChCYXNpYyl8KEJlYXJlcil8KERpZ2VzdCl8KEhPQkEpfChNdXR1YWwpfChOZWdvdGlhdGUpfChPQXV0aCl8KFNDUkFNLVNIQS0xKXwoU0NSQU0tU0hBLTI1Nil8KHZhcGlkKSkpIiwiQ29sb3IiOiJncmVlbiIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvclJlc3BvbnNlIjp0cnVlLCJFbmFibGVGb3JIZWFkZXIiOnRydWUsIkluZGV4IjoyMiwiRXh0cmFUYWciOlsiSFRUUOiupOivgeWktCJdLCJWZXJib3NlTmFtZSI6IkhUVFDorqTor4HlpLQiLCJOb1JlcGxhY2UiOnRydWV9LHsiUnVsZSI6IihHRVQuKlxcdys9XFx3Kyl8KD9pcykoUE9TVC4qXFxuXFxuLipcXHcrPVxcdyspIiwiQ29sb3IiOiJncmVlbiIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvclJlc3BvbnNlIjp0cnVlLCJFbmFibGVGb3JIZWFkZXIiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4IjoyMywiRXh0cmFUYWciOlsiU1FM5rOo5YWl5rWL6K+V54K5Il0sIlZlcmJvc2VOYW1lIjoiU1FM5rOo5YWl5rWL6K+V54K5IiwiTm9SZXBsYWNlIjp0cnVlfSx7IlJ1bGUiOiIoR0VULipcXHcrPVxcdyspfCg/aXMpKFBPU1QuKlxcblxcbi4qXFx3Kz1cXHcrKSIsIkNvbG9yIjoiZ3JlZW4iLCJFbmFibGVGb3JSZXF1ZXN0Ijp0cnVlLCJFbmFibGVGb3JSZXNwb25zZSI6dHJ1ZSwiRW5hYmxlRm9ySGVhZGVyIjp0cnVlLCJFbmFibGVGb3JCb2R5Ijp0cnVlLCJJbmRleCI6MjQsIkV4dHJhVGFnIjpbIlhQYXRo5rOo5YWl5rWL6K+V54K5Il0sIlZlcmJvc2VOYW1lIjoiWFBhdGjms6jlhaXmtYvor5XngrkiLCJOb1JlcGxhY2UiOnRydWV9LHsiUnVsZSI6IigoUE9TVC4qP3dzZGwpfChHRVQuKj93c2RsKXwoeG1sPSl8KDxcXD94bWwgKXwoJmx0O1xcP3htbCkpfCgoUE9TVC4qP2FzbXgpfChHRVQuKj9hc214KSkiLCJDb2xvciI6ImdyZWVuIiwiRW5hYmxlRm9yUmVxdWVzdCI6dHJ1ZSwiRW5hYmxlRm9yUmVzcG9uc2UiOnRydWUsIkVuYWJsZUZvckhlYWRlciI6dHJ1ZSwiRW5hYmxlRm9yQm9keSI6dHJ1ZSwiSW5kZXgiOjI1LCJFeHRyYVRhZyI6WyJYWEXmtYvor5XngrkiXSwiVmVyYm9zZU5hbWUiOiJYWEXmtYvor5XngrkiLCJOb1JlcGxhY2UiOnRydWV9LHsiUnVsZSI6IihmaWxlPXxwYXRoPXx1cmw9fGxhbmc9fHNyYz18bWVudT18bWV0YS1pbmY9fHdlYi1pbmY9fGZpbGVuYW1lPXx0b3BpYz18cGFnZT3vvZxfRmlsZVBhdGg9fHRhcmdldD3vvZxmaWxlcGF0aD0pIiwiQ29sb3IiOiJncmVlbiIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvclJlc3BvbnNlIjp0cnVlLCJFbmFibGVGb3JIZWFkZXIiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4IjoyNiwiRXh0cmFUYWciOlsi5paH5Lu25LiL6L295Y+C5pWwIl0sIlZlcmJvc2VOYW1lIjoi5paH5Lu25LiL6L295Y+C5pWwIiwiTm9SZXBsYWNlIjp0cnVlfSx7IlJ1bGUiOiIoKHVlZGl0b3JcXC4oY29uZmlnfGFsbClcXC5qcykpIiwiQ29sb3IiOiJncmVlbiIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvclJlc3BvbnNlIjp0cnVlLCJFbmFibGVGb3JIZWFkZXIiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4IjoyNywiRXh0cmFUYWciOlsiVUVkaXRvcua1i+ivleeCuSJdLCJWZXJib3NlTmFtZSI6IlVFZGl0b3LmtYvor5XngrkiLCJOb1JlcGxhY2UiOnRydWV9LHsiUnVsZSI6IihraW5kZWRpdG9yXFwtKGFsbFxcLW1pbnxhbGwpXFwuanMpIiwiQ29sb3IiOiJncmVlbiIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvclJlc3BvbnNlIjp0cnVlLCJFbmFibGVGb3JIZWFkZXIiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4IjoyOCwiRXh0cmFUYWciOlsiS2luZEVkaXRvcua1i+ivleeCuSJdLCJWZXJib3NlTmFtZSI6IktpbmRFZGl0b3LmtYvor5XngrkiLCJOb1JlcGxhY2UiOnRydWV9LHsiUnVsZSI6IigoY2FsbGJhY2s9KXwodXJsPSl8KHJlcXVlc3Q9KXwocmVkaXJlY3RfdG89KXwoanVtcD0pfCh0bz0pfChsaW5rPSl8KGRvbWFpbj0pKSIsIkNvbG9yIjoiZ3JlZW4iLCJFbmFibGVGb3JSZXF1ZXN0Ijp0cnVlLCJFbmFibGVGb3JSZXNwb25zZSI6dHJ1ZSwiRW5hYmxlRm9ySGVhZGVyIjp0cnVlLCJFbmFibGVGb3JCb2R5Ijp0cnVlLCJJbmRleCI6MjksIkV4dHJhVGFnIjpbIlVybOmHjeWumuWQkeWPguaVsCJdLCJWZXJib3NlTmFtZSI6IlVybOmHjeWumuWQkeWPguaVsCIsIk5vUmVwbGFjZSI6dHJ1ZX0seyJSdWxlIjoiKHdhcD18dXJsPXxsaW5rPXxzcmM9fHNvdXJjZT18ZGlzcGxheT18c291cmNlVVJsPXxpbWFnZVVSTD18ZG9tYWluPSkiLCJDb2xvciI6ImdyZWVuIiwiRW5hYmxlRm9yUmVxdWVzdCI6dHJ1ZSwiRW5hYmxlRm9yUmVzcG9uc2UiOnRydWUsIkVuYWJsZUZvckhlYWRlciI6dHJ1ZSwiRW5hYmxlRm9yQm9keSI6dHJ1ZSwiSW5kZXgiOjMwLCJFeHRyYVRhZyI6WyJTU1JG5rWL6K+V5Y+C5pWwIl0sIlZlcmJvc2VOYW1lIjoiU1NSRua1i+ivleWPguaVsCIsIk5vUmVwbGFjZSI6dHJ1ZX0seyJSdWxlIjoiKChHRVR8UE9TVHxodHRwW3NdPykuKlxcLihkb3xhY3Rpb24pKVteYS16QS1aXSIsIkNvbG9yIjoicmVkIiwiRW5hYmxlRm9yUmVxdWVzdCI6dHJ1ZSwiRW5hYmxlRm9yUmVzcG9uc2UiOnRydWUsIkVuYWJsZUZvckhlYWRlciI6dHJ1ZSwiRW5hYmxlRm9yQm9keSI6dHJ1ZSwiSW5kZXgiOjMxLCJFeHRyYVRhZyI6WyJTdHJ1dHMy5rWL6K+V54K5Il0sIlZlcmJvc2VOYW1lIjoiU3RydXRzMua1i+ivleeCuSIsIk5vUmVwbGFjZSI6dHJ1ZX0seyJSdWxlIjoiKChHRVR8UE9TVHxodHRwW3NdPykuKj9cXD8uKj8odG9rZW49fHNlc3Npb25cXHcrPSkpIiwiQ29sb3IiOiJncmVlbiIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvclJlc3BvbnNlIjp0cnVlLCJFbmFibGVGb3JIZWFkZXIiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4IjozMiwiRXh0cmFUYWciOlsiU2Vzc2lvbi9Ub2tlbua1i+ivleeCuSJdLCJWZXJib3NlTmFtZSI6IlNlc3Npb24vVG9rZW7mtYvor5XngrkiLCJOb1JlcGxhY2UiOnRydWV9LHsiUnVsZSI6IigoQUtJQXxBR1BBfEFJREF8QVJPQXxBSVBBfEFOUEF8QU5WQXxBU0lBKVthLXpBLVowLTldezE2fSkiLCJDb2xvciI6ImdyZWVuIiwiRW5hYmxlRm9yUmVxdWVzdCI6dHJ1ZSwiRW5hYmxlRm9yUmVzcG9uc2UiOnRydWUsIkVuYWJsZUZvckhlYWRlciI6dHJ1ZSwiRW5hYmxlRm9yQm9keSI6dHJ1ZSwiSW5kZXgiOjMzLCJFeHRyYVRhZyI6WyJBbWF6b24gQUsiXSwiVmVyYm9zZU5hbWUiOiJBbWF6b24gQUsiLCJOb1JlcGxhY2UiOnRydWV9LHsiUnVsZSI6IihEaXJlY3RvcnkgbGlzdGluZyBmb3J8UGFyZW50IERpcmVjdG9yeXxJbmRleCBvZnxmb2xkZXIgbGlzdGluZzopIiwiQ29sb3IiOiJncmVlbiIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvclJlc3BvbnNlIjp0cnVlLCJFbmFibGVGb3JIZWFkZXIiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4IjozNCwiRXh0cmFUYWciOlsi55uu5b2V5p6a5Li+54K5Il0sIlZlcmJvc2VOYW1lIjoi55uu5b2V5p6a5Li+54K5IiwiTm9SZXBsYWNlIjp0cnVlfSx7IlJ1bGUiOiIoPC4qP1VuYXV0aG9yaXplZCkiLCJDb2xvciI6ImdyZWVuIiwiRW5hYmxlRm9yUmVxdWVzdCI6dHJ1ZSwiRW5hYmxlRm9yUmVzcG9uc2UiOnRydWUsIkVuYWJsZUZvckhlYWRlciI6dHJ1ZSwiRW5hYmxlRm9yQm9keSI6dHJ1ZSwiSW5kZXgiOjM1LCJFeHRyYVRhZyI6WyLpnZ7mjojmnYPpobXpnaLngrkiXSwiVmVyYm9zZU5hbWUiOiLpnZ7mjojmnYPpobXpnaLngrkiLCJOb1JlcGxhY2UiOnRydWV9LHsiUnVsZSI6IigoXCJ8Jyk/W3VdKHNlcnxuYW1lfGFtZXxzZXJuYW1lKShcInwnfFxccyk/KDp8PSkuKj8sKSIsIkNvbG9yIjoiZ3JlZW4iLCJFbmFibGVGb3JSZXF1ZXN0Ijp0cnVlLCJFbmFibGVGb3JSZXNwb25zZSI6dHJ1ZSwiRW5hYmxlRm9ySGVhZGVyIjp0cnVlLCJFbmFibGVGb3JCb2R5Ijp0cnVlLCJJbmRleCI6MzYsIkV4dHJhVGFnIjpbIueUqOaIt+WQjeazhOa8j+eCuSJdLCJWZXJib3NlTmFtZSI6IueUqOaIt+WQjeazhOa8j+eCuSIsIk5vUmVwbGFjZSI6dHJ1ZX0seyJSdWxlIjoiKChcInwnKT9bcF0oYXNzfHdkfGFzc3dkfGFzc3dvcmQpKFwifCd8XFxzKT8oOnw9KS4qPywpIiwiQ29sb3IiOiJncmVlbiIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvclJlc3BvbnNlIjp0cnVlLCJFbmFibGVGb3JIZWFkZXIiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4IjozNywiRXh0cmFUYWciOlsi5a+G56CB5rOE5ryP54K5Il0sIlZlcmJvc2VOYW1lIjoi5a+G56CB5rOE5ryP54K5IiwiTm9SZXBsYWNlIjp0cnVlfSx7IlJ1bGUiOiIoKCgoW2EtekEtWjAtOS5fLV0rXFwuczN8czMpKFxcLnxcXC0pK1thLXpBLVowLTkuXy1dK3xbYS16QS1aMC05Ll8tXStcXC5zM3xzMylcXC5hbWF6b25hd3NcXC5jb20pfChzMzpcXC9cXC9bYS16QS1aMC05LVxcLlxcX10rKXwoczMuY29uc29sZS5hd3MuYW1hem9uLmNvbVxcL3MzXFwvYnVja2V0c1xcL1thLXpBLVowLTktXFwuXFxfXSspfChhbXpuXFwubXdzXFwuWzAtOWEtZl17OH0tWzAtOWEtZl17NH0tWzAtOWEtZl17NH0tWzAtOWEtZl17NH0tWzAtOWEtZl17MTJ9KXwoZWMyLVswLTktXSsuY2QtW2EtejAtOS1dKy5jb21wdXRlLmFtYXpvbmF3cy5jb20pfCh1c1tfLV0/ZWFzdFtfLV0/MVtfLV0/ZWxiW18tXT9hbWF6b25hd3NbXy1dP2NvbSkpIiwiQ29sb3IiOiJyZWQiLCJFbmFibGVGb3JSZXF1ZXN0Ijp0cnVlLCJFbmFibGVGb3JSZXNwb25zZSI6dHJ1ZSwiRW5hYmxlRm9ySGVhZGVyIjp0cnVlLCJFbmFibGVGb3JCb2R5Ijp0cnVlLCJJbmRleCI6MzgsIkV4dHJhVGFnIjpbIkFtYXpvbiBBV1MgVVJMIl0sIlZlcmJvc2VOYW1lIjoiQW1hem9uIEFXUyBVUkwiLCJOb1JlcGxhY2UiOnRydWV9LHsiUnVsZSI6Iig/aXMpKDxmb3JtLip0eXBlPS4qP3RleHQuKj88L2Zvcm0uKj8+KSIsIkNvbG9yIjoiZ3JlZW4iLCJFbmFibGVGb3JSZXNwb25zZSI6dHJ1ZSwiRW5hYmxlRm9yQm9keSI6dHJ1ZSwiSW5kZXgiOjM5LCJFeHRyYVRhZyI6WyJIVFRQIFhTU+a1i+ivleeCuSJdLCJWZXJib3NlTmFtZSI6IkhUVFAgWFNT5rWL6K+V54K5IiwiTm9SZXBsYWNlIjp0cnVlfSx7IlJ1bGUiOiIoP2kpKDx0aXRsZT4uKj8o5ZCO5Y+wfGFkbWluKS4qPzwvdGl0bGU+KSIsIkNvbG9yIjoiZ3JlZW4iLCJFbmFibGVGb3JSZXF1ZXN0Ijp0cnVlLCJFbmFibGVGb3JSZXNwb25zZSI6dHJ1ZSwiRW5hYmxlRm9ySGVhZGVyIjp0cnVlLCJFbmFibGVGb3JCb2R5Ijp0cnVlLCJJbmRleCI6NDAsIkV4dHJhVGFnIjpbIuWQjuWPsOeZu+mZhiJdLCJWZXJib3NlTmFtZSI6IuWQjuWPsOeZu+mZhiIsIk5vUmVwbGFjZSI6dHJ1ZX0seyJSdWxlIjoiKChnaHB8Z2h1KVxcX1thLXpBLVowLTldezM2fSkiLCJDb2xvciI6InJlZCIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvclJlc3BvbnNlIjp0cnVlLCJFbmFibGVGb3JIZWFkZXIiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4Ijo0MSwiRXh0cmFUYWciOlsiR2l0aHViQWNjZXNzVG9rZW4iXSwiVmVyYm9zZU5hbWUiOiJHaXRodWJBY2Nlc3NUb2tlbiIsIk5vUmVwbGFjZSI6dHJ1ZX0seyJSdWxlIjoiKChhY2Nlc3M9KXwoYWRtPSl8KGFkbWluPSl8KGFsdGVyPSl8KGNmZz0pfChjbG9uZT0pfChjb25maWc9KXwoY3JlYXRlPSl8KGRiZz0pfChkZWJ1Zz0pfChkZWxldGU9KXwoZGlzYWJsZT0pfChlZGl0PSl8KGVuYWJsZT0pfChleGVjPSl8KGV4ZWN1dGU9KXwoZ3JhbnQ9KXwobG9hZD0pfChtYWtlPSl8KG1vZGlmeT0pfChyZW5hbWU9KXwocmVzZXQ9KXwocm9vdD0pfChzaGVsbD0pfCh0ZXN0PSl8KHRvZ2dsPSkpIiwiQ29sb3IiOiJncmVlbiIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvclJlc3BvbnNlIjp0cnVlLCJFbmFibGVGb3JIZWFkZXIiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4Ijo0MiwiRXh0cmFUYWciOlsi6LCD6K+V5Y+C5pWwIl0sIlZlcmJvc2VOYW1lIjoi6LCD6K+V5Y+C5pWwIiwiTm9SZXBsYWNlIjp0cnVlfSx7IlJ1bGUiOiIoamRiYzpbYS16Ol0rOi8vW0EtWmEtejAtOVxcLlxcLV86Oz0vQD8sJl0rKSIsIkNvbG9yIjoiZ3JlZW4iLCJFbmFibGVGb3JSZXF1ZXN0Ijp0cnVlLCJFbmFibGVGb3JSZXNwb25zZSI6dHJ1ZSwiRW5hYmxlRm9ySGVhZGVyIjp0cnVlLCJFbmFibGVGb3JCb2R5Ijp0cnVlLCJJbmRleCI6NDMsIkV4dHJhVGFnIjpbIkpEQkPov57mjqXlj4LmlbAiXSwiVmVyYm9zZU5hbWUiOiJKREJD6L+e5o6l5Y+C5pWwIiwiTm9SZXBsYWNlIjp0cnVlfSx7IlJ1bGUiOiIoZXlbQS1aYS16MC05Xy1dezEwLH1cXC5bQS1aYS16MC05Ll8tXXsxMCx9fGV5W0EtWmEtejAtOV9cXC8rLV17MTAsfVxcLltBLVphLXowLTkuX1xcLystXXsxMCx9KSIsIkNvbG9yIjoiZ3JlZW4iLCJFbmFibGVGb3JSZXF1ZXN0Ijp0cnVlLCJFbmFibGVGb3JSZXNwb25zZSI6dHJ1ZSwiRW5hYmxlRm9ySGVhZGVyIjp0cnVlLCJFbmFibGVGb3JCb2R5Ijp0cnVlLCJJbmRleCI6NDQsIkV4dHJhVGFnIjpbIkpXVCDmtYvor5XngrkiXSwiVmVyYm9zZU5hbWUiOiJKV1Qg5rWL6K+V54K5IiwiTm9SZXBsYWNlIjp0cnVlfSx7IlJ1bGUiOiIoP2kpKGpzb25wX1thLXowLTldKyl8KChfP2NhbGxiYWNrfF9jYnxfY2FsbHxfP2pzb25wXz8pPSkiLCJDb2xvciI6ImdyZWVuIiwiRW5hYmxlRm9yUmVxdWVzdCI6dHJ1ZSwiRW5hYmxlRm9yUmVzcG9uc2UiOnRydWUsIkVuYWJsZUZvckhlYWRlciI6dHJ1ZSwiRW5hYmxlRm9yQm9keSI6dHJ1ZSwiSW5kZXgiOjQ1LCJFeHRyYVRhZyI6WyJKU09OUCDmtYvor5XngrkiXSwiVmVyYm9zZU5hbWUiOiJqc29ucF9wcmVfdGVzdCIsIk5vUmVwbGFjZSI6dHJ1ZX0seyJSdWxlIjoiKFtjfENdb3JbcHxQXWlkfFtjfENdb3JwW3N8U11lY3JldCkiLCJDb2xvciI6InJlZCIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvclJlc3BvbnNlIjp0cnVlLCJFbmFibGVGb3JIZWFkZXIiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4Ijo0NiwiRXh0cmFUYWciOlsiV2Vjb20gS2V5KFNlY3JldCkiXSwiVmVyYm9zZU5hbWUiOiJXZWNvbSBLZXkoU2VjcmV0KSIsIk5vUmVwbGFjZSI6dHJ1ZX0seyJSdWxlIjoiKGh0dHBzOi8vb3V0bG9va1xcLm9mZmljZVxcLmNvbS93ZWJob29rL1thLXowLTlALV0rL0luY29taW5nV2ViaG9vay9bYS16MC05LV0rL1thLXowLTktXSspIiwiQ29sb3IiOiJyZWQiLCJFbmFibGVGb3JSZXF1ZXN0Ijp0cnVlLCJFbmFibGVGb3JSZXNwb25zZSI6dHJ1ZSwiRW5hYmxlRm9ySGVhZGVyIjp0cnVlLCJFbmFibGVGb3JCb2R5Ijp0cnVlLCJJbmRleCI6NDcsIkV4dHJhVGFnIjpbIk1pY3Jvc29mdFRlYW1zIFdlYmhvb2siXSwiVmVyYm9zZU5hbWUiOiJNaWNyb3NvZnRUZWFtcyBXZWJob29rIiwiTm9SZXBsYWNlIjp0cnVlfSx7IlJ1bGUiOiJodHRwczovL2NyZWF0b3JcXC56b2hvXFwuY29tL2FwaS9bQS1aYS16MC05L1xcLV9cXC5dK1xcP2F1dGh0b2tlbj1bQS1aYS16MC05XSsiLCJDb2xvciI6InJlZCIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvclJlc3BvbnNlIjp0cnVlLCJFbmFibGVGb3JIZWFkZXIiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4Ijo0OCwiRXh0cmFUYWciOlsiWm9obyBXZWJob29rIl0sIlZlcmJvc2VOYW1lIjoiWm9obyBXZWJob29rIiwiTm9SZXBsYWNlIjp0cnVlfSx7IlJ1bGUiOiIoW2EtekEtWl06XFxcXChcXHcrXFxcXCkrfFthLXpBLVpdOlxcXFxcXFxcKFxcdytcXFxcXFxcXCkrKXwoLyhiaW58ZGV2fGhvbWV8bWVkaWF8b3B0fHJvb3R8c2JpbnxzeXN8dXNyfGJvb3R8ZGF0YXxldGN8bGlifG1udHxwcm9jfHJ1bnxzcnZ8dG1wfHZhcikvW148PigpW1xcXSw7Olxcc1wiXSsvKSIsIkNvbG9yIjoicmVkIiwiRW5hYmxlRm9yUmVxdWVzdCI6dHJ1ZSwiRW5hYmxlRm9yUmVzcG9uc2UiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4Ijo0OSwiRXh0cmFUYWciOlsi5pON5L2c57O757uf6Lev5b6EIl0sIlZlcmJvc2VOYW1lIjoi5pON5L2c57O757uf6Lev5b6EIiwiTm9SZXBsYWNlIjp0cnVlfSx7IlJ1bGUiOiIoamF2YXhcXC5mYWNlc1xcLlZpZXdTdGF0ZSkiLCJDb2xvciI6ImJsdWUiLCJFbmFibGVGb3JSZXF1ZXN0Ijp0cnVlLCJFbmFibGVGb3JSZXNwb25zZSI6dHJ1ZSwiRW5hYmxlRm9ySGVhZGVyIjp0cnVlLCJFbmFibGVGb3JCb2R5Ijp0cnVlLCJJbmRleCI6NTAsIkV4dHJhVGFnIjpbIkphdmHlj43luo/liJfljJbmtYvor5XngrkiXSwiVmVyYm9zZU5hbWUiOiJKYXZh5Y+N5bqP5YiX5YyW5rWL6K+V54K5IiwiTm9SZXBsYWNlIjp0cnVlfSx7IlJ1bGUiOiIoc29uYXIuezAsNTB9KD86XCJ8XFwnfGApP1swLTlhLWZdezQwfSg/OlwifFxcJ3xgKT8pIiwiQ29sb3IiOiJyZWQiLCJFbmFibGVGb3JSZXF1ZXN0Ijp0cnVlLCJFbmFibGVGb3JSZXNwb25zZSI6dHJ1ZSwiRW5hYmxlRm9ySGVhZGVyIjp0cnVlLCJFbmFibGVGb3JCb2R5Ijp0cnVlLCJJbmRleCI6NTEsIkV4dHJhVGFnIjpbIlNvbmFycXViZSBUb2tlbiJdLCJWZXJib3NlTmFtZSI6IlNvbmFycXViZSBUb2tlbiIsIk5vUmVwbGFjZSI6dHJ1ZX0seyJSdWxlIjoiKCh1cygtZ292KT98YXB8Y2F8Y258ZXV8c2EpLShjZW50cmFsfChub3J0aHxzb3V0aCk/KGVhc3R8d2VzdCk/KS1cXGQpIiwiQ29sb3IiOiJyZWQiLCJFbmFibGVGb3JSZXF1ZXN0Ijp0cnVlLCJFbmFibGVGb3JSZXNwb25zZSI6dHJ1ZSwiRW5hYmxlRm9ySGVhZGVyIjp0cnVlLCJFbmFibGVGb3JCb2R5Ijp0cnVlLCJJbmRleCI6NTIsIkV4dHJhVGFnIjpbIkFtYXpvbiBBV1MgUmVnaW9u5rOE5ryPIl0sIlZlcmJvc2VOYW1lIjoiQW1hem9uIEFXUyBSZWdpb27ms4TmvI8iLCJOb1JlcGxhY2UiOnRydWV9LHsiUnVsZSI6Iig9KGh0dHBzPzovLy4qfGh0dHBzPyUzKGF8QSklMihmfEYpJTIoZnxGKS4qKSkiLCJDb2xvciI6ImJsdWUiLCJFbmFibGVGb3JSZXF1ZXN0Ijp0cnVlLCJFbmFibGVGb3JSZXNwb25zZSI6dHJ1ZSwiRW5hYmxlRm9ySGVhZGVyIjp0cnVlLCJFbmFibGVGb3JCb2R5Ijp0cnVlLCJJbmRleCI6NTMsIkV4dHJhVGFnIjpbIlVSTOS9nOS4uuWPguaVsCJdLCJWZXJib3NlTmFtZSI6IlVSTOS9nOS4uuWPguaVsCIsIk5vUmVwbGFjZSI6dHJ1ZX0seyJSdWxlIjoiKHlhMjlcXC5bMC05QS1aYS16Xy1dKykiLCJDb2xvciI6InJlZCIsIkVuYWJsZUZvclJlcXVlc3QiOnRydWUsIkVuYWJsZUZvclJlc3BvbnNlIjp0cnVlLCJFbmFibGVGb3JIZWFkZXIiOnRydWUsIkVuYWJsZUZvckJvZHkiOnRydWUsIkluZGV4Ijo1NCwiRXh0cmFUYWciOlsiT2F1dGggQWNjZXNzIEtleSJdLCJWZXJib3NlTmFtZSI6Ik9hdXRoIEFjY2VzcyBLZXkiLCJOb1JlcGxhY2UiOnRydWV9LHsiUnVsZSI6IihFcnJvciByZXBvcnR8aW4geW91ciBTUUwgc3ludGF4fG15c3FsX2ZldGNoX2FycmF5fG15c3FsX2Nvbm5lY3QoKXxvcmcuYXBhY2hlLmNhdGFsaW5hKSIsIkNvbG9yIjoicmVkIiwiRW5hYmxlRm9yUmVxdWVzdCI6dHJ1ZSwiRW5hYmxlRm9yUmVzcG9uc2UiOnRydWUsIkVuYWJsZUZvckhlYWRlciI6dHJ1ZSwiRW5hYmxlRm9yQm9keSI6dHJ1ZSwiSW5kZXgiOjU1LCJFeHRyYVRhZyI6WyLnvZHnq5nlh7rplJkiXSwiVmVyYm9zZU5hbWUiOiLnvZHnq5nlh7rplJkiLCJOb1JlcGxhY2UiOnRydWV9XQ==`
	ruleBytes, _ := codec.DecodeBase64(rule)
	_, err = client.ImportMITMReplacerRules(ctx, &ypb.ImportMITMReplacerRulesRequest{
		JsonRaw:    []byte(ruleBytes),
		ReplaceAll: true,
	})
	if err != nil {
		panic("IMPORT MITM REPLACER RULE FAILED")
	}

	// 启动MITM服务器
	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host:             "127.0.0.1",
		Port:             uint32(rPort),
		Recover:          true,
		Forward:          true,
		SetAutoForward:   true,
		AutoForwardValue: true,
		EnableHttp2:      true,
	})
	var wg sync.WaitGroup
	wg.Add(1)
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if strings.Contains(spew.Sdump(rsp), `starting mitm server`) && !started {
			println("----------------------")
			started = true
			go func() {
				defer func() {
					wg.Done()
					cancel()
				}()
				var token = utils.RandStringBytes(100)
				var params = map[string]any{
					"packet": lowhttp.FixHTTPRequest(lowhttp.ReplaceHTTPPacketHeader([]byte(`GET / HTTP/1.1
Host: www.example.com

`+token), "Host", utils.HostPort(mockHost, mockPort))),
					"host":  mockHost,
					"port":  mockPort,
					"proxy": proxy,
					"token": token,
				}
				// 将MITM作为代理向mock的http服务器发包 这个过程成功说明 MITM开启H2支持的情况下 能够正确处理H1请求
				_, err := yak.NewScriptEngine(10).ExecuteEx(`
log.info("Start to send packet echo")
packet := getParam("packet")
host, port = getParam("host"), getParam("port")
rsp, req = poc.HTTP(string(packet), poc.proxy(getParam("proxy")), poc.host(host), poc.port(port))~
if rsp.Contains(getParam("token")) {
	println("基础发包测试：success")	
}else{
	die("echo test not pass!")
}
`, params)
				if err != nil {
					t.Fatal(err)
				}
				echoTested = true

				var tokenRaw, _ = utils.GzipCompress([]byte(token))
				params["packet"] = "GET /gziptestted HTTP/1.1\r\nHost: " + utils.HostPort(mockHost, mockPort)
				params["packet"] = lowhttp.ReplaceHTTPPacketBody(utils.InterfaceToBytes(params["packet"]), tokenRaw, false)
				params["packet"] = lowhttp.ReplaceHTTPPacketHeader(utils.InterfaceToBytes(params["packet"]), "Content-Encoding", "gzip")
				time.Sleep(time.Second)
				_, err = yak.NewScriptEngine(10).ExecuteEx(`
log.info("Start to send packet echo")
packet := getParam("packet")
host, port = getParam("host"), getParam("port")
rsp, req = poc.HTTP(string(packet), poc.proxy(getParam("proxy")), poc.host(host), poc.port(port))~
if rsp.Contains(getParam("token")) {
		println("gzip auto decode success")	
}else{
	dump(rsp)
	die("gzipAutoDecode not pass!")
}
`, params)
				if err != nil {
					t.Fatal(err)
				}
				gzipAutoDecode = true

				tokenRaw, _ = utils.GzipCompress([]byte(token))
				params["packet"] = "GET /chunked-and-gziped-test HTTP/1.1\r\nHost: " + utils.HostPort(mockHost, mockPort)
				params["packet"] = lowhttp.ReplaceHTTPPacketHeader(utils.InterfaceToBytes(params["packet"]), "Content-Encoding", "gzip")
				params["packet"] = lowhttp.ReplaceHTTPPacketBody(utils.InterfaceToBytes(params["packet"]), tokenRaw, true)
				originPacket := params["packet"].([]byte)
				_ = originPacket

				time.Sleep(time.Second)
				_, err = yak.NewScriptEngine(10).ExecuteEx(`
log.info("Start to send packet echo")
packet := getParam("packet")
host, port = getParam("host"), getParam("port")
rsp, req = poc.HTTP(string(packet), poc.proxy(getParam("proxy")), poc.host(host), poc.port(port), poc.retryTimes(3))~
if rsp.Contains(getParam("token")) {
		println("chunk + gzip auto decode success")	
}else{
	dump(rsp)
	die("chunkDecode + gzip not pass!")
}
`, params)
				if err != nil {
					t.Fatal(err)
				}
				chunkDecode = true

				tokenRaw = []byte(token)
				params["h2packet"] = lowhttp.ReplaceHTTPPacketBody([]byte(`GET /mitm/test/h2/token/`+token+` HTTP/2.0
Host: `+h2Addr+`
D: 1
`), tokenRaw, false)
				params["h2host"] = h2Host
				params["h2port"] = h2Port

				_, err = yak.NewScriptEngine(10).ExecuteEx(`
log.info("Start to send packet h2")
packet := getParam("h2packet")
println("-------------------------------------------------------------------------------------")
dump(packet)
retry := 10
var rsp, req, err
for retry >0{
	rsp, req, err = poc.HTTP(string(packet), poc.proxy(getParam("proxy")), poc.http2(true), poc.https(true))
	if err != nil{
		retry = retry -1
		sleep(0.5)
		continue
	}
	break
}
if rsp.Contains(getParam("token")) {
		println("h2 auto decode success")	
}else{
	dump(rsp)
	die("not pass!")
}
println("-------------------------------------------------------------------------------------")
`, params)
				if err != nil {
					t.Fatal(err)
				}

				ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
				_ = ctx
				defer cancel()
				time.Sleep(time.Second)
				_, flows, err := yakit.QueryHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{
					SearchURL: "/mitm/test/h2/token/" + token,
				})
				if err != nil {
					t.Fatal(err)
				}
				if len(flows) > 0 {
					h2Test = true
				} else {
					panic("/mitm/test/h2/token/" + token + " is not logged in db")
				}
			}()
		}
	}
	wg.Wait()

	if !started {
		t.Fatal("MITM NOT STARTED!")
	}

	if !passthroughTested {
		t.Fatal("MITM PASSTHROUGH TEST FAILED")
	}

	if !echoTested {
		t.Fatal("MITM ECHO TEST FAILED")
	}

	if !gzipAutoDecode {
		t.Fatal("GZIP AUTO DECODE FAILED")
	}

	if !chunkDecode {
		t.Fatal("CHUNK DECODE FAILED")
	}

	if !h2Test {
		panic("H2 TEST FAILED")
	}
}

func TestGRPCMUSTPASS_MITM_GM(t *testing.T) {
	client, err := NewLocalClient() // 新建一个 yakit client
	if err != nil {
		t.Fatal(err)
	}

	var (
		started                bool // MITM正常启动（此时MITM开启HTTP2支持）
		gmPassthroughTested    bool // Mock的GM-HTTPS服务器正常工作
		httpPassthroughTested  bool // Mock的HTTP服务器正常工作
		httpsPassthroughTested bool // Mock的HTTPS服务器正常工作
		httpTest               bool // 将开启了GM支持的MITM作为代理向mock的HTTP服务器发包 这个过程成功说明 MITM开启GM支持的情况下 能够正确处理HTTP请求和响应
		httpsTest              bool // 将开启了GM支持的MITM作为代理向mock的HTTPS服务器发包 这个过程成功说明 MITM开启GM支持的情况下 能够正确处理Vanilla-HTTPS请求和响应
		gmTest                 bool // 将开启了GM支持的MITM作为代理向mock的GM-HTTPS服务器发包 这个过程成功说明 MITM开启GM支持的情况下 能够正确处理GM-HTTPS请求和响应
	)

	var mockGMHost, mockGMPort = utils.DebugMockGMHTTP(context.Background(), func(req []byte) []byte {
		gmPassthroughTested = true // 测试标识位 收到了http请求
		rsp, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 OK
Content-Type: text/html
Content-Length: 3

111`))
		_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(req)
		rsp = lowhttp.ReplaceHTTPPacketBodyFast(rsp, body) // 返回包的body是请求包的body
		if lowhttp.GetHTTPPacketHeader(req, "Content-Encoding") == "gzip" {
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Content-Encoding", "gzip")
		}
		if lowhttp.GetHTTPPacketHeader(req, "Transfer-Encoding") == "chunked" {
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Transfer-Encoding", "chunked")
		}
		return rsp
	})
	var mockHost, mockPort = utils.DebugMockHTTPEx(func(req []byte) []byte {
		httpPassthroughTested = true // 测试标识位 收到了http请求
		rsp, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 OK\n
Content-Type: text/html
Content-Length: 3

111`))
		_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(req)
		rsp = lowhttp.ReplaceHTTPPacketBodyFast(rsp, body) // 返回包的body是请求包的body
		if lowhttp.GetHTTPPacketHeader(req, "Content-Encoding") == "gzip" {
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Content-Encoding", "gzip")
		}
		if lowhttp.GetHTTPPacketHeader(req, "Transfer-Encoding") == "chunked" {
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Transfer-Encoding", "chunked")
		}
		return rsp
	})
	var mockHttpsHost, mockHttpsPort = utils.DebugMockHTTPSEx(func(req []byte) []byte {
		httpsPassthroughTested = true // 测试标识位 收到了http请求
		rsp, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 OK\n
Content-Type: text/html
Content-Length: 3

111`))
		_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(req)
		rsp = lowhttp.ReplaceHTTPPacketBodyFast(rsp, body) // 返回包的body是请求包的body
		if lowhttp.GetHTTPPacketHeader(req, "Content-Encoding") == "gzip" {
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Content-Encoding", "gzip")
		}
		if lowhttp.GetHTTPPacketHeader(req, "Transfer-Encoding") == "chunked" {
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Transfer-Encoding", "chunked")
		}
		return rsp
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer func() {
		cancel()
	}()

	var rPort = utils.GetRandomAvailableTCPPort()
	var proxy = "http://127.0.0.1:" + fmt.Sprint(rPort)
	_ = proxy

	// 启动MITM服务器
	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host:             "127.0.0.1",
		Port:             uint32(rPort),
		Recover:          true,
		Forward:          true,
		SetAutoForward:   true,
		AutoForwardValue: true,
		EnableGMTLS:      true,
	})

	var wg sync.WaitGroup
	wg.Add(1)
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if strings.Contains(spew.Sdump(rsp), `starting mitm server`) && !started {
			println("--------------------------------------------")

			started = true

			var token = utils.RandStringBytes(100)
			var params = map[string]any{
				"packet": lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /GMTLS`+token+` HTTP/1.1
Host: www.example.com

`+token), "Host", utils.HostPort(mockGMHost, mockGMPort)),
				"proxy": proxy,
				"token": token,
			}

			params["gmHost"] = mockGMHost
			params["gmPort"] = mockGMPort
			_, err = yak.NewScriptEngine(10).ExecuteEx(`
log.info("Start to send packet echo")
packet := getParam("packet")
host, port = getParam("gmHost"), getParam("gmPort")
rsp, req = poc.HTTP(string(packet), poc.proxy(getParam("proxy")), poc.host(host), poc.port(port), poc.https(true))~
if rsp.Contains(getParam("token")) {
		println("success")	
}else{
	dump(rsp)
	die("GM HTTPS not pass!")
}
`, params)
			if err != nil {
				t.Fatal(err)
			}
			gmPassthroughTested = true

			params["packet"] = lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /HTTPS`+token+` HTTP/1.1
Host: www.example.com

`+token), "Host", utils.HostPort(mockHttpsHost, mockHttpsPort))
			params["httpsHost"] = mockHttpsHost
			params["httpsPort"] = mockHttpsPort
			_, err = yak.NewScriptEngine(10).ExecuteEx(`
log.info("Start to send packet echo")
packet := getParam("packet")
host, port = getParam("httpsHost"), getParam("httpsPort")
rsp, req = poc.HTTP(string(packet), poc.proxy(getParam("proxy")), poc.host(host), poc.port(port), poc.https(true))~
if rsp.Contains(getParam("token")) {
		println("success")	
}else{
	dump(rsp)
	die("TLS HTTPS not pass!")
}
`, params)
			if err != nil {
				t.Fatal(err)
			}
			httpsPassthroughTested = true

			params["packet"] = lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /HTTP`+token+` HTTP/1.1
Host: www.example.com

`+token), "Host", utils.HostPort(mockHost, mockPort))
			params["host"] = mockHost
			params["port"] = mockPort
			_, err = yak.NewScriptEngine(10).ExecuteEx(`
log.info("Start to send packet echo")
packet := getParam("packet")
host, port = getParam("host"), getParam("port")
rsp, req = poc.HTTP(string(packet), poc.proxy(getParam("proxy")), poc.host(host), poc.port(port))~
if rsp.Contains(getParam("token")) {
		println("success")	
}else{
	dump(rsp)
	die("Plain HTTP not pass!")
}
`, params)
			if err != nil {
				t.Fatal(err)
			}
			httpsPassthroughTested = true

			time.Sleep(time.Second)
			_, flows, err := yakit.QueryHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{
				SearchURL: "/GMTLS" + token,
			})
			if err != nil {
				t.Fatal(err)
			}

			if len(flows) > 0 {
				gmTest = true
			}

			_, flows, err = yakit.QueryHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{
				SearchURL: "/HTTPS" + token,
			})
			if err != nil {
				t.Fatal(err)
			}

			if len(flows) > 0 {
				httpsTest = true
			}

			// 执行查询操作
			_, flows, err = yakit.QueryHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{
				SearchURL: "/HTTP" + token,
			})
			if err != nil {
				t.Fatal(err)
			}

			if len(flows) > 0 {
				httpTest = true
			}
			break
		}
	}

	time.Sleep(time.Second)
	if !started {
		panic("MITM NOT STARTED!")
	}

	if !gmPassthroughTested {
		panic("GM PassthroughTEST FAILED")
	}

	if !gmTest {
		panic("GM TEST FAILED")
	}

	if !httpsPassthroughTested {
		panic("HTTPS PassthroughTEST FAILED")
	}

	if !httpsTest {
		panic("HTTPS TEST FAILED")
	}

	if !httpPassthroughTested {
		panic("HTTP PassthroughTEST FAILED")
	}

	if !httpTest {
		panic("HTTP TEST FAILED")
	}

}

// TestGRPCMUSTPASS_MITM_Drop 测试MITM设置手动劫持并丢弃响应后MITM响应的行为和HTTP History的记录是否符合预期
func TestGRPCMUSTPASS_MITM_Drop(t *testing.T) {
	client, err := NewLocalClient() // 新建一个 yakit client
	if err != nil {
		t.Fatal(err)
	}

	var (
		started         bool // MITM正常启动（此时MITM开启HTTP2支持
		h2Test          bool // 将MITM作为代理向mock的http2服务器发包 这个过程成功说明 MITM开启H2支持的情况下 能够正确处理H2请求和响应
		h2serverHandled int
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer func() {
		cancel()
	}()

	/* H2 */
	h2Host, h2Port := utils.DebugMockHTTP2(ctx, func(req []byte) []byte {
		h2serverHandled++
		return req
	})
	h2Addr := utils.HostPort(h2Host, h2Port)
	log.Infof("start to mock h2 server: %v", utils.HostPort(h2Host, h2Port))
	var rPort = utils.GetRandomAvailableTCPPort()
	var proxy = "http://127.0.0.1:" + fmt.Sprint(rPort)
	// 测试我们的h2 mock服务器是否正常工作
	_, err = yak.NewScriptEngine(10).ExecuteEx(`
rsp,req = poc.HTTP(getParam("packet"), poc.http2(true), poc.https(true),poc.save(false))~
`, map[string]any{
		"packet": `GET / HTTP/2.0
User-Agent: 111
Host: ` + h2Addr,
	})
	if err != nil {
		t.Fatal(err)
	}
	// 启动MITM服务器
	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host:        "127.0.0.1",
		Port:        uint32(rPort),
		Recover:     true,
		EnableHttp2: true,
	})
	var wg sync.WaitGroup
	wg.Add(1)
	dropped := false
	manual := false
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if strings.Contains(spew.Sdump(rsp), `starting mitm server`) && !started {
			started = true
			// 前置测试会替换默认的规则导致运行到MITM GRPC测试时，过滤器不再是默认值，这会影响手动劫持规则，导致connect请求被拦截，进而超时
			// 因此此处做重置过滤器操作
			stream.Send(&ypb.MITMRequest{
				SetResetFilter: true,
			})
			stream.Send(&ypb.MITMRequest{
				SetAutoForward:   true,
				AutoForwardValue: false, //手动劫持
			})
			time.Sleep(time.Second * 3)
			manual = true
			go func() {
				defer func() {
					wg.Done()
					cancel()
				}()
				var token = utils.RandStringBytes(100)
				var params = map[string]any{
					"proxy": proxy,
					"token": token,
				}
				tokenRaw := []byte(token)
				params["h2packet"] = lowhttp.ReplaceHTTPPacketBody([]byte(`GET /mitm/test/h2/drop/token/`+token+` HTTP/2.0
Host: `+h2Addr+`
D: 1
`), tokenRaw, false)
				params["h2host"] = h2Host
				params["h2port"] = h2Port

				_, err = yak.NewScriptEngine(10).ExecuteEx(`
log.info("Start to send packet h2")
packet := getParam("h2packet")
println("-------------------------------------------------------------------------------------")
a, b, _ = poc.HTTP(string(packet), poc.proxy(getParam("proxy")), poc.https(true), poc.http2(true), poc.timeout(120),poc.save(false))
`, params)
				if err != nil {
					t.Fatal(err)

				}
				defer cancel()
				if utils.Spinlock(15, func() bool {
					return dropped
				}) == nil {
					_, flows, err := yakit.QueryHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{
						SearchURL: "/mitm/test/h2/drop/token/" + token,
					})
					if err != nil {
						t.Fatal(err)
					}
					if len(flows) > 0 && len(flows[0].Response) == 0 { // 被用户手动丢弃的请求 不会有响应
						h2Test = true
					} else if len(flows) == 0 {
						t.Fatal("/mitm/test/h2/drop/token/" + token + " not found")
					} else if !strings.Contains(flows[0].Tags, "被丢弃") {
						t.Fatal("/mitm/test/h2/drop/token/" + token + "should indicate user manually drop in http history")
					} else {
						t.Fatal("unknown err")
					}
				}

			}()
		}

		if started && manual && strings.Contains(spew.Sdump(rsp), `/mitm/test/h2/drop/`) {
			err := stream.Send(&ypb.MITMRequest{
				Id:   rsp.GetId(),
				Drop: true,
			})
			dropped = true
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	wg.Wait()
	if !started {
		t.Fatal("MITM NOT STARTED!")
	}

	if !h2Test {
		t.Fatal("H2 TEST FAILED")
	}

	if h2serverHandled <= 0 {
		t.Fatal("H2 SERVER NOT HANDLED")
	}
}

func TestGRPCMUSTPASS_MITMDnsAndHosts(t *testing.T) {
	client, err := NewLocalClient() // 新建一个 yakit client
	if err != nil {
		t.Fatal(err)
	}

	port1 := utils.GetRandomAvailableTCPPort()

	// mock http server
	go func() {
		err = facades.Serve("127.0.0.1", port1, facades.SetHttpResource("/ok", []byte("")))
		if err != nil {
			t.Fatal(err)
		}
	}()
	err = utils.WaitConnect(fmt.Sprintf("127.0.0.1:%d", port1), 5)
	if err != nil {
		t.Fatal(err)
	}

	hostForDns := utils.RandStringBytes(10) + ".com"
	hostForHost := utils.RandStringBytes(10) + ".com"
	dnsRecordCount := 0
	// mock dns server
	var dnsServer = facades.MockDNSServerDefault(hostForDns, func(record string, domain string) string {

		dnsRecordCount++
		return "127.0.0.1"
	})
	defer func() {
		if dnsRecordCount != 5 {
			t.Fatal("dns server should be called")
		}
	}()

	for _, mitmConfig := range []func(request *ypb.MITMRequest){
		func(request *ypb.MITMRequest) {},
		func(request *ypb.MITMRequest) {
			request.EnableGMTLS = true
		},
		func(request *ypb.MITMRequest) {
			request.EnableHttp2 = true
		},
		func(request *ypb.MITMRequest) {
			request.EnableHttp2 = true
			request.EnableGMTLS = true
			request.PreferGMTLS = true
		},
		func(request *ypb.MITMRequest) {
			request.EnableHttp2 = true
			request.EnableGMTLS = true
			request.OnlyEnableGMTLS = true
		},
	} {
		// start mitm server
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		stream, err := client.MITM(ctx)
		if err != nil {
			t.Fatalf("start mitm stream failed: %s", err)
		}
		port := utils.GetRandomAvailableTCPPort()
		mitmAddr := fmt.Sprintf("127.0.0.1:%d", port)
		request := &ypb.MITMRequest{
			Host:       "127.0.0.1",
			Port:       uint32(port),
			DnsServers: []string{dnsServer},
			Hosts: []*ypb.KVPair{
				{
					Key:   hostForHost,
					Value: "127.0.0.1",
				},
			},
		}
		mitmConfig(request)
		err = stream.Send(request)
		if err != nil {
			t.Fatalf("send mitm request failed: %s", err)
		}
		// wait mitm server started
		//err = utils.WaitConnect(mitmAddr, 5)
		//if err != nil {
		//	t.Fatal(err)
		//}

		for {
			msg, err := stream.Recv()
			if err != nil {
				break
			}
			msgStr := string(msg.GetMessage().GetMessage())
			if strings.Contains(msgStr, `starting mitm server`) {
				for _, host := range []string{hostForDns, hostForHost} {
					urlForDns := "http://" + fmt.Sprintf("%s:%d/ok", host, port1)
					_, err := yak.Execute(
						`rsp, req := poc.Get(urlForDns, poc.proxy(proxy))~; println(string(rsp.RawPacket))`,
						map[string]interface{}{
							"urlForDns": urlForDns,
							"proxy":     "http://" + mitmAddr,
						},
					)
					if err != nil {
						t.Fatalf("get url `%v` failed: %s", urlForDns, err)
					}
				}
				cancel()
			}
		}
	}
}

func TestMitmDropWithHijackResp(t *testing.T) {
	client, err := NewLocalClient() // 新建一个 yakit client
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	host, port := utils.DebugMockHTTPExContext(ctx, func(req []byte) []byte {

		return []byte(`HTTP/1.1 200 OK
Content-Type: text/html

ok
`)
	})

	addr := utils.HostPort(host, port)
	log.Infof("start to mock h2 server: %v", utils.HostPort(host, port))
	var rPort = utils.GetRandomAvailableTCPPort()
	var proxy = "http://127.0.0.1:" + fmt.Sprint(rPort)
	//启动mitm服务器
	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(rPort),
	})

	packet := []byte(`GET / HTTP/1.1
User-Agent: 111
Host: ` + addr)

	timeChecker := time.AfterFunc(5*time.Second, func() {
		cancel()
		t.Fatal("timeout err")
	})
	var hasDrop, started bool
	var useID int64
	for {
		rsp, err := stream.Recv()
		timeChecker.Reset(5 * time.Second)
		if err != nil {
			break
		}
		if hasDrop && len(rsp.GetResponse()) > 0 {
			t.Fatal("hijackResp err")
		}
		if len(rsp.GetRequest()) > 0 {
			err := stream.Send(&ypb.MITMRequest{
				Id:             rsp.GetId(),
				HijackResponse: true,
			})
			err = stream.Send(&ypb.MITMRequest{
				Id:   rsp.GetId(),
				Drop: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if hasDrop && rsp.GetId() != useID {
				cancel()
				break
			}

			hasDrop = true
			useID = rsp.GetId()
		}
		//启动完毕之后换手动劫持，开始发包
		if strings.Contains(spew.Sdump(rsp), `starting mitm server`) && !started {
			started = true
			stream.Send(&ypb.MITMRequest{
				SetAutoForward:   true,
				AutoForwardValue: false, //手动劫持
			})
			time.Sleep(1 * time.Second)
			go func() {
				for i := 0; i < 10; i++ {
					_, err := lowhttp.HTTPWithoutRedirect(lowhttp.WithPacketBytes(packet), lowhttp.WithProxy(proxy))
					if err != nil {
						log.Infof("send packet err : [%v]", err)
					}
				}
			}()
		}
	}
}

func TestHijackResp(t *testing.T) {
	client, err := NewLocalClient() // 新建一个 yakit client
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	host, port := utils.DebugMockHTTPExContext(ctx, func(req []byte) []byte {

		return []byte(`HTTP/1.1 200 OK
Content-Type: text/html

ok
`)
	})
	addr := utils.HostPort(host, port)
	log.Infof("start to mock http server: %v", utils.HostPort(host, port))
	var rPort = utils.GetRandomAvailableTCPPort()
	var proxy = "http://127.0.0.1:" + fmt.Sprint(rPort)
	//启动mitm服务器
	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(rPort),
	})

	packet := `GET /%d HTTP/1.1
User-Agent: 111
Host: ` + addr
	var hasForward, started bool
	var useID int64
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}

		if len(rsp.GetResponse()) > 0 && hasForward {
			cancel()
			break
		}

		if len(rsp.GetRequest()) > 0 {
			if hasForward && useID != rsp.GetId() {
				t.Fatal("hijack resp err : [get other request]")
			}

			err := stream.Send(&ypb.MITMRequest{
				Id:             rsp.GetId(),
				HijackResponse: true,
			})
			err = stream.Send(&ypb.MITMRequest{
				Id:         rsp.GetId(),
				Request:    rsp.GetRequest(),
				ResponseId: rsp.GetResponseId(),
			})
			if err != nil {
				t.Fatal(err)
			}
			log.Infof("get packet")
			useID = rsp.GetId()
			hasForward = true
		}
		//启动完毕之后换手动劫持，开始发包
		if strings.Contains(spew.Sdump(rsp), `starting mitm server`) && !started {
			started = true
			stream.Send(&ypb.MITMRequest{
				SetAutoForward:   true,
				AutoForwardValue: false, //手动劫持
			})
			time.Sleep(1 * time.Second)
			go func() {
				for i := 0; i < 10; i++ {
					_, err := lowhttp.HTTPWithoutRedirect(lowhttp.WithPacketBytes([]byte(fmt.Sprintf(packet, i))), lowhttp.WithProxy(proxy))
					if err != nil {
						t.Fatal(err)
					}
				}
			}()
		}
	}
}

func TestGRPCMUSTPASS_MITMCancelHijackResponse(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	token := utils.RandStringBytes(20)
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		fmt.Fprint(writer, token)
	})
	target := utils.HostPort(host, port)

	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(5))
	defer cancel()
	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}

	mitmPort := utils.GetRandomAvailableTCPPort()
	stream.Send(&ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})
	stream.Send(&ypb.MITMRequest{
		SetResetFilter: true,
	})
	stream.Send(&ypb.MITMRequest{SetAutoForward: true, AutoForwardValue: false})
	once := false
	for {
		rcpResponse, err := stream.Recv()
		if err != nil {
			break
		}
		rspMsg := string(rcpResponse.GetMessage().GetMessage())
		if rcpResponse.GetHaveMessage() {

		} else if len(rcpResponse.GetRequest()) > 0 {

			// 模拟用户点击切换劫持响应为从不
			if !once {
				once = true
				stream.Send(&ypb.MITMRequest{
					Id:             rcpResponse.GetId(),
					HijackResponse: true,
				})
				time.Sleep(100 * time.Microsecond)
				stream.Send(&ypb.MITMRequest{
					Id:                   rcpResponse.GetId(),
					CancelhijackResponse: true, // 代表取消劫持响应
				})
				time.Sleep(100 * time.Microsecond)
			}

			stream.Send(&ypb.MITMRequest{
				Id:      rcpResponse.GetId(),
				Request: rcpResponse.GetRequest(),
			})

			// 如果劫持了响应，第二次会进来
			if len(rcpResponse.GetResponse()) > 0 {
				t.Fatalf("Should not hijack response, but hijacked")
			}
		}
		if strings.Contains(rspMsg, `starting mitm serve`) {
			go func() {
				time.Sleep(time.Second)
				packet := `GET / HTTP/1.1
Host: ` + target
				_, err := yak.Execute(`
rsp, req = poc.HTTP(packet, poc.proxy(mitmProxy))~
`, map[string]any{
					"packet":    []byte(packet),
					"mitmProxy": "http://" + utils.HostPort("127.0.0.1", mitmPort),
				})
				if err != nil {
					t.Fatal(err)
				}
				cancel()
			}()
		}
	}

}

func TestGRPCMUSTPASS_MITM_LegacyProxy(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(40))
	defer cancel()
	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}

	mitmPort := utils.GetRandomAvailableTCPPort()
	stream.Send(&ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})

	var token = utils.RandSecret(100)
	var pass = false
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/abc" {
			pass = true
			writer.Write([]byte(token))
		}
	})

	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}

		msg := string(rsp.GetMessage().GetMessage())
		fmt.Println(msg)
		if strings.Contains(msg, `starting mitm server`) {
			packet, err := lowhttp.BuildLegacyProxyRequest(
				lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /abc HTTP/1.1
Host: example.com

`), "Host", utils.HostPort(host, port)))
			if err != nil {
				t.Fatal(err)
			}
			conn, err := netx.DialX(utils.HostPort("127.0.0.1", mitmPort), netx.DialX_WithDisableProxy(true))
			if err != nil {
				spew.Dump(err)
				t.Fatal("dialx mitm proxy failed")
			}
			conn.Write(packet)
			rsp, err := utils.ReadHTTPResponseFromBufioReader(bufio.NewReader(conn), nil)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := utils.HttpDumpWithBody(rsp, true)
			if !bytes.Contains(raw, []byte(token)) {
				t.Fatal("no token found")
			}
			cancel()
			break
		}
	}

	if !pass {
		t.Fatal("not pass")
	}
}

func TestGRPCMUSTPASS_MITM_LegacyProxyLowhttp(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(40))
	defer cancel()
	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}

	mitmPort := utils.GetRandomAvailableTCPPort()
	stream.Send(&ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})

	var token = utils.RandSecret(100)
	var pass = false
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/abc" {
			pass = true
			writer.Write([]byte(token))
		}
	})

	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}

		msg := string(rsp.GetMessage().GetMessage())
		fmt.Println(msg)
		if strings.Contains(msg, `starting mitm server`) {
			packet := lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /abc HTTP/1.1
Host: example.com

`), "Host", utils.HostPort(host, port))
			rsp, err := lowhttp.HTTP(
				lowhttp.WithPacketBytes(packet),
				lowhttp.WithProxy("http://"+utils.HostPort("127.0.0.1", mitmPort)),
				lowhttp.WithForceLegacyProxy(true),
				lowhttp.WithHost("127.0.0.1"),
				lowhttp.WithPort(mitmPort),
			)
			if err != nil {
				spew.Dump(err)
				t.Fatal("lowhttp mitm proxy failed")
			}
			var raw = rsp.RawPacket
			if !bytes.Contains(raw, []byte(token)) {
				t.Fatal("no token found")
			}
			cancel()
			break
		}
	}

	if !pass {
		t.Fatal("not pass")
	}
}
