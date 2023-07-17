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
			{"/static/abc.ppt", 0},
			{"/abc.aaac", 0},
			{"/a1bc.aaac", 1},
		} {
			path := utils.InterfaceToString(ct[0])
			expectCount := utils.Atoi(utils.InterfaceToString(ct[1]))
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
			count := yakit.QuickSearchHTTPFlowCount(token)
			log.Infof("yakit.QuickSearchHTTPFlowCount("+`[`+token+`]`+") == %v", count)
			if count != expectCount {
				cancel()
				t.Log("search httpflow by token failed: yakit.QuickSearchHTTPFlowCount(token)")
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
			ExcludeSuffix:       []string{".aaac"},
			ExcludeMethod:       []string{"NONONO"},
			ExcludeContentTypes: nil,
			ExcludeUri:          nil,
			UpdateFilter:        true,
		})
		time.Sleep(500 * time.Millisecond)
		for _, ct := range [][]any{
			{"/abc.a", 1},
			{"/a/abc.js", 1},
			{"/static/abc.ppt", 1},
			{"/abc.aaac", 0},
		} {
			path := utils.InterfaceToString(ct[0])
			expectCount := utils.Atoi(utils.InterfaceToString(ct[1]))
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
			count := yakit.QuickSearchHTTPFlowCount(token)
			log.Infof("yakit.QuickSearchHTTPFlowCount("+`[`+token+`]`+") == %v", count)
			if count != expectCount {
				cancel()
				t.Log("search httpflow by token failed: yakit.QuickSearchHTTPFlowCount(token)")
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
			{"application/abc", 1},
			{"abc1111", 1},
			{"application/oct", 0},
			{"bbbbbb", 0},
			{"aabb", 1},
			{"cccc", 0},
			{"ccc", 0},
			{"cc", 0},
		} {
			var path = "/"
			var contentType = utils.InterfaceToString(ct[0])
			expectCount := utils.Atoi(utils.InterfaceToString(ct[1]))
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
			count := yakit.QuickSearchHTTPFlowCount(token)
			log.Infof("yakit.QuickSearchHTTPFlowCount("+`[`+token+`]`+") == %v", count)
			if count != expectCount {
				cancel()
				t.Log("search httpflow by token failed: yakit.QuickSearchHTTPFlowCount(token)")
				t.FailNow()
			}
		}
		cancel()
	})
}
