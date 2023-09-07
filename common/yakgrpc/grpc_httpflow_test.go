package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_QueryHTTPFlow_Oversize(t *testing.T) {
	var client, err = NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	rsp, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 OK
Server: test
`))

	var flow *yakit.HTTPFlow
	flow, err = yakit.CreateHTTPFlowFromHTTPWithBodySavedFromRaw(true, lowhttp.FixHTTPRequest([]byte(
		`GET / HTTP/1.1
Host: www.example.com
`)), lowhttp.ReplaceHTTPPacketBodyFast(rsp, []byte(strings.Repeat(strings.Repeat("a", 1000), 1000))), "abc",
		"https://www.example.com", "",
	)
	if err != nil {
		t.Fatal(err)
	}
	flow.CalcHash()
	consts.GetGormProjectDatabase().Save(flow)

	flow, err = yakit.CreateHTTPFlowFromHTTPWithBodySavedFromRaw(true, lowhttp.FixHTTPRequest([]byte(
		`GET / HTTP/1.1
Host: www.example.com
`)), lowhttp.ReplaceHTTPPacketBodyFast(rsp, []byte(strings.Repeat(strings.Repeat("a", 11), 11))), "abc",
		"https://www.example.com", "",
	)
	if err != nil {
		t.Fatal(err)
	}
	flow.CalcHash()
	consts.GetGormProjectDatabase().Save(flow)

	resp, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   100,
			OrderBy: "body_length",
			Order:   "desc",
		},
		Full:       false,
		SourceType: "abc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetData()) <= 0 {
		t.Fatal("resp should not be empty")
	}

	var checkLargeBodyId int64
	for _, r := range resp.GetData() {
		if r.BodyLength > 800*1000 {
			checkLargeBodyId = int64(r.GetId())
			if len(r.Response) != 0 {
				t.Fatal("response should be empty")
			}
		} else if r.BodyLength < 100*1000 {
			if len(r.Response) == 0 {
				spew.Dump(r.Response)
				println(string(r.Response))
				t.Fatal("response should not be empty")
			}
		}
	}

	if checkLargeBodyId <= 0 {
		t.Fatal("no large body found")
	}

	start := time.Now()
	response, err := client.GetHTTPFlowById(utils.TimeoutContext(1*time.Second), &ypb.GetHTTPFlowByIdRequest{Id: checkLargeBodyId})
	if err != nil {
		spew.Dump(err)
		t.Fatal("cannot found large response")
	}
	if time.Now().Sub(start).Seconds() > 500 {
		t.Fatal("should be cached")
	}
	_ = response
	if len(response.GetResponse()) < 1000*800 {
		t.Fatal("response is missed")
	}
}

func TestGRPCMUSTPASS_HijackedFlow_Request(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	token1 := utils.RandStringBytes(20)
	token2 := utils.RandStringBytes(20)
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Token") == token1 {
			writer.Write([]byte(token2))
		} else {
			writer.Write([]byte("nonono"))
		}
	})
	target := utils.HostPort(host, port)

	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(1000))
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
	stream.Send(&ypb.MITMRequest{SetAutoForward: true, AutoForwardValue: false})
	for {
		rcpResponse, err := stream.Recv()
		if err != nil {
			break
		}
		rspMsg := string(rcpResponse.GetMessage().GetMessage())
		if rcpResponse.GetHaveMessage() {
		} else if len(rcpResponse.GetRequest()) > 0 {
			req := bytes.ReplaceAll(rcpResponse.GetRequest(), []byte("aaaaa"), []byte(token1))
			stream.Send(&ypb.MITMRequest{
				Request:    req,
				Id:         rcpResponse.GetId(),
				ResponseId: rcpResponse.GetResponseId(),
			})
		}
		if strings.Contains(rspMsg, `starting mitm serve`) {
			go func() {
				time.Sleep(time.Second)
				packet := `GET / HTTP/1.1
Host: ` + target
				_, err := yak.Execute(`
rsp, req = poc.HTTP(packet, poc.proxy(mitmProxy))~
assert string(poc.Split(rsp)[1]) == token2
`, map[string]any{
					"packet":    []byte(lowhttp.ReplaceHTTPPacketHeader([]byte(packet), "Token", "aaaaa")),
					"token2":    token2,
					"mitmProxy": "http://" + utils.HostPort("127.0.0.1", mitmPort),
				})
				if err != nil {
					t.Fatal(err)
				}
				cancel()
			}()
		}
	}

	rpcResponse, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
		Pagination: &ypb.Paging{
			Page:  1,
			Limit: 100,
		},
		SourceType: "mitm",
		Keyword:    token2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rpcResponse.GetTotal() <= 0 {
		t.Fatal("no flow")
	}
	flow := rpcResponse.GetData()[0]
	finalRequest := flow.Request
	rpcResponse2, err := client.GetHTTPFlowBare(context.Background(), &ypb.HTTPFlowBareRequest{
		Id:       int64(flow.GetId()),
		BareType: "request",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 检验原始请求
	if !strings.Contains(string(rpcResponse2.GetData()), "Token: aaaaa") {
		t.Fatal("not found origin token")
	}
	// 检验最终请求
	data := finalRequest
	if !strings.Contains(string(data), "Token: "+token1) {
		t.Fatal("not found replaced token")
	}
}

func TestGRPCMUSTPASS_HijackedFlow_Response(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	token1 := utils.RandStringBytes(20)
	token2 := utils.RandStringBytes(20)
	log.Infof("token1: %s, token2: %s", token1, token2)
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		fmt.Fprint(writer, token1)
	})
	target := utils.HostPort(host, port)

	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(1000))
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
	for {
		rcpResponse, err := stream.Recv()
		if err != nil {
			break
		}
		rspMsg := string(rcpResponse.GetMessage().GetMessage())
		if rcpResponse.GetHaveMessage() {
		} else if len(rcpResponse.GetRequest()) > 0 {
			stream.Send(&ypb.MITMRequest{
				Id:      rcpResponse.GetId(),
				Request: rcpResponse.GetRequest(),
			})
			stream.Send(&ypb.MITMRequest{
				Id:             rcpResponse.GetId(),
				HijackResponse: true,
			})
			if len(rcpResponse.GetResponse()) > 0 {
				rsp := bytes.ReplaceAll(rcpResponse.GetResponse(), []byte(token1), []byte(token2))
				stream.Send(&ypb.MITMRequest{
					Response:   rsp,
					Id:         rcpResponse.GetId(),
					ResponseId: rcpResponse.GetResponseId(),
				})
			}
		}
		if strings.Contains(rspMsg, `starting mitm serve`) {
			go func() {
				time.Sleep(time.Second)
				packet := `GET / HTTP/1.1
Host: ` + target
				_, err := yak.Execute(`
rsp, req = poc.HTTP(packet, poc.proxy(mitmProxy))~
body = poc.Split(rsp)[1]
assert string(body) == token2, sprintf("get %s != %s", string(body), string(token2))
`, map[string]any{
					"packet":    []byte(packet),
					"token2":    token2,
					"mitmProxy": "http://" + utils.HostPort("127.0.0.1", mitmPort),
				})
				if err != nil {
					t.Fatal(err)
				}
				cancel()
			}()
		}
	}

	rpcResponse, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
		Pagination: &ypb.Paging{
			Page:  1,
			Limit: 100,
		},
		SourceType: "mitm",
		Keyword:    token2,
		Full:       true,
	})

	if err != nil {
		t.Fatal(err)
	}
	if rpcResponse.GetTotal() <= 0 {
		t.Fatal("no flow")
	}

	flow := rpcResponse.GetData()[0]
	finalResponse := flow.Response
	rpcResponse2, err := client.GetHTTPFlowBare(context.Background(), &ypb.HTTPFlowBareRequest{
		Id:       int64(flow.GetId()),
		BareType: "response",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 检验原始响应
	if !strings.Contains(string(rpcResponse2.GetData()), token1) {
		t.Fatalf("not found origin token, raw response: %s", string(rpcResponse2.GetData()))
	}
	// 检验最终响应
	if !strings.Contains(string(finalResponse), token2) {
		t.Fatalf("not found replaced token, final response: %s", string(finalResponse))
	}
}
