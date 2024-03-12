package yakgrpc

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
	"time"
)

func TestGRPCMUSTPASS_MITM_WebSocket(t *testing.T) {
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(20))
	defer cancel()
	token := utils.RandNumberStringBytes(20)

	host, port := utils.DebugMockEchoWs([]byte(token))
	log.Infof("addr:  %s:%d", host, port)
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	var mitmPort = utils.GetRandomAvailableTCPPort()
	var proxy = "http://" + utils.HostPort("127.0.0.1", mitmPort)

	RunMITMTestServer(client, ctx, &ypb.MITMRequest{
		Port: uint32(mitmPort),
		Host: "127.0.0.1",
	}, func(mitmClient ypb.Yak_MITMClient) {

		defer NewMITMFilterManager(consts.GetGormProfileDatabase()).Recover()
		wsClient, err := lowhttp.NewWebsocketClient([]byte(fmt.Sprintf(`GET /echo HTTP/1.1
Host: %s
Accept-Encoding: gzip, deflate
Sec-WebSocket-Extensions: permessage-deflate
Sec-WebSocket-Key: 3o0bLKJzcaNwhJQs4wBw2g==
Accept-Language: zh-CN,zh;q=0.8,zh-TW;q=0.7,zh-HK;q=0.5,en-US;q=0.3,en;q=0.2
Cache-Control: no-cache
Pragma: no-cache
Upgrade: websocket
Sec-WebSocket-Version: 13
Connection: keep-alive, Upgrade
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:122.0) Gecko/20100101 Firefox/122.0
Accept: */*
`, utils.HostPort(host, port))), lowhttp.WithWebsocketProxy(proxy))
		if err != nil {
			t.Fatalf("send websocket request err: %v", err)
		}
		for i := 0; i < 3; i++ {
			err = wsClient.Write([]byte(token))
		}
		if err != nil {
			t.Fatalf("send websocket request err: %v", err)
		}
		time.Sleep(1 * time.Second)
		count := yakit.SearchWebsocketFlow("server: " + token)
		fmt.Println(count)
		if count != 3 {
			t.Errorf("search httpflow by token failed: yakit.QuickSearchMITMHTTPFlowCount(token)")
		}
	})
}

func TestGRPCMUSTPASS_MITM_WebSocket_Payload(t *testing.T) {
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(20))
	defer cancel()
	token := utils.RandNumberStringBytes(20)

	host, port := utils.DebugMockEchoWs([]byte(token))

	client, err := NewLocalClient() // 新建一个 yakit client
	if err != nil {
		t.Fatal(err)
	}

	rPort := utils.GetRandomAvailableTCPPort()
	var proxy = "http://" + utils.HostPort("127.0.0.1", rPort)

	// 启动MITM服务器
	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(rPort),
	})

	stream.Send(&ypb.MITMRequest{SetAutoForward: true, AutoForwardValue: false})
	tokenCount := 0
	for {
		rcpResponse, err := stream.Recv()
		if err != nil {
			break
		}
		rspMsg := string(rcpResponse.GetMessage().GetMessage())
		if len(rcpResponse.GetRequest()) > 0 {
			if len(rcpResponse.GetResponse()) > 0 {
				stream.Send(&ypb.MITMRequest{
					Response:   rcpResponse.GetResponse(),
					Id:         rcpResponse.GetId(),
					ResponseId: rcpResponse.GetResponseId(),
				})
			}
			if len(rcpResponse.GetPayload()) > 0 {
				if rcpResponse.GetRequest() == nil {
					t.Fatalf("websocket rcpResponse.GetRequest() is nil")
				}
				if !strings.Contains(string(rcpResponse.GetPayload()), token) {
					t.Fatalf("websocket rcpResponse.GetPayload() not contains token")
				}
				if strings.Contains(string(rcpResponse.GetPayload()), token) {
					tokenCount++
				}
				stream.Send(&ypb.MITMRequest{
					Id:         rcpResponse.GetId(),
					ResponseId: rcpResponse.GetResponseId(),
				})
			}

			stream.Send(&ypb.MITMRequest{
				Id:             rcpResponse.GetId(),
				HijackResponse: true,
			})
			stream.Send(&ypb.MITMRequest{
				Id:      rcpResponse.GetId(),
				Request: rcpResponse.GetRequest(),
			})
		}
		if strings.Contains(rspMsg, `starting mitm serve`) {
			go func() {
				time.Sleep(1 * time.Second)
				wsClient, err := lowhttp.NewWebsocketClient([]byte(fmt.Sprintf(`GET /echo HTTP/1.1
Host: %s
Accept-Encoding: gzip, deflate
Sec-WebSocket-Key: 3o0bLKJzcaNwhJQs4wBw2g==
Accept-Language: zh-CN,zh;q=0.8,zh-TW;q=0.7,zh-HK;q=0.5,en-US;q=0.3,en;q=0.2
Cache-Control: no-cache
Pragma: no-cache
Upgrade: websocket
Sec-WebSocket-Version: 13
Connection: keep-alive, Upgrade
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:122.0) Gecko/20100101 Firefox/122.0
Accept: */*
`, utils.HostPort(host, port))), lowhttp.WithWebsocketProxy(proxy))
				if err != nil {
					t.Fatal(err)
				}
				time.Sleep(1 * time.Second)
				//for i := 0; i < 3; i++ {
				err = wsClient.Write([]byte(token))
				//}
				if err != nil {
					t.Fatal(err)
				}
				time.Sleep(2 * time.Second)
				cancel()
			}()
		}
	}

	if tokenCount != 2 {
		t.Fatalf("tokenCount != 2")
	}
}
