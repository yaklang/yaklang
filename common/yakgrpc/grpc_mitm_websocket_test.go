package yakgrpc

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_MITM_WebSocket(t *testing.T) {
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(20))
	defer cancel()
	token := utils.RandStringBytes(60)

	host, port := utils.DebugMockEchoWs("enPayload")
	log.Infof("addr: %s:%d", host, port)
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	mitmPort := utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
	count := 0
	RunMITMTestServer(client, ctx, &ypb.MITMRequest{
		Port: uint32(mitmPort),
		Host: "127.0.0.1",
	}, func(mitmClient ypb.Yak_MITMClient) {
		defer NewMITMFilterManager(consts.GetGormProfileDatabase()).Recover()
		wsClient, err := lowhttp.NewWebsocketClient([]byte(fmt.Sprintf(`GET /enPayload HTTP/1.1
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
`, utils.HostPort(host, port))), lowhttp.WithWebsocketProxy(proxy), lowhttp.WithWebsocketFromServerHandler(func(bytes []byte) {
			if string(bytes) == "server: "+token {
				log.Infof("client recv: %s", bytes)
				count++
			}
			if count == 3 {
				cancel()
			}
		}))
		if err != nil {
			t.Fatalf("send websocket request err: %v", err)
		}
		wsClient.StartFromServer()
		for i := 0; i < 3; i++ {
			err = wsClient.WriteText([]byte(token))
			log.Infof("client send: %s", token)
			if err != nil {
				t.Fatalf("send websocket request err: %v", err)
			}
		}
		defer wsClient.WriteClose()
	})

	if count != 3 {
		t.Fatalf("TestGRPCMUSTPASS_MITM_WebSocket count(%d) != 3", count)
	}
}

func TestGRPCMUSTPASS_MITM_WebSocket_Payload(t *testing.T) {
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(20))
	defer cancel()
	token := utils.RandStringBytes(60)

	host, port := utils.DebugMockEchoWs("payload")

	client, err := NewLocalClient() // 新建一个 yakit client
	if err != nil {
		t.Fatal(err)
	}

	rPort := utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort("127.0.0.1", rPort)

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
	hijackClientPayload := false
	hijackServerPayload := false
	state := 0
	for {
		rpcResponse, err := stream.Recv()
		if err != nil {
			break
		}
		rspMsg := string(rpcResponse.GetMessage().GetMessage())
		if len(rpcResponse.GetRequest()) > 0 {
			switch state {
			case 0:
				// hijack http response
				stream.Send(&ypb.MITMRequest{
					Id:             rpcResponse.GetId(),
					HijackResponse: true,
				})
				// forward http request
				stream.Send(&ypb.MITMRequest{
					Id:      rpcResponse.GetId(),
					Request: rpcResponse.GetRequest(),
				})
				state++
			case 1:
				// skip other request, like JustContentReplacer
				if len(rpcResponse.GetResponse()) == 0 {
					continue
				}

				// forward http response
				stream.Send(&ypb.MITMRequest{
					Id:         rpcResponse.GetId(),
					ResponseId: rpcResponse.GetResponseId(),
					Response:   rpcResponse.GetResponse(),
				})
				state++
			case 2:
				payload := rpcResponse.GetPayload()
				require.Greater(t, len(payload), 0, "payload is empty")
				require.NotNil(t, rpcResponse.GetRequest(), "rcpResponse.GetRequest() is nil")

				log.Infof("recv payload: %s", payload)
				if string(payload) == token {
					hijackClientPayload = true
				} else if string(payload) == "server: "+token {
					hijackServerPayload = true
				}

				// forward payload
				stream.Send(&ypb.MITMRequest{
					Id:         rpcResponse.GetId(),
					ResponseId: rpcResponse.GetResponseId(),
					Request:    payload,
					Response:   payload,
				})
			}
		}
		if strings.Contains(rspMsg, `starting mitm serve`) {
			go func() {
				time.Sleep(1 * time.Second)
				wsClient, err := lowhttp.NewWebsocketClient([]byte(fmt.Sprintf(`GET /payload HTTP/1.1
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
`, utils.HostPort(host, port))), lowhttp.WithWebsocketProxy(proxy), lowhttp.WithWebsocketFromServerHandler(func(bytes []byte) {
					if string(bytes) == "server: "+token {
						cancel()
					}
				}))

				require.NoError(t, err)
				wsClient.StartFromServer()
				err = wsClient.Write([]byte(token))
				require.NoError(t, err)
				defer wsClient.WriteClose()
			}()
		}
	}

	if !hijackClientPayload || !hijackServerPayload {
		t.Fatalf("TestGRPCMUSTPASS_MITM_WebSocket_Payload hijackClientPayload(%v) hijackServerPayload(%v)", hijackClientPayload, hijackServerPayload)
	}
}
