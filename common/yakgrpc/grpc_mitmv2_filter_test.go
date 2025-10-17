package yakgrpc

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"golang.org/x/net/websocket"
)

func RunMITMV2TestServer(
	client ypb.YakClient,
	ctx context.Context,
	req *ypb.MITMV2Request,
	onLoad func(mitmClient ypb.Yak_MITMV2Client),
) (host, port string) {
	return RunMITMV2TestServerEx(client, ctx, func(mitmClient ypb.Yak_MITMV2Client) {
		mitmClient.Send(req)
	}, onLoad, nil)
}

func RunMITMV2TestServerEx(
	client ypb.YakClient,
	ctx context.Context,
	onInit func(mitmClient ypb.Yak_MITMV2Client),
	onLoad func(mitmClient ypb.Yak_MITMV2Client),
	onRecv func(mitmClient ypb.Yak_MITMV2Client, msg *ypb.MITMV2Response),
) (host, port string) {
	stream, err := client.MITMV2(ctx)
	if err != nil {
		panic(err)
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	if onInit != nil {
		onInit(stream)
	}
	for {
		msg, err := stream.Recv()
		if err != nil {
			break
		}
		if msg.GetHaveMessage() {
			msgStr := string(msg.GetMessage().GetMessage())
			if strings.Contains(msgStr, `starting mitm serve`) {
				if onLoad != nil {
					go func() {
						defer wg.Done()
						onLoad(stream)
					}()
				}
			}
		}
		if onRecv != nil {
			onRecv(stream, msg)
		}
	}
	wg.Wait()
	return
}

func TestGRPCMUSTPASS_MITMV2_ForExcludeBadCase(t *testing.T) {
	_, mockPort := utils.DebugMockHTTPEx(func(req []byte) []byte {
		ct := lowhttp.GetHTTPRequestQueryParam(req, "ct")
		rsp, _, _ := lowhttp.FixHTTPResponse([]byte("HTTP/1.1 200 OK\r\nD: 1\r\n\r\n" + time.Now().String()))
		if ct != "" {
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Content-Type", ct)
			rsp = lowhttp.ReplaceHTTPPacketBodyFast(rsp, []byte("abc"))
		}
		return rsp
	})

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	mitmPort := utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	RunMITMV2TestServer(client, ctx, &ypb.MITMV2Request{
		Port: uint32(mitmPort),
		Host: "127.0.0.1",
	}, func(mitmClient ypb.Yak_MITMV2Client) {
		var token string
		var packet []byte

		mitmClient.Send(&ypb.MITMV2Request{
			FilterData: &ypb.MITMFilterData{
				ExcludeSuffix: []*ypb.FilterDataItem{
					{
						MatcherType: "suffix",
						Group:       []string{".gif"},
					},
				},
			},
			UpdateFilter: true,
		})
		defer GetMITMFilterManager(consts.GetGormProjectDatabase(), consts.GetGormProfileDatabase()).Recover()
		time.Sleep(500 * time.Millisecond)
		for _, ct := range [][]any{
			{"/abc.a", 0},
		} {
			// path := utils.InterfaceToString(ct[0])
			expectCount := codec.Atoi(utils.InterfaceToString(ct[1]))
			token = uuid.NewString()
			packet = []byte(`GET /-L-` + token + `/v.gif?logactid=1234567890&showTab=10000&opType=showpv&mod=superman%3Alib&submod=index&superver=supernewplus&glogid=2147883968&type=2011&pid=315&isLogin=0&version=PCHome&terminal=PC&qid=0xc349374900061bc0&sid=36551_38642_38831_39027_39022_38958_38955_39014_39038_38811_39084_38639_26350_39095_39100&super_frm=&from_login=&from_reg=&query=&curcard=2&curcardtab=&_r=0.9024198609355389 HTTP/1.1
Host: sp1.baidu.com
Accept: image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8
Accept-Encoding: gzip, deflate, br
Accept-Language: zh-CN,zh;q=0.9
Cookie: BIDUPSID=1B2FE3FEA32C14877E77E27E1D768790; PSTM=1689326364; BAIDUID=1B2FE3FEA32C1487604415A535F8EF61:FG=1; BDORZ=B490B5EBF6F3CD402E515D22BCDA1598; BAIDUID_BFESS=1B2FE3FEA32C1487604415A535F8EF61:FG=1; BA_HECTOR=0gag2l8k250kag25a58h2l2c1ib9jdk1p; ZFY=kUW6pGiefcPpyX9xXHyZciUqxlwzGV4vQLsYNl4qb:BU:C; ab_sr=1.0.1_OWUxNjI4NjRmODZjNDYzY2RjY2NmOGQ0ZTlkM2E5Y2I5MTJiYjYxMjMyNGU0YjhiODEwMzllMTljMTU0OTJiOThhODc3MjRjYzQxYzhlNjk0MzM1YjM1OWI4YzJmMTlmNjhhYjE5N2RlODI5ZjRiMmU3MjdlMWRiYzVkMDUxMjNmMzFlMjA2ZGMzNDI2OTRiYWNmNThkMjAzMjI1OWY5Mg==; H_PS_PSSID=36561_38642_38831_39027_39022_38942_38957_38956_39009_38961_38972_38802_38826_38986_39087_38637_26350_39042_39095_39100_39043
Referer: https://www.baidu.com/
Sec-Fetch-Dest: image
Sec-Fetch-Mode: no-cors
Sec-Fetch-Site: same-site
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36
sec-ch-ua: "Not.A/Brand";v="8", "Chromium";v="114", "Google Chrome";v="114"
sec-ch-ua-mobile: ?0
sec-ch-ua-platform: "macOS"

`)
			params := map[string]any{"proxy": proxy, "mockHost": "127.0.0.1", "mockPort": mockPort, "token": token}
			params["packet"] = packet
			_, err = yak.Execute(`
rsp, _ = poc.HTTP(packet, poc.proxy(proxy), poc.host(mockHost), poc.port(mockPort))~
sleep(0.3)
`, params)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			count := yakit.QuickSearchMITMHTTPFlowCount(token)
			log.Infof("yakit.QuickSearchMITMHTTPFlowCount("+`[`+token+`]`+") == %v", count)
			if count != expectCount {
				cancel()
				t.Fatal("search httpflow by token failed: yakit.QuickSearchMITMHTTPFlowCount(token)")
			}
		}
		cancel()
	})
}

func TestGRPCMUSTPASS_MITMV2_Filter_ForExcludeURI(t *testing.T) {
	_, mockPort := utils.DebugMockHTTPEx(func(req []byte) []byte {
		ct := lowhttp.GetHTTPRequestQueryParam(req, "ct")
		rsp, _, _ := lowhttp.FixHTTPResponse([]byte("HTTP/1.1 200 OK\r\nD: 1\r\n\r\n" + time.Now().String()))
		if ct != "" {
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Content-Type", ct)
			rsp = lowhttp.ReplaceHTTPPacketBodyFast(rsp, []byte("abc"))
		}
		return rsp
	})

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	mitmPort := utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	RunMITMV2TestServer(client, ctx, &ypb.MITMV2Request{
		Port: uint32(mitmPort),
		Host: "127.0.0.1",
	}, func(mitmClient ypb.Yak_MITMV2Client) {
		var token string
		var packet []byte

		FilterData := &ypb.MITMFilterData{
			ExcludeUri: []*ypb.FilterDataItem{
				{
					MatcherType: "word",
					Group:       []string{"abc"},
				},
			},
			ExcludeMethods: []*ypb.FilterDataItem{
				{
					MatcherType: "glob",
					Group:       []string{"NONONO"},
				},
			},
		}
		mitmClient.Send(&ypb.MITMV2Request{
			FilterData:   FilterData,
			UpdateFilter: true,
		})
		defer GetMITMFilterManager(consts.GetGormProjectDatabase(), consts.GetGormProfileDatabase()).Recover()
		time.Sleep(500 * time.Millisecond)
		for _, ct := range [][]any{
			{"/abc.a", 0},
			{"/a/abc.js", 0},
			{"/abc.aaac", 0},
			{"/a1bc.aaac", 1},
			{"/a1bc.aaac?abc=1", 0},
			{"/a1bc.aaac?a222bc=1", 1},
			{"/a1bc.aaac?a222bc=1&a=abc", 0},
			{"/a1bc.aaac?a222bc=1&a=abcc", 0},
		} {
			path := utils.InterfaceToString(ct[0])
			expectCount := codec.Atoi(utils.InterfaceToString(ct[1]))
			token = ksuid.New().String()
			packet = []byte("GET " + path + " HTTP/1.1\r\nHost: " + utils.HostPort("127.0.0.1", mockPort))
			params := map[string]any{"proxy": proxy, "mockHost": "127.0.0.1", "mockPort": mockPort, "token": token}
			packet = lowhttp.ReplaceHTTPPacketHeader(packet, "X-TOKEN", token)
			params["packet"] = packet
			_, err = yak.Execute(`
println(string(packet))
rsp, _ = poc.HTTP(packet, poc.proxy(proxy), poc.host(mockHost), poc.port(mockPort))~
println(string(rsp))
sleep(0.3)
`, params)
			if err != nil {
				t.Logf("err: %v", err)
				t.Fail()
			}
			count := yakit.QuickSearchMITMHTTPFlowCount(token)
			log.Infof("yakit.QuickSearchMITMHTTPFlowCount("+`[`+token+`]`+") == %v", count)
			fmt.Println("checking path : " + path)
			require.Equal(t, expectCount, count)
		}
		cancel()
	})
}

func TestGRPCMUSTPASS_MITMV2_Filter_ForExcludeSuffixAndContentType(t *testing.T) {
	_, mockPort := utils.DebugMockHTTPEx(func(req []byte) []byte {
		ct := lowhttp.GetHTTPRequestQueryParam(req, "ct")
		rsp, _, _ := lowhttp.FixHTTPResponse([]byte("HTTP/1.1 200 OK\r\nD: 1\r\n\r\n" + time.Now().String()))
		if ct != "" {
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Content-Type", ct)
			rsp = lowhttp.ReplaceHTTPPacketBodyFast(rsp, []byte("abc"))
		}
		return rsp
	})

	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	mitmPort := utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	RunMITMV2TestServer(client, ctx, &ypb.MITMV2Request{
		Port: uint32(mitmPort),
		Host: "127.0.0.1",
	}, func(mitmClient ypb.Yak_MITMV2Client) {
		var token string
		var packet []byte

		mitmClient.Send(&ypb.MITMV2Request{
			FilterData: &ypb.MITMFilterData{
				ExcludeSuffix: []*ypb.FilterDataItem{
					{
						MatcherType: "suffix",
						Group:       []string{".aaac", ".zip", ".js"},
					},
				},
			},
			UpdateFilter: true,
		})
		defer GetMITMFilterManager(consts.GetGormProjectDatabase(), consts.GetGormProfileDatabase()).Recover()
		time.Sleep(500 * time.Millisecond)
		for _, ct := range [][]any{
			{"/abc.png.zip?ab=1", 0},
			{"/abc.a", 1},
			{"/static/abc.ppt", 1},
			{"/abc.aaac", 0},
			{"/abc.jpg", 1},
			{"/abc.png.zip", 0},
			{"/static/abc.js", 0},
			{"/abc.ajs", 1},
			{"/abc.json", 1},
		} {
			path := utils.InterfaceToString(ct[0])
			expectCount := codec.Atoi(utils.InterfaceToString(ct[1]))
			token = ksuid.New().String()
			packet = []byte("GET " + path + " HTTP/1.1\r\nHost: " + utils.HostPort("127.0.0.1", mockPort))
			params := map[string]any{"proxy": proxy, "mockHost": "127.0.0.1", "mockPort": mockPort, "token": token}
			packet = lowhttp.ReplaceHTTPPacketHeader(packet, "X-TOKEN", token)
			params["packet"] = packet
			_, err = yak.Execute(`
rsp, _ = poc.HTTP(packet, poc.proxy(proxy), poc.host(mockHost), poc.port(mockPort))~
sleep(0.3)
`, params)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			count := yakit.QuickSearchMITMHTTPFlowCount(token)
			log.Infof("yakit.QuickSearchMITMHTTPFlowCount("+`[`+token+`]`+") == %v", count)
			if count != expectCount {
				t.Fatalf("exclude suffix [.aaac, .zip, .js] failed, [%s] except %d but got %d", path, expectCount, count)
				cancel()
			}
		}

		FilterData := &ypb.MITMFilterData{
			ExcludeSuffix: []*ypb.FilterDataItem{
				{
					MatcherType: "suffix",
					Group:       []string{".aaac"},
				},
			},
			ExcludeMethods: []*ypb.FilterDataItem{
				{
					MatcherType: "glob",
					Group:       []string{"NONONO"},
				},
			},
			ExcludeMIME: []*ypb.FilterDataItem{
				{
					MatcherType: "mime",
					Group:       []string{"bbbbbb", "*cc", "*oct", "abc", "text"},
				},
			},
		}

		mitmClient.Send(&ypb.MITMV2Request{
			FilterData:   FilterData,
			UpdateFilter: true,
		})
		defer GetMITMFilterManager(consts.GetGormProjectDatabase(), consts.GetGormProfileDatabase()).Recover()

		time.Sleep(500 * time.Millisecond)
		for _, ct := range [][]any{
			{"application/abc", 0},
			{"abc1111", 0},
			{"application/oct", 0},
			{"application/zip", 1},
			{"bbbbbb", 0},
			{"aabb", 1},
			{"cccc", 0},
			{"ccc", 0},
			{"cc", 0},
			{"text/plain", 0},     // text 命中 前半部分
			{"textplain/test", 1}, // text 无法命中
			{"textplain/text", 0}, // text 命中 后半部分
		} {
			path := "/"
			contentType := utils.InterfaceToString(ct[0])
			expectCount := codec.Atoi(utils.InterfaceToString(ct[1]))
			token = ksuid.New().String()
			packet = []byte("GET " + path + "?ct=" + codec.QueryEscape(contentType) + " HTTP/1.1\r\nHost: " + utils.HostPort("127.0.0.1", mockPort))
			params := map[string]any{"proxy": proxy, "mockHost": "127.0.0.1", "mockPort": mockPort, "token": token}
			packet = lowhttp.ReplaceHTTPPacketHeader(packet, "X-TOKEN", token)
			params["packet"] = packet
			_, err = yak.Execute(`
rsp, _ = poc.HTTP(packet, poc.proxy(proxy), poc.host(mockHost), poc.port(mockPort))~
sleep(0.5)
`, params)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			count := yakit.QuickSearchMITMHTTPFlowCount(token)
			log.Infof("yakit.QuickSearchMITMHTTPFlowCount("+`[`+token+`]`+") == %v", count)
			if count != expectCount {
				t.Fatalf("search httpflow by token failed: yakit.QuickSearchMITMHTTPFlowCount(token) mimetype:[%v]", ct[0])
				cancel()
			}
		}
		cancel()
	})
}

func TestGRPCMUSTPASS_MITMV2_WebSockett_Filter_RSP(t *testing.T) {
	token := utils.RandStringBytes(10)
	sendCompleteCh := make(chan struct{})
	host, port := utils.DebugMockWs(func(ws *websocket.Conn) {
		for i := 0; i < 10; i++ {
			ws.Write([]byte(token))
			time.Sleep(50 * time.Millisecond)
		}
		close(sendCompleteCh)
	})

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	mitmPort := utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	RunMITMV2TestServer(client, ctx, &ypb.MITMV2Request{
		Port: uint32(mitmPort),
		Host: "127.0.0.1",
	}, func(mitmClient ypb.Yak_MITMV2Client) {
		mitmClient.Send(&ypb.MITMV2Request{
			FilterWebsocket:       true,
			UpdateFilterWebsocket: true,
		})
		defer GetMITMFilterManager(consts.GetGormProjectDatabase(), consts.GetGormProfileDatabase()).Recover()
		_, err = lowhttp.NewWebsocketClient([]byte(fmt.Sprintf(`GET /?%s HTTP/1.1
Host: %s
Connection: Upgrade
Upgrade: websocket
Sec-WebSocket-Version: 13
Sec-WebSocket-Key: w4v7O6xFTi36lq3RNcgctw==
`, token, utils.HostPort(host, port))), lowhttp.WithWebsocketProxy(proxy))
		if err != nil {
			t.Fatalf("send websocket request err: %v", err)
		}
		select {
		case <-sendCompleteCh:
		}
		count := yakit.SearchWebsocketFlow(token)
		// fmt.Println(count)
		if count != 0 {
			cancel()
			t.Fatalf("search httpflow by token failed: yakit.QuickSearchMITMHTTPFlowCount(token)")
			t.FailNow()
		}
		cancel()
	})
}

func TestGRPCMUSTPASS_MITMV2_WebSockett_Filter_REQ(t *testing.T) {
	token := utils.RandStringBytes(10)
	sendOKCh := make(chan struct{})
	host, port := utils.DebugMockWs(func(ws *websocket.Conn) {
		var res []byte
		for i := 0; i < 10; i++ {
			ws.Read(res)
			time.Sleep(50 * time.Millisecond)
		}
		close(sendOKCh)
	})

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	mitmPort := utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	RunMITMV2TestServer(client, ctx, &ypb.MITMV2Request{
		Port: uint32(mitmPort),
		Host: "127.0.0.1",
	}, func(mitmClient ypb.Yak_MITMV2Client) {
		mitmClient.Send(&ypb.MITMV2Request{
			FilterWebsocket:       true,
			UpdateFilterWebsocket: true,
		})
		defer GetMITMFilterManager(consts.GetGormProjectDatabase(), consts.GetGormProfileDatabase()).Recover()
		wsClient, err := lowhttp.NewWebsocketClient([]byte(fmt.Sprintf(`GET /?%s HTTP/1.1
Host: %s
Connection: Upgrade
Upgrade: websocket
Sec-WebSocket-Version: 13
Sec-WebSocket-Key: w4v7O6xFTi36lq3RNcgctw==
`, token, utils.HostPort(host, port))), lowhttp.WithWebsocketProxy(proxy))
		if err != nil {
			t.Fatalf("send websocket request err: %v", err)
		}
		for i := 0; i < 10; i++ {
			wsClient.Write([]byte(token))
		}
		select {
		case <-sendOKCh:
		}
		count := yakit.SearchWebsocketFlow(token)
		fmt.Println(count)
		if count != 0 {
			cancel()
			t.Errorf("search httpflow by token failed: yakit.QuickSearchMITMHTTPFlowCount(token)")
		}
		cancel()
	})
}

func TestGRPCMUSTPASS_MITMV2_Filter_Plugin(t *testing.T) {
	var shouldFilter bool
	var notFilter bool

	shouldFilterToken := utils.RandStringBytes(10)
	notFilterToken := utils.RandStringBytes(10)
	_, mockPort := utils.DebugMockHTTPEx(func(req []byte) []byte {
		token := lowhttp.GetHTTPRequestQueryParam(req, "token")
		if token == shouldFilterToken {
			shouldFilter = true
		}
		if token == notFilterToken {
			notFilter = true
		}
		return []byte("HTTP/1.1 200 OK\r\nContent-length: 0\r\n\r\n")
	})
	packet := []byte(fmt.Sprintf(`GET /abc HTTP/1.1
Host: 127.0.0.1:%d`, mockPort))

	code := fmt.Sprintf(`
mirrorFilteredHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
poc.HTTP(req,poc.replaceQueryParam("token", "%s"))
}

hijackSaveHTTPFlow = func(flow /* *yakit.HTTPFlow */, modify /* func(modified *yakit.HTTPFlow) */, drop/* func() */) {
  poc.HTTP(flow.Request,poc.replaceQueryParam("token", "%s")) 
}

hijackHTTPRequest = func(isHttps, url, req, forward /*func(modifiedRequest []byte)*/, drop /*func()*/) {
poc.HTTP(req,poc.replaceQueryParam("token", "%s"))
}

beforeRequest = func(ishttps, oreq/*原始请求*/, req/*hijack修改后的请求*/){
	poc.HTTP(req,poc.replaceQueryParam("token", "%s"))
}

afterRequest = func(ishttps, oreq/*原始请求*/ ,req/*hiajck修改之后的请求*/ ,orsp/*原始响应*/ ,rsp/*hijack修改后的响应*/){
	poc.HTTP(req,poc.replaceQueryParam("token", "%s"))	
}

mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
   poc.HTTP(req,poc.replaceQueryParam("token", "%s"))	 
}
`, shouldFilterToken, shouldFilterToken, shouldFilterToken, shouldFilterToken, shouldFilterToken, notFilterToken)
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	mitmPort := utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	RunMITMV2TestServer(client, ctx, &ypb.MITMV2Request{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	}, func(stream ypb.Yak_MITMV2Client) {
		stream.Send(&ypb.MITMV2Request{
			SetYakScript:     true,
			YakScriptContent: code,
		})
		stream.Recv()
		stream.Send(&ypb.MITMV2Request{
			UpdateFilter: true,
			FilterData: &ypb.MITMFilterData{
				ExcludeUri: []*ypb.FilterDataItem{{MatcherType: "word", Group: []string{"abc"}}},
			},
		})
		stream.Recv()
		defer GetMITMFilterManager(consts.GetGormProjectDatabase(), consts.GetGormProfileDatabase()).Recover()
		_, err := lowhttp.HTTPWithoutRedirect(lowhttp.WithPacketBytes(packet), lowhttp.WithProxy(proxy))
		require.NoError(t, err)
		time.Sleep(3 * time.Second)
		cancel()
	})
	require.False(t, shouldFilter)
	require.True(t, notFilter)
}

func TestGRPCMUSTPASS_MITMV2_Static_Filter(t *testing.T) {
	shouldFilterToken := utils.RandStringBytes(10)
	notFilterToken := utils.RandStringBytes(10)
	notFilterToken2 := utils.RandStringBytes(10)

	t.Run("url", func(t *testing.T) {
		var shouldFilter bool
		var notFilter bool

		_, mockPort := utils.DebugMockHTTPEx(func(req []byte) []byte {
			token := lowhttp.GetHTTPRequestQueryParam(req, "token")
			if token == shouldFilterToken {
				shouldFilter = true
			}
			if token == notFilterToken {
				notFilter = true
			}
			return []byte("HTTP/1.1 200 OK\r\nContent-length: 0\r\n\r\n")
		})
		packet := []byte(fmt.Sprintf(`GET /abc.js HTTP/1.1
Host: 127.0.0.1:%d`, mockPort))

		code := fmt.Sprintf(`
mirrorFilteredHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
	poc.HTTP(req,poc.replaceQueryParam("token", "%s"))
}

mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
   poc.HTTP(req,poc.replaceQueryParam("token", "%s"))	 
}
`, shouldFilterToken, notFilterToken)
		client, err := NewLocalClient()
		if err != nil {
			panic(err)
		}
		mitmPort := utils.GetRandomAvailableTCPPort()
		proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()

		RunMITMV2TestServer(client, ctx, &ypb.MITMV2Request{
			Host: "127.0.0.1",
			Port: uint32(mitmPort),
		}, func(stream ypb.Yak_MITMV2Client) {
			stream.Send(&ypb.MITMV2Request{
				SetYakScript:     true,
				YakScriptContent: code,
			})
			stream.Recv()
			stream.Send(&ypb.MITMV2Request{
				UpdateFilter: true,
				FilterData:   &ypb.MITMFilterData{},
			})
			stream.Recv()
			defer GetMITMFilterManager(consts.GetGormProjectDatabase(), consts.GetGormProfileDatabase()).Recover()
			_, err := lowhttp.HTTPWithoutRedirect(lowhttp.WithPacketBytes(packet), lowhttp.WithProxy(proxy))
			require.NoError(t, err)
			time.Sleep(3 * time.Second)
			cancel()
		})
		require.True(t, notFilter)
		require.False(t, shouldFilter)
	})

	t.Run("url extend", func(t *testing.T) {
		var notFilter, notFilter2 bool

		_, mockPort := utils.DebugMockHTTPEx(func(req []byte) []byte {
			token := lowhttp.GetHTTPRequestQueryParam(req, "token")
			if token == notFilterToken {
				notFilter = true
			}
			if token == notFilterToken2 {
				notFilter2 = true
			}
			return []byte("HTTP/1.1 200 OK\r\nContent-length: 0\r\n\r\n")
		})
		packet := []byte(fmt.Sprintf(`GET /abc.jsp HTTP/1.1
Host: 127.0.0.1:%d`, mockPort))

		code := fmt.Sprintf(`
mirrorFilteredHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
	poc.HTTP(req,poc.replaceQueryParam("token", "%s"))
}

mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
   poc.HTTP(req,poc.replaceQueryParam("token", "%s"))	 
}
`, notFilterToken, notFilterToken2)
		client, err := NewLocalClient()
		if err != nil {
			panic(err)
		}
		mitmPort := utils.GetRandomAvailableTCPPort()
		proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()

		RunMITMV2TestServer(client, ctx, &ypb.MITMV2Request{
			Host: "127.0.0.1",
			Port: uint32(mitmPort),
		}, func(stream ypb.Yak_MITMV2Client) {
			stream.Send(&ypb.MITMV2Request{
				SetYakScript:     true,
				YakScriptContent: code,
			})
			stream.Recv()
			stream.Send(&ypb.MITMV2Request{
				UpdateFilter: true,
				FilterData:   &ypb.MITMFilterData{},
			})
			stream.Recv()
			defer GetMITMFilterManager(consts.GetGormProjectDatabase(), consts.GetGormProfileDatabase()).Recover()
			_, err := lowhttp.HTTPWithoutRedirect(lowhttp.WithPacketBytes(packet), lowhttp.WithProxy(proxy))
			require.NoError(t, err)
			time.Sleep(3 * time.Second)
			cancel()
		})
		require.True(t, notFilter)
		require.True(t, notFilter2)
	})

	t.Run("content-type", func(t *testing.T) {
		var shouldFilter bool
		var notFilter bool

		_, mockPort := utils.DebugMockHTTPEx(func(req []byte) []byte {
			token := lowhttp.GetHTTPRequestQueryParam(req, "token")
			if token == shouldFilterToken {
				shouldFilter = true
			}
			if token == notFilterToken {
				notFilter = true
			}
			return []byte("HTTP/1.1 200 OK\r\nContent-length: 0\r\n\r\n")
		})
		packet := []byte(fmt.Sprintf(`POST /abc HTTP/1.1
Host: 127.0.0.1:%d
Content-Type: application/javascript
Content-Length: 98
`, mockPort))

		code := fmt.Sprintf(`
mirrorFilteredHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
	poc.HTTP(req,poc.replaceQueryParam("token", "%s"))
}

mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
   poc.HTTP(req,poc.replaceQueryParam("token", "%s"))	 
}
`, shouldFilterToken, notFilterToken)
		client, err := NewLocalClient()
		if err != nil {
			panic(err)
		}
		mitmPort := utils.GetRandomAvailableTCPPort()
		proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()

		RunMITMV2TestServer(client, ctx, &ypb.MITMV2Request{
			Host: "127.0.0.1",
			Port: uint32(mitmPort),
		}, func(stream ypb.Yak_MITMV2Client) {
			stream.Send(&ypb.MITMV2Request{
				SetYakScript:     true,
				YakScriptContent: code,
			})
			stream.Recv()
			stream.Send(&ypb.MITMV2Request{
				UpdateFilter: true,
				FilterData:   &ypb.MITMFilterData{},
			})
			stream.Recv()
			defer GetMITMFilterManager(consts.GetGormProjectDatabase(), consts.GetGormProfileDatabase()).Recover()
			_, err := lowhttp.HTTPWithoutRedirect(lowhttp.WithPacketBytes(packet), lowhttp.WithProxy(proxy))
			require.NoError(t, err)
			time.Sleep(3 * time.Second)
			cancel()
		})
		require.True(t, notFilter)
		require.False(t, shouldFilter)
	})

	t.Run("content-type and accept", func(t *testing.T) {
		var notFilter, notFilter2 bool

		_, mockPort := utils.DebugMockHTTPEx(func(req []byte) []byte {
			token := lowhttp.GetHTTPRequestQueryParam(req, "token")
			if token == notFilterToken {
				notFilter = true
			}
			if token == notFilterToken2 {
				notFilter2 = true
			}
			return []byte("HTTP/1.1 200 OK\r\nContent-length: 0\r\n\r\n")
		})
		packet := []byte(fmt.Sprintf(`POST /abc HTTP/1.1
Host:  127.0.0.1:%d
Content-Type: application/json
Accept: application/json, text/javascript, */*; q=0.01
`, mockPort))

		code := fmt.Sprintf(`
mirrorFilteredHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
	poc.HTTP(req,poc.replaceQueryParam("token", "%s"))
}

mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
   poc.HTTP(req,poc.replaceQueryParam("token", "%s"))	 
}
`, notFilterToken, notFilterToken2)
		client, err := NewLocalClient()
		if err != nil {
			panic(err)
		}
		mitmPort := utils.GetRandomAvailableTCPPort()
		proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()

		RunMITMV2TestServer(client, ctx, &ypb.MITMV2Request{
			Host: "127.0.0.1",
			Port: uint32(mitmPort),
		}, func(stream ypb.Yak_MITMV2Client) {
			stream.Send(&ypb.MITMV2Request{
				SetYakScript:     true,
				YakScriptContent: code,
			})
			stream.Recv()
			stream.Send(&ypb.MITMV2Request{
				UpdateFilter: true,
				FilterData:   &ypb.MITMFilterData{},
			})
			stream.Recv()
			defer GetMITMFilterManager(consts.GetGormProjectDatabase(), consts.GetGormProfileDatabase()).Recover()
			_, err := lowhttp.HTTPWithoutRedirect(lowhttp.WithPacketBytes(packet), lowhttp.WithProxy(proxy))
			require.NoError(t, err)
			time.Sleep(3 * time.Second)
			cancel()
		})
		require.True(t, notFilter)
		require.True(t, notFilter2)
	})
}

func TestGRPCMUSTPASS_MITMV2_Filter_Set_Get(t *testing.T) {
	ctx := utils.TimeoutContextSeconds(5)
	client, err := NewLocalClient()
	require.NoError(t, err)

	data := &ypb.MITMFilterData{
		IncludeUri: []*ypb.FilterDataItem{{MatcherType: "word", Group: []string{"abc"}}},
	}

	_, err = client.SetMITMFilter(ctx, &ypb.SetMITMFilterRequest{
		FilterData: data,
	})
	require.NoError(t, err)

	defer GetMITMFilterManager(consts.GetGormProjectDatabase(), consts.GetGormProfileDatabase()).Recover()
	filter, err := client.GetMITMFilter(ctx, &ypb.Empty{})
	require.NoError(t, err)
	require.Equal(t, "word", filter.FilterData.IncludeUri[0].MatcherType)
	require.Equal(t, []string{"abc"}, filter.FilterData.IncludeUri[0].Group)
}

func TestGRPCMUSTPASS_MITMV2_Filter_Reset(t *testing.T) {
	ctx := utils.TimeoutContextSeconds(5)
	client, err := NewLocalClient()
	require.NoError(t, err)
	token := uuid.NewString()
	data := &ypb.MITMFilterData{
		IncludeUri: []*ypb.FilterDataItem{{MatcherType: "word", Group: []string{token}}},
	}

	_, err = client.SetMITMFilter(ctx, &ypb.SetMITMFilterRequest{
		FilterData: data,
	})
	require.NoError(t, err)
	rsp, err := client.ResetMITMFilter(ctx, &ypb.Empty{})
	require.NoError(t, err)
	require.NotContains(t, rsp.FilterData.String(), token)
	require.Contains(t, rsp.FilterData.String(), "google.com")
}
