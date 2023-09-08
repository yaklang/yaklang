package yakgrpc

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"strings"
	"testing"
)

func TestGRPCMUSTPASS_MITM_PlainProxy(t *testing.T) {
	var ctx, cancel = context.WithCancel(utils.TimeoutContextSeconds(5))
	defer cancel()

	mockHost, mockPort := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("hello"))
	})

	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	stream, err := client.MITM(ctx)
	if err != nil {
		panic(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})
	token := utils.RandStringBytes(19)
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		if data.GetMessage().GetIsMessage() {
			var msg = string(data.GetMessage().GetMessage())
			fmt.Println(msg)
			if strings.Contains(msg, "starting mitm server") {
				var packet = `GET http://` + utils.HostPort(mockHost, mockPort) + `/mh/zwww/hlwjjg/index.jsp?a=` + token + ` HTTP/1.1
Host: ` + utils.HostPort(mockHost, mockPort) + `
Proxy-Connection: keep-alive
Upgrade-Insecure-Requests: 1
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7
Referer: http://` + utils.HostPort(mockHost, mockPort) + `/mh/index.jsp?ticket=ST-116464-K9vybT12B8ngtOdc5vYmTj0Cie0-host-10-18-127-7
Accept-Encoding: gzip, deflate
Accept-Language: zh-CN,zh;q=0.9
Cookie: name=value; JSESSIONID=ChIBvh-RZPqDGFAv9DOn-0BVpVvzy73DGnYA
Connection: close

`
				packetBytes := lowhttp.FixHTTPRequest([]byte(packet))
				fmt.Println(string(packetBytes))
				_, err := yak.Execute(`
host, port = str.ParseStringToHostPort(target)~
conn = tcp.Connect(host, port)~
conn.Write(packet)
sleep(0.5)
conn.Close()
`, map[string]any{
					"packet": string(packetBytes),
					"target": utils.HostPort("127.0.0.1", mitmPort),
				})
				if err != nil {
					t.Fatal(err)
				}
				cancel()
			}
		}
	}

	data, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{Keyword: token})
	if err != nil {
		t.Fatal(err)
	}
	request := string(data.GetData()[0].Request)
	if utils.MatchAnyOfSubString(request, "Proxy-Connection: ", "GET http://", "GET https://") {
		t.Fatal("request should not contains proxy connection")
	}
}

func TemplateTestGRPCMUSTPASS_MITM_PlainProxy(t *testing.T) {
	var ctx, cancel = context.WithCancel(utils.TimeoutContextSeconds(5))
	defer cancel()

	mockHost, mockPort := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("hello"))
	})

	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	stream, err := client.MITM(ctx)
	if err != nil {
		panic(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		if data.GetMessage().GetIsMessage() {
			var msg = string(data.GetMessage().GetMessage())
			fmt.Println(msg)
			if strings.Contains(msg, "starting mitm server") {
				var packet = `GET http://` + utils.HostPort(mockHost, mockPort) + `/mh/zwww/hlwjjg/index.jsp HTTP/1.1
Host: ` + utils.HostPort(mockHost, mockPort) + `
Proxy-Connection: keep-alive
Upgrade-Insecure-Requests: 1
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7
Referer: http://` + utils.HostPort(mockHost, mockPort) + `/mh/index.jsp?ticket=ST-116464-K9vybT12B8ngtOdc5vYmTj0Cie0-host-10-18-127-7
Accept-Encoding: gzip, deflate
Accept-Language: zh-CN,zh;q=0.9
Cookie: name=value; JSESSIONID=ChIBvh-RZPqDGFAv9DOn-0BVpVvzy73DGnYA
Connection: close

`
				packetBytes := lowhttp.FixHTTPRequest([]byte(packet))
				println(string(packetBytes))
				cancel()
			}
		}
	}
}
