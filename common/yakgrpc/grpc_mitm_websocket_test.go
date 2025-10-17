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
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_MITM_WebSocket(t *testing.T) {
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(20))
	defer cancel()
	token := utils.RandStringBytes(60)
	token2 := utils.RandStringBytes(60)

	host, port := utils.DebugMockEchoWs("enPayload")
	log.Infof("addr: %s:%d", host, port)
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	mitmPort := utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
	count := 0

	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})

	for {

		rpcResponse, err := stream.Recv()
		if err != nil {
			break
		}

		if msg := rpcResponse.GetMessage(); msg != nil && len(msg.GetMessage()) > 0 {
			if !strings.Contains(string(msg.GetMessage()), `MITM 服务器已启动`) {
				continue
			}

			defer GetMITMFilterManager(consts.GetGormProjectDatabase(), consts.GetGormProfileDatabase()).Recover()
			wsClient, err := lowhttp.NewWebsocketClient([]byte(fmt.Sprintf(`GET /enPayload?token=%s HTTP/1.1
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
`, token2, utils.HostPort(host, port))), lowhttp.WithWebsocketProxy(proxy), lowhttp.WithWebsocketFromServerHandler(func(bytes []byte) {
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
			wsClient.Start()
			for i := 0; i < 3; i++ {
				err = wsClient.WriteText([]byte(token))
				log.Infof("client send: %s", token)
				if err != nil {
					t.Fatalf("send websocket request err: %v", err)
				}
			}
			defer wsClient.WriteClose()
		}
	}
	rsp, err := QueryHTTPFlows(utils.TimeoutContextSeconds(2), client, &ypb.QueryHTTPFlowRequest{
		Keyword: token2,
	}, 1)
	require.NoError(t, err)
	flow := rsp.Data[0]
	require.True(t, flow.IsWebsocket, "flow is not websocket")
	hash := flow.WebsocketHash

	var wsFlows []*schema.WebsocketFlow
	err = utils.AttemptWithDelayFast(func() error {
		_, wsFlows, err = yakit.QueryWebsocketFlowByWebsocketHash(consts.GetGormProjectDatabase(), hash, 1, 10)
		if len(wsFlows) != 6 {
			return utils.Errorf("len(wsFlows) != 6, got %d", len(wsFlows))
		}
		return err
	})

	require.NoError(t, err)
	require.Len(t, wsFlows, 6, "len(wsFlows) != 6")

	require.Equal(t, 3, count, "TestGRPCMUSTPASS_MITM_WebSocket count(%d) != 3")
}

func TestGRPCMUSTPASS_MITM_WebSocket_EmptyRequestOrResponse(t *testing.T) {
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(10))
	defer cancel()

	token := utils.RandStringBytes(60)

	host, port := utils.DebugMockEchoWs("test_empty")
	log.Infof("addr: %s:%d", host, port)

	err := utils.WaitConnect(utils.HostPort(host, port), 5)
	if err != nil {
		t.Fatal(err)
	}

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	mitmPort := utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)

	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})

	for {

		rpcResponse, err := stream.Recv()
		if err != nil {
			break
		}

		if msg := rpcResponse.GetMessage(); msg != nil && len(msg.GetMessage()) > 0 {
			if !strings.Contains(string(msg.GetMessage()), `MITM 服务器已启动`) {
				continue
			}

			defer GetMITMFilterManager(consts.GetGormProjectDatabase(), consts.GetGormProfileDatabase()).Recover()

			wsClient, err := lowhttp.NewWebsocketClient([]byte(fmt.Sprintf(`GET /test_empty?token=%s HTTP/1.1
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
`, token, utils.HostPort(host, port))), lowhttp.WithWebsocketProxy(proxy))
			require.NoError(t, err)
			wsClient.Close()
			cancel()
		}
	}

	_, err = QueryHTTPFlows(utils.TimeoutContextSeconds(5), client, &ypb.QueryHTTPFlowRequest{Keyword: token}, 1)
	require.NoError(t, err)
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
				wsClient.Start()
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

