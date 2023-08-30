package yakgrpc

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"sync"
	"testing"
	"time"
)

func RunMITMTestServer(
	client ypb.YakClient,
	ctx context.Context,
	req *ypb.MITMRequest,
	onLoad func(mitmClient ypb.Yak_MITMClient),
) (host, port string) {
	stream, err := client.MITM(ctx)
	if err != nil {
		panic(err)
	}
	stream.Send(req)
	wg := sync.WaitGroup{}
	wg.Add(1)
	for {
		msg, err := stream.Recv()
		if err != nil {
			break
		}
		msgStr := spew.Sdump(msg)
		fmt.Println("MTIM CLIENT RECV: " + msgStr)
		if strings.Contains(msgStr, `MITM 服务器已启动`) {
			go func() {
				defer wg.Done()
				onLoad(stream)
			}()
		}
	}
	wg.Wait()
	return
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

	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	var mitmPort = utils.GetRandomAvailableTCPPort()
	var proxy = "http://" + utils.HostPort("127.0.0.1", mitmPort)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	RunMITMTestServer(client, ctx, &ypb.MITMRequest{
		Port: uint32(mitmPort),
		Host: "127.0.0.1",
	}, func(mitmClient ypb.Yak_MITMClient) {
		var token string
		var packet []byte

		mitmClient.Send(&ypb.MITMRequest{
			ExcludeSuffix: []string{".gif"},
			UpdateFilter:  true,
		})
		time.Sleep(500 * time.Millisecond)
		for _, ct := range [][]any{
			{"/abc.a", 0},
		} {
			// path := utils.InterfaceToString(ct[0])
			expectCount := codec.Atoi(utils.InterfaceToString(ct[1]))
			token = "Xsjip0QIZ8tyhnq"
			packet = []byte(`GET /-L-Xsjip0QIZ8tyhnq/v.gif?logactid=1234567890&showTab=10000&opType=showpv&mod=superman%3Alib&submod=index&superver=supernewplus&glogid=2147883968&type=2011&pid=315&isLogin=0&version=PCHome&terminal=PC&qid=0xc349374900061bc0&sid=36551_38642_38831_39027_39022_38958_38955_39014_39038_38811_39084_38639_26350_39095_39100&super_frm=&from_login=&from_reg=&query=&curcard=2&curcardtab=&_r=0.9024198609355389 HTTP/1.1
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
			if count != expectCount {
				cancel()
				t.Log("search httpflow by token failed: yakit.QuickSearchMITMHTTPFlowCount(token)")
				t.FailNow()
			}
		}
		cancel()
	})
}

func TestGRPCMUSTPASS_MITMFilter_ForExcludeURI(t *testing.T) {
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
	var mitmPort = utils.GetRandomAvailableTCPPort()
	var proxy = "http://" + utils.HostPort("127.0.0.1", mitmPort)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	RunMITMTestServer(client, ctx, &ypb.MITMRequest{
		Port: uint32(mitmPort),
		Host: "127.0.0.1",
	}, func(mitmClient ypb.Yak_MITMClient) {
		var token string
		var packet []byte

		mitmClient.Send(&ypb.MITMRequest{
			ExcludeMethod: []string{"NONONO"},
			ExcludeUri:    []string{"abc"},
			UpdateFilter:  true,
		})
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
			if count != expectCount {
				cancel()
				t.Log("search httpflow by token failed: yakit.QuickSearchMITMHTTPFlowCount(token)")
				t.FailNow()
			}
		}
		cancel()
	})
}

func TestGRPCMUSTPASS_MITMFilter_ForExcludeSuffixAndContentType(t *testing.T) {
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
	var mitmPort = utils.GetRandomAvailableTCPPort()
	var proxy = "http://" + utils.HostPort("127.0.0.1", mitmPort)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	RunMITMTestServer(client, ctx, &ypb.MITMRequest{
		Port: uint32(mitmPort),
		Host: "127.0.0.1",
	}, func(mitmClient ypb.Yak_MITMClient) {
		var token string
		var packet []byte

		mitmClient.Send(&ypb.MITMRequest{
			ExcludeSuffix: []string{".aaac", ".zip"},
			UpdateFilter:  true,
		})
		time.Sleep(500 * time.Millisecond)
		for _, ct := range [][]any{
			{"/abc.png.zip?ab=1", 0},
			{"/abc.a", 1},
			{"/a/abc.js", 1},
			{"/static/abc.ppt", 1},
			{"/abc.aaac", 0},
			{"/abc.jpg", 1},
			{"/abc.png.zip", 0},
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
			if count != expectCount {
				cancel()
				t.Log("search httpflow by token failed: yakit.QuickSearchMITMHTTPFlowCount(token)")
				t.FailNow()
			}
		}

		mitmClient.Send(&ypb.MITMRequest{
			ExcludeSuffix:       []string{".aaac"},
			ExcludeMethod:       []string{"NONONO"},
			ExcludeContentTypes: []string{"bbbbbb", "*cc", "*oct", "abc"},
			ExcludeUri:          nil,
			IncludeUri:          nil,
			UpdateFilter:        true,
		})
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
		} {
			var path = "/"
			var contentType = utils.InterfaceToString(ct[0])
			expectCount := codec.Atoi(utils.InterfaceToString(ct[1]))
			token = ksuid.New().String()
			packet = []byte("GET " + path + "?ct=" + codec.QueryEscape(contentType) + " HTTP/1.1\r\nHost: " + utils.HostPort("127.0.0.1", mockPort))
			params := map[string]any{"proxy": proxy, "mockHost": "127.0.0.1", "mockPort": mockPort, "token": token}
			packet = lowhttp.ReplaceHTTPPacketHeader(packet, "X-TOKEN", token)
			params["packet"] = packet
			_, err = yak.Execute(`
println(string(packet))
rsp, _ = poc.HTTP(packet, poc.proxy(proxy), poc.host(mockHost), poc.port(mockPort))~
println(string(rsp))
sleep(0.5)
`, params)
			if err != nil {
				t.Logf("err: %v", err)
				t.Fail()
			}
			count := yakit.QuickSearchMITMHTTPFlowCount(token)
			log.Infof("yakit.QuickSearchMITMHTTPFlowCount("+`[`+token+`]`+") == %v", count)
			if count != expectCount {
				cancel()
				t.Log("search httpflow by token failed: yakit.QuickSearchMITMHTTPFlowCount(token)")
				t.FailNow()
			}
		}
		cancel()
	})
}
