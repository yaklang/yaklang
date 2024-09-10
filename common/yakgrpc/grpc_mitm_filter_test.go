package yakgrpc

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/websocket"

	"github.com/bytedance/mockey"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var (
	mockGetFilterMap = make(map[*Server]*MITMFilterManager, 0)
	mockLock         = new(sync.Mutex)
)

func init() {
	mockey.Mock((*Server).getMITMFilter).When(func(self *Server) bool {
		_, ok := mockGetFilterMap[self]
		return ok
	}).To(func(self *Server) *MITMFilterManager {
		return mockGetFilterMap[self]
	}).Build()
}

func registerMockServerGetMITMFilter(s *Server, filter *MITMFilterManager) {
	mockLock.Lock()
	defer mockLock.Unlock()

	mockGetFilterMap[s] = filter
}

func Test_ForExcludeBadCase(t *testing.T) {
	_, mockPort := utils.DebugMockHTTPEx(func(req []byte) []byte {
		ct := lowhttp.GetHTTPRequestQueryParam(req, "ct")
		rsp, _, _ := lowhttp.FixHTTPResponse([]byte("HTTP/1.1 200 OK\r\nD: 1\r\n\r\n" + time.Now().String()))
		if ct != "" {
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Content-Type", ct)
			rsp = lowhttp.ReplaceHTTPPacketBodyFast(rsp, []byte("abc"))
		}
		return rsp
	})

	client, err := NewLocalClient(true)
	require.NoError(t, err)
	server := client.server
	require.NotNil(t, server)
	filter := &MITMFilterManager{
		db: server.GetProjectDatabase(),
	}
	filter.Recover(false)
	registerMockServerGetMITMFilter(server, filter)

	mitmPort := utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	RunMITMTestServer(client, ctx, &ypb.MITMRequest{
		Port: uint32(mitmPort),
		Host: "127.0.0.1",
	}, func(mitmClient ypb.Yak_MITMClient) {
		var token string
		var packet []byte

		filter.ExcludeSuffix = []string{".gif"}
		for _, ct := range [][]any{
			{"/abc.a", 0},
		} {
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
			_, _, err = poc.HTTP(packet, poc.WithProxy(proxy), poc.WithHost("127.0.0.1"), poc.WithPort(mockPort))
			require.NoError(t, err)
			_, err := QueryHTTPFlows(utils.TimeoutContextSeconds(2), client, &ypb.QueryHTTPFlowRequest{
				Keyword:    token,
				SourceType: "mitm",
			}, expectCount)
			require.NoErrorf(t, err, "test path failed: %s", ct[0])
		}
		cancel()
	})
}

func TestGRPCMUSTPASS_MITM_Filter_ForExcludeURI(t *testing.T) {
	_, mockPort := utils.DebugMockHTTPEx(func(req []byte) []byte {
		ct := lowhttp.GetHTTPRequestQueryParam(req, "ct")
		rsp, _, _ := lowhttp.FixHTTPResponse([]byte("HTTP/1.1 200 OK\r\nD: 1\r\n\r\n" + time.Now().String()))
		if ct != "" {
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Content-Type", ct)
			rsp = lowhttp.ReplaceHTTPPacketBodyFast(rsp, []byte("abc"))
		}
		return rsp
	})
	client, err := NewLocalClient(true)
	require.NoError(t, err)
	server := client.server
	require.NotNil(t, server)
	filter := &MITMFilterManager{
		db: server.GetProjectDatabase(),
	}
	filter.Recover(false)
	registerMockServerGetMITMFilter(server, filter)

	mitmPort := utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	RunMITMTestServer(client, ctx, &ypb.MITMRequest{
		Port: uint32(mitmPort),
		Host: "127.0.0.1",
	}, func(mitmClient ypb.Yak_MITMClient) {
		var token string
		var packet []byte

		filter.ExcludeMethods = []string{"NONONO"}
		filter.ExcludeUri = []string{"abc"}
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
			token = uuid.NewString()
			packet = []byte(fmt.Sprintf("GET %s HTTP/1.1\r\nX-TOKEN: %s\r\nHost: %s\r\n", path, token, utils.HostPort("127.0.0.1", mockPort)))
			_, _, err = poc.HTTP(packet, poc.WithProxy(proxy), poc.WithHost("127.0.0.1"), poc.WithPort(mockPort))
			require.NoError(t, err)
			_, err := QueryHTTPFlows(utils.TimeoutContextSeconds(2), client, &ypb.QueryHTTPFlowRequest{
				Keyword:    token,
				SourceType: "mitm",
			}, expectCount)
			require.NoErrorf(t, err, "test path failed: %s", ct[0])
		}
		cancel()
	})
}