func TestGRPCMUSTPASS_MITM_WebSocket_RULE(t *testing.T) {
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(20))
	defer cancel()
	token := utils.RandStringBytes(60)
	token2 := utils.RandStringBytes(60)
	tagToken := utils.RandStringBytes(10)

	host, port := utils.DebugMockEchoWs("ruleCheck")
	log.Infof("addr: %s:%d", host, port)
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	mitmPort := utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
	RunMITMTestServerEx(client, ctx, func(stream ypb.Yak_MITMClient) {
		stream.Send(&ypb.MITMRequest{
			Host:        "127.0.0.1",
			Port:        uint32(mitmPort),
			EnableGMTLS: true,
			PreferGMTLS: true,
		})
	}, func(stream ypb.Yak_MITMClient) {
		stream.Send(&ypb.MITMRequest{
			SetContentReplacers: true,
			Replacers: []*ypb.MITMContentReplacer{
				{
					Rule:             token,
					NoReplace:        true,
					Result:           ``,
					Color:            "red",
					EnableForRequest: true,
					EnableForHeader:  true,
					EnableForBody:    true,
					Index:            0,
					ExtraTag:         []string{tagToken},
					Disabled:         false,
					VerboseName:      "",
				},
			},
		})
		time.Sleep(3 * time.Second)
		defer cancel()
		wsClient, err := lowhttp.NewWebsocketClient([]byte(fmt.Sprintf(`GET /ruleCheck?token=%s HTTP/1.1
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
`, token2, utils.HostPort(host, port))), lowhttp.WithWebsocketProxy(proxy), lowhttp.WithWebsocketFromServerHandler(func(bytes []byte) {
			if string(bytes) == "server: "+token {
				log.Infof("client recv: %s", bytes)
				cancel()
			}
		}))
		if err != nil {
			t.Fatalf("send websocket request err: %v", err)
		}
		wsClient.Start()
		err = wsClient.WriteText([]byte(token))
		log.Infof("client send: %s", token)
		if err != nil {
			t.Fatalf("send websocket request err: %v", err)
		}
		defer wsClient.WriteClose()
	}, func(stream ypb.Yak_MITMClient, msg *ypb.MITMResponse) {

	})

	rsp, err := QueryHTTPFlows(utils.TimeoutContextSeconds(2), client, &ypb.QueryHTTPFlowRequest{
		Keyword: token2,
	}, 1)
	require.NoError(t, err)
	flow := rsp.Data[0]
	require.True(t, flow.IsWebsocket, "flow is not websocket")
	hash := flow.WebsocketHash

	var wsFlows []*schema.WebsocketFlow
	err = utils.AttemptWithDelayFast(func() error {
		_, wsFlows, err = yakit.QueryWebsocketFlowByWebsocketHash(consts.GetGormProjectDatabase(), hash, 1, 10)
		if len(wsFlows) != 2 {
			return utils.Errorf("len(wsFlows) != 6, got %d", len(wsFlows))
		}
		return err
	})

	require.NoError(t, err)
	require.Len(t, wsFlows, 2, "len(wsFlows) != 2")
	require.Contains(t, wsFlows[0].Tags, tagToken, "wsFlows[0].Tags not contains tagToken")
	require.Contains(t, wsFlows[1].Tags, tagToken, "wsFlows[1].Tags not contains tagToken")
	require.Contains(t, wsFlows[0].Tags, schema.FLOW_COLOR_RED, "wsFlows[0].Tags not contains color tag")
	require.Contains(t, wsFlows[1].Tags, schema.FLOW_COLOR_RED, "wsFlows[1].Tags not contains color tag")
}

func TestGRPCMUSTPASS_MITMV2_WebSocket(t *testing.T) {
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(20))
	defer cancel()
	token := utils.RandStringBytes(60)
	token2 := utils.RandStringBytes(60)

	host, port := utils.DebugMockEchoWss("enPayload2")
	log.Infof("addr: %s:%d", host, port)
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	mitmPort := utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
	count := 0

	stream, err := client.MITMV2(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMV2Request{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})

	for {

		rpcResponse, err := stream.Recv()
		if err != nil {
			break
		}

		if msg := rpcResponse.GetMessage(); msg != nil && len(msg.GetMessage()) > 0 {
			if !strings.Contains(string(msg.GetMessage()), `MITM 服务器已启动`) {
				continue
			}

			defer GetMITMFilterManager(consts.GetGormProjectDatabase(), consts.GetGormProfileDatabase()).Recover()
			wsClient, err := lowhttp.NewWebsocketClient([]byte(fmt.Sprintf(`GET /enPayload2?token=%s HTTP/1.1
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
`, token2, utils.HostPort(host, port))), lowhttp.WithWebsocketProxy(proxy), lowhttp.WithWebsocketTLS(true), lowhttp.WithWebsocketFromServerHandler(func(bytes []byte) {
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
			wsClient.Start()
			for i := 0; i < 3; i++ {
				err = wsClient.WriteText([]byte(token))
				log.Infof("client send: %s", token)
				if err != nil {
					t.Fatalf("send websocket request err: %v", err)
				}
			}
			defer wsClient.WriteClose()
		}
	}
	rsp, err := QueryHTTPFlows(utils.TimeoutContextSeconds(2), client, &ypb.QueryHTTPFlowRequest{
		Keyword: token2,
	}, 1)
	require.NoError(t, err)
	flow := rsp.Data[0]
	require.True(t, flow.IsWebsocket, "flow is not websocket")
	hash := flow.WebsocketHash

	var wsFlows []*schema.WebsocketFlow
	err = utils.AttemptWithDelayFast(func() error {
		_, wsFlows, err = yakit.QueryWebsocketFlowByWebsocketHash(consts.GetGormProjectDatabase(), hash, 1, 10)
		if len(wsFlows) != 6 {
			return utils.Errorf("len(wsFlows) != 6, got %d", len(wsFlows))
		}
		return err
	})

	require.NoError(t, err)
	require.Len(t, wsFlows, 6, "len(wsFlows) != 6")

	require.Equal(t, 3, count, "TestGRPCMUSTPASS_MITM_WebSocket count(%d) != 3")
}