func TestGRPCMUSTPASS_MITM_Filter_ForExcludeSuffixAndContentType(t *testing.T) {
	_, mockPort := utils.DebugMockHTTPEx(func(req []byte) []byte {
		ct := lowhttp.GetHTTPRequestQueryParam(req, "ct")
		rsp, _, _ := lowhttp.FixHTTPResponse([]byte("HTTP/1.1 200 OK\r\nD: 1\r\n\r\n" + time.Now().String()))
		if ct != "" {
			rsp = lowhttp.ReplaceHTTPPacketHeader(rsp, "Content-Type", ct)
			rsp = lowhttp.ReplaceHTTPPacketBodyFast(rsp, []byte("abc"))
		}
		return rsp
	})

	client, err := NewLocalClient(true)
	require.NoError(t, err)
	server := client.server
	require.NotNil(t, server)
	filter := &MITMFilterManager{
		db: server.GetProjectDatabase(),
	}
	filter.Recover(false)
	registerMockServerGetMITMFilter(server, filter)

	mitmPort := utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	RunMITMTestServer(client, ctx, &ypb.MITMRequest{
		Port: uint32(mitmPort),
		Host: "127.0.0.1",
	}, func(mitmClient ypb.Yak_MITMClient) {
		var token string
		var packet []byte

		filter.ExcludeSuffix = []string{".aaac", ".zip", ".js"}
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
			token = uuid.NewString()
			packet = []byte(fmt.Sprintf("GET %s HTTP/1.1\r\nX-TOKEN: %s\r\nHost: %s\r\n", path, token, utils.HostPort("127.0.0.1", mockPort)))
			_, _, err = poc.HTTP(packet, poc.WithProxy(proxy), poc.WithHost("127.0.0.1"), poc.WithPort(mockPort))
			require.NoError(t, err)
			_, err := QueryHTTPFlows(utils.TimeoutContextSeconds(2), client, &ypb.QueryHTTPFlowRequest{
				Keyword:    token,
				SourceType: "mitm",
			}, expectCount)
			require.NoErrorf(t, err, "test path failed: %s", ct[0])
		}

		{
			filter.ExcludeSuffix = []string{".aaac"}
			filter.ExcludeMethods = []string{"NONONO"}
			filter.ExcludeMIME = []string{"bbbbbb", "*cc", "*oct", "abc", "text"}
		}

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
			contentType := utils.InterfaceToString(ct[0])
			expectCount := codec.Atoi(utils.InterfaceToString(ct[1]))
			token = uuid.NewString()
			packet = []byte(fmt.Sprintf("GET /?ct=%s HTTP/1.1\r\nX-TOKEN: %s\r\nHost: %s\r\n", contentType, token, utils.HostPort("127.0.0.1", mockPort)))
			_, _, err = poc.HTTP(packet, poc.WithProxy(proxy), poc.WithHost("127.0.0.1"), poc.WithPort(mockPort))
			require.NoError(t, err)
			_, err := QueryHTTPFlows(utils.TimeoutContextSeconds(2), client, &ypb.QueryHTTPFlowRequest{
				Keyword:    token,
				SourceType: "mitm",
			}, expectCount)
			require.NoErrorf(t, err, "test content-type failed: %s", ct[0])
		}
		cancel()
	})
}

func TestMITMFilterManager_Filter(t *testing.T) {
	type Case struct {
		Filter *MITMFilterManager
		Send   [][]any
		Count  int
	}
	cases := []Case{
		{
			Filter: &MITMFilterManager{
				IncludeUri: []string{"abc"},
			},
			Send: [][]any{
				{
					"GET", "localhost:80", "/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabc", "", false,
				},
			},
			Count: 1,
		},
		{
			Filter: &MITMFilterManager{
				IncludeUri: []string{"/ab*c"},
			},
			Send: [][]any{
				{
					"GET", "localhost:80", "/abbbbbbc", "", false,
				},
				{
					"GET", "localhost:80", "/abaaaac", "", false,
				},
			},
			Count: 2,
		},
	}

	for _, c := range cases {
		var count int
		for _, send := range c.Send {
			if c.Filter.IsPassed(send[0].(string), send[1].(string), send[2].(string), send[3].(string), send[4].(bool)) {
				count++
			}
		}
		assert.Equal(t, c.Count, count)
	}
}

func TestGRPCMUSTPASS_WebSocket_Filter_RSP(t *testing.T) {
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

	RunMITMTestServer(client, ctx, &ypb.MITMRequest{
		Port: uint32(mitmPort),
		Host: "127.0.0.1",
	}, func(mitmClient ypb.Yak_MITMClient) {
		mitmClient.Send(&ypb.MITMRequest{
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

func TestGRPCMUSTPASS_WebSocket_Filter_REQ(t *testing.T) {
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

	RunMITMTestServer(client, ctx, &ypb.MITMRequest{
		Port: uint32(mitmPort),
		Host: "127.0.0.1",
	}, func(mitmClient ypb.Yak_MITMClient) {
		mitmClient.Send(&ypb.MITMRequest{
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
