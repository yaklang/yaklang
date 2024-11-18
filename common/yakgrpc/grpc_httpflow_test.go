package yakgrpc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_HTTP_QueryHTTPFlow_Oversize(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	rsp, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 OK
Server: test
`))

	var flow *schema.HTTPFlow
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
		t.Fatalf("cannot found large response. error: %v", err)
	}
	if time.Now().Sub(start).Seconds() > 500 {
		t.Fatal("should be cached")
	}
	_ = response
	if len(response.GetResponse()) < 1000*800 {
		t.Fatal("response is missed")
	}
}

func TestGRPCMUSTPASS_HTTP_HijackedFlow_Request(t *testing.T) {
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

	var rpcResponse *ypb.QueryHTTPFlowResponse
	err = utils.AttemptWithDelayFast(func() error {
		rpcResponse, err = client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 100,
			},
			SourceType: "mitm",
			Keyword:    token2,
		})
		if err != nil {
			return err
		}
		if rpcResponse.GetTotal() <= 0 {
			return utils.Errorf("got 0 flows")
		}
		return nil
	})
	require.NoError(t, err)

	flow := rpcResponse.GetData()[0]
	finalRequest := flow.Request
	var rpcResponse2 *ypb.HTTPFlowBareResponse
	err1 := utils.AttemptWithDelayFast(func() error {
		rpcResponse2, err = client.GetHTTPFlowBare(context.Background(), &ypb.HTTPFlowBareRequest{
			Id:       int64(flow.GetId()),
			BareType: "request",
		})
		return err
	})
	require.NoError(t, err1)

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

func TestGRPCMUSTPASS_HTTP_HijackedFlow_Response(t *testing.T) {
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
	var hasForward bool
	for {
		rcpResponse, err := stream.Recv()
		if err != nil {
			break
		}
		rspMsg := string(rcpResponse.GetMessage().GetMessage())
		if rcpResponse.GetHaveMessage() {
		} else if len(rcpResponse.GetRequest()) > 0 {
			if len(rcpResponse.GetResponse()) > 0 {
				rsp := bytes.ReplaceAll(rcpResponse.GetResponse(), []byte(token1), []byte(token2))
				stream.Send(&ypb.MITMRequest{
					Response:   rsp,
					Id:         rcpResponse.GetId(),
					ResponseId: rcpResponse.GetResponseId(),
				})
			}
			if hasForward {
				continue
			}
			stream.Send(&ypb.MITMRequest{
				Id:             rcpResponse.GetId(),
				HijackResponse: true,
			})
			stream.Send(&ypb.MITMRequest{
				Id:      rcpResponse.GetId(),
				Request: rcpResponse.GetRequest(),
			})
			hasForward = true
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
					panic(err)
				}
				cancel()
			}()
		}
	}

	var rpcResponse *ypb.QueryHTTPFlowResponse
	err = utils.AttemptWithDelayFast(func() error {
		rpcResponse, err = client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 100,
			},
			SourceType: "mitm",
			Keyword:    token2,
			Full:       true,
		})
		if err != nil {
			return err
		}
		if rpcResponse.GetTotal() <= 0 {
			return utils.Errorf("got 0 flows")
		}
		return nil
	})
	require.NoError(t, err)

	flow := rpcResponse.GetData()[0]
	finalResponse := flow.Response
	var rpcResponse2 *ypb.HTTPFlowBareResponse
	err1 := utils.AttemptWithDelayFast(func() error {
		rpcResponse2, err = client.GetHTTPFlowBare(context.Background(), &ypb.HTTPFlowBareRequest{
			Id:       int64(flow.GetId()),
			BareType: "response",
		})
		return err
	})
	require.NoError(t, err1)

	// 检验原始响应
	if !strings.Contains(string(rpcResponse2.GetData()), token1) {
		t.Fatalf("not found origin token, raw response: %s", string(rpcResponse2.GetData()))
	}
	// 检验最终响应
	if !strings.Contains(string(finalResponse), token2) {
		t.Fatalf("not found replaced token, final response: %s", string(finalResponse))
	}
}

//func TestHTTPFlowTreeHelper(t *testing.T) {
//	//db := yakit.FilterHTTPFlowByDomain(consts.GetGormProjectDatabase(), "w.baidu.com").Debug()
//	//for result := range yakit.YieldHTTPFlows(db, context.Background()) {
//	//	fmt.Println(result.Url)
//	//}
//	result := yakit.GetHTTPFlowNextPartPathByPathPrefix(consts.GetGormProjectDatabase(), "v1")
//	spew.Dump(result)
//}

func TestExportHTTPFlows(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.ExportHTTPFlows(context.Background(), &ypb.ExportHTTPFlowsRequest{
		ExportWhere: &ypb.QueryHTTPFlowRequest{
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 20,
			},
			Full: true,
		},
		Ids:       []int64{1, 2, 3, 4, 5},
		FieldName: []string{"url", "method", "status_code"},
	})
	if err != nil {
		t.Fatalf("export httpFlows error: %v", err)
	}
	_ = response
}

func TestExportHTTPFlowsWithPayload(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	host, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK
Content-Length: 5

hello`))

	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	stream, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
		Request: fmt.Sprintf(`GET /a={{int(1-5)}} HTTP/1.1
Host: %s

`, utils.HostPort(host, port)),
		ForceFuzz: true,
	})
	require.NoError(t, err)
	runtimeIDs := make([]string, 0)

	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		runtimeIDs = append(runtimeIDs, resp.RuntimeID)
	}

	responses, err := client.ExportHTTPFlows(context.Background(), &ypb.ExportHTTPFlowsRequest{
		ExportWhere: &ypb.QueryHTTPFlowRequest{
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 20,
			},
			Full:       true,
			RuntimeIDs: runtimeIDs,
		},
		FieldName: []string{"payloads"},
	})
	require.NoErrorf(t, err, "export httpFlows error")
	for _, flow := range responses.Data {
		require.NotEmpty(t, flow.Payloads)
	}
}

func TestGRPCMUSTPASS_MITM_PreSetTags(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	token1 := utils.RandStringBytes(20)
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("hello"))
	})
	target := utils.HostPort(host, port)

	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(1000))
	defer cancel()
	stream, err := client.MITM(ctx)
	require.NoError(t, err)

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
				Tags:       []string{"YAKIT_COLOR_RED"},
			})
		}
		if strings.Contains(rspMsg, `starting mitm serve`) {
			go func() {
				time.Sleep(time.Second)
				packet := `GET / HTTP/1.1
Host: ` + target
				_, err := yak.Execute(`
rsp, req = poc.HTTP(packet, poc.proxy(mitmProxy))~
`, map[string]any{
					"packet":    []byte(lowhttp.ReplaceHTTPPacketHeader([]byte(packet), "Token", "aaaaa")),
					"mitmProxy": "http://" + utils.HostPort("127.0.0.1", mitmPort),
				})
				require.NoError(t, err)
				cancel()
			}()
		}
	}

	rpcResponse, err := QueryHTTPFlows(utils.TimeoutContextSeconds(2), client, &ypb.QueryHTTPFlowRequest{
		SourceType: "mitm",
		Keyword:    token1,
	}, 1)
	require.NoError(t, err)

	flow := rpcResponse.GetData()[0]
	tags := strings.Split(flow.Tags, "|")
	require.Greater(t, len(tags), 0, "flow no tags")
	require.Equal(t, tags[0], "YAKIT_COLOR_RED", "flow preset tag not set")

	_, err = client.SetTagForHTTPFlow(context.Background(), &ypb.SetTagForHTTPFlowRequest{
		Id:   int64(flow.GetId()),
		Tags: strings.Split(strings.ReplaceAll(flow.GetTags(), "YAKIT_COLOR_RED", "YAKIT_COLOR_BLUE"), "|"),

		CheckTags: nil,
	})
	require.NoError(t, err)

	rpcResponse, err = QueryHTTPFlows(utils.TimeoutContextSeconds(2), client, &ypb.QueryHTTPFlowRequest{
		SourceType: "mitm",
		Keyword:    token1,
	}, 1)
	require.NoError(t, err)

	fixFlow := rpcResponse.GetData()[0]
	tags = strings.Split(fixFlow.Tags, "|")
	require.Greater(t, len(tags), 0, "flow no tags")
	require.Equal(t, tags[0], "YAKIT_COLOR_BLUE", "client.SetTagForHTTPFlow not work")
}

func TestGRPCMUSTPASS_HTTP_WithPayload(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	token := utils.RandStringBytes(20)
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		fmt.Fprint(writer, token)
	})
	target := utils.HostPort(host, port)

	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(5))
	defer cancel()
	stream, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
		Request: fmt.Sprintf(`GET /?a={{int(1-2)}} HTTP/1.1
Host: %s
`, target),
		ForceFuzz: true,
	})
	require.NoError(t, err)
	runtimeID := ""
	// wait until webfuzzer done
	for {
		resp, err := stream.Recv()
		if runtimeID == "" {
			runtimeID = resp.RuntimeID
		}
		if err != nil {
			break
		}
	}

	responses, err := QueryHTTPFlows(utils.TimeoutContextSeconds(5), client, &ypb.QueryHTTPFlowRequest{
		Pagination: &ypb.Paging{
			Page:  1,
			Limit: 100,
		},
		RuntimeId:   runtimeID,
		WithPayload: true,
	}, 2)
	require.NoError(t, err)
	require.ElementsMatch(t,
		lo.Map(responses.Data, func(f *ypb.HTTPFlow, _ int) []string {
			return f.Payloads
		}),
		[][]string{{"1"}, {"2"}},
	)
}

func TestGRPCMUSTPASS_HTTP_ConvertFuzzerResponseToHTTPFlow(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	token := utils.RandStringBytes(20)
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		fmt.Fprint(writer, token)
	})
	target := utils.HostPort(host, port)

	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(5))
	defer cancel()
	stream, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
		Request: fmt.Sprintf(`GET /?a HTTP/1.1
Host: %s
`, target),
		ForceFuzz: true,
	})
	require.NoError(t, err)
	var gotFlow *ypb.HTTPFlow
	// wait until webfuzzer done
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		gotFlow, err = client.ConvertFuzzerResponseToHTTPFlow(context.Background(), resp)
		require.NoError(t, err)
	}
	require.NotEmpty(t, gotFlow)

	reQueryFlow, err := client.GetHTTPFlowById(context.Background(), &ypb.GetHTTPFlowByIdRequest{
		Id: int64(gotFlow.GetId()),
	})
	_ = reQueryFlow
	require.NoError(t, err)
	require.NotEmpty(t, reQueryFlow)

	log.Infof("gotFlow: %v", gotFlow)
	log.Infof("reQueryFlow: %v", reQueryFlow)
	// require.Equal(t, gotFlow.GetId(), reQueryFlow.GetId())
}

func TestGRPCMUSTPASS_Delete_HTTPFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1000*time.Second)
	defer cancel()

	client, err := NewLocalClient(true)
	require.NoError(t, err)

	db := consts.GetGormProjectDatabase()
	token1 := utils.RandStringBytes(5)
	token2 := utils.RandStringBytes(5)

	url1 := "http://" + token1 + ".com"
	url2 := "http://" + token2 + ".com"
	for i := 0; i < 100; i++ {
		flow, err := yakit.CreateHTTPFlow(yakit.CreateHTTPFlowWithURL(url1))
		require.NoError(t, err)
		err = yakit.InsertHTTPFlow(db, flow)
		require.NoError(t, err)

		flow, err = yakit.CreateHTTPFlow(yakit.CreateHTTPFlowWithURL(url2))
		require.NoError(t, err)
		err = yakit.InsertHTTPFlow(db, flow)
		require.NoError(t, err)
	}

	_, err = client.DeleteHTTPFlows(ctx, &ypb.DeleteHTTPFlowRequest{
		Filter: &ypb.QueryHTTPFlowRequest{
			Keyword: token1,
		},
	})
	require.NoError(t, err)

	var count int
	yakit.FilterHTTPFlow(db, &ypb.QueryHTTPFlowRequest{Keyword: token1}).Count(&count)
	require.Equal(t, 0, count, "delete token1 fail")

	yakit.FilterHTTPFlow(db, &ypb.QueryHTTPFlowRequest{Keyword: token2}).Count(&count)
	require.Equal(t, 100, count, "error delete token2")

	err = yakit.DeleteHTTPFlow(db, &ypb.DeleteHTTPFlowRequest{
		Filter: &ypb.QueryHTTPFlowRequest{
			Keyword: token2,
		},
	})
	require.NoError(t, err)
}

func TestGRPCMUSTPASS_GetHTTPFlowBodyById(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client, err := NewLocalClient(true)
	require.NoError(t, err)

	db := consts.GetGormProjectDatabase()

	t.Run("request", func(t *testing.T) {
		token := utils.RandStringBytes(5)
		url1 := "http://" + token + ".com"
		flow1, err := yakit.CreateHTTPFlow(yakit.CreateHTTPFlowWithURL(url1), yakit.CreateHTTPFlowWithRequestRaw([]byte("GET / HTTP/1.1\r\nHost: "+token+".com\r\n\r\n"+token)))
		require.NoError(t, err)
		err = yakit.InsertHTTPFlow(db, flow1)
		require.NoError(t, err)
		defer yakit.DeleteHTTPFlowByID(db, int64(flow1.ID))

		count := 0
		stream, err := client.GetHTTPFlowBodyById(ctx, &ypb.GetHTTPFlowBodyByIdRequest{Id: int64(flow1.ID), IsRequest: true})
		require.NoError(t, err)
		for {
			msg, err := stream.Recv()
			if err != nil {
				break
			}
			count++
			if count == 1 {
				require.Equal(t, "body.txt", msg.GetFilename())
			} else if count == 2 {
				require.Equal(t, token, string(msg.GetData()))
				require.True(t, msg.GetEOF())
			}
		}
		require.Equal(t, 2, count, "should only have 2 messages")
	})

	t.Run("response", func(t *testing.T) {
		token := utils.RandStringBytes(5)
		url2 := "http://" + token + ".com/a.jpg"
		flow2, err := yakit.CreateHTTPFlow(yakit.CreateHTTPFlowWithURL(url2), yakit.CreateHTTPFlowWithRequestRaw([]byte("GET / HTTP/1.1\r\nHost: "+token+".com\r\n\r\n")), yakit.CreateHTTPFlowWithResponseRaw([]byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\nContent-Type: image/jpeg\r\n\r\n"+token)))
		require.NoError(t, err)
		err = yakit.InsertHTTPFlow(db, flow2)
		require.NoError(t, err)

		defer yakit.DeleteHTTPFlowByID(db, int64(flow2.ID))

		count := 0
		stream, err := client.GetHTTPFlowBodyById(ctx, &ypb.GetHTTPFlowBodyByIdRequest{Id: int64(flow2.ID)})
		require.NoError(t, err)
		for {
			msg, err := stream.Recv()
			if err != nil {
				break
			}
			count++
			if count == 1 {
				require.Equal(t, "a.jpg", msg.GetFilename())
			} else if count == 2 {
				require.Equal(t, token, string(msg.GetData()))
				require.True(t, msg.GetEOF())
			}
		}
		require.Equal(t, 2, count, "should only have 2 messages")
	})

	t.Run("too large response", func(t *testing.T) {
		token := utils.RandStringBytes(16)
		tempFileName, err := utils.SaveTempFile(token, "test-GetHTTPFlowBodyById")
		defer os.Remove(tempFileName)

		url2 := "http://test.com/a.jpg"
		flow2, err := yakit.CreateHTTPFlow(
			yakit.CreateHTTPFlowWithURL(url2),
			yakit.CreateHTTPFlowWithRequestRaw([]byte("GET / HTTP/1.1\r\nHost: test.com\r\n\r\n")),
			yakit.CreateHTTPFlowWithTooLargeResponseBodyFile(tempFileName),
		)
		require.NoError(t, err)
		err = yakit.InsertHTTPFlow(db, flow2)
		require.NoError(t, err)

		defer yakit.DeleteHTTPFlowByID(db, int64(flow2.ID))

		count := 0
		stream, err := client.GetHTTPFlowBodyById(ctx, &ypb.GetHTTPFlowBodyByIdRequest{Id: int64(flow2.ID)})
		require.NoError(t, err)
		for {
			msg, err := stream.Recv()
			if err != nil {
				break
			}
			count++
			if count == 1 {
				require.Equal(t, "a.jpg", msg.GetFilename())
			} else if count == 2 {
				require.Equal(t, token, string(msg.GetData()))
				require.True(t, msg.GetEOF())
			}
		}
		require.Equal(t, 2, count, "should only have 2 messages")
	})
	t.Run("get risk body", func(t *testing.T) {
		target := uuid.NewString()
		content := uuid.NewString()
		risk := &schema.Risk{
			Url: target,
			QuotedRequest: strconv.Quote(fmt.Sprintf(`POST / HTTP/1.1
Content-Type: application/json
Host: www.example.com

%s`, content)),
		}
		err2 := yakit.SaveRisk(risk)
		require.NoError(t, err2)
		defer func() {
			yakit.DeleteRiskByTarget(consts.GetGormProjectDatabase(), target)
		}()
		c, err2 := NewLocalClient(true)
		require.NoError(t, err2)
		stream, err2 := c.GetHTTPFlowBodyById(context.Background(), &ypb.GetHTTPFlowBodyByIdRequest{
			Id:        int64(risk.ID),
			IsRequest: true,
			IsRisk:    true,
		})
		require.NoError(t, err2)
		count := 0
		for {
			recv, err2 := stream.Recv()
			if err2 != nil {
				break
			}
			count++
			if count == 2 {
				data := recv.GetData()
				fmt.Println(content)
				fmt.Println(string(data))
				require.True(t, string(data) == content)
			}
		}
	})
}

func TestGetHTTPPacketBody(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1000*time.Second)
	defer cancel()

	client, err := NewLocalClient(true)
	require.NoError(t, err)

	packet := []byte(`HTTP/1.1 200 OK
Content-Type: text/plain; charset=utf-8
Content-Length: 19

{{unquote("\x41")}}`)

	t.Run("not render fuzztag", func(t *testing.T) {
		packetBody, err := client.GetHTTPPacketBody(ctx, &ypb.GetHTTPPacketBodyRequest{
			PacketRaw: packet,
		})
		require.NoError(t, err)
		require.Equal(t, []byte("{{unquote(\"\\x41\")}}"), packetBody.GetRaw())
	})

	t.Run("render fuzztag", func(t *testing.T) {
		packetBody, err := client.GetHTTPPacketBody(ctx, &ypb.GetHTTPPacketBodyRequest{
			PacketRaw:          packet,
			ForceRenderFuzztag: true,
		})
		require.NoError(t, err)
		require.Equal(t, []byte("A"), packetBody.GetRaw())
	})
}

func TestGetHttpFlowByIdOrRuntimeId(t *testing.T) {
	projectDb := consts.GetGormProjectDatabase()
	runtimeId := uuid.NewString()
	yakit.SaveHTTPFlow(projectDb, &schema.HTTPFlow{
		RuntimeId: runtimeId,
	})
	httpflow, err := yakit.GetHttpFlowByRuntimeId(projectDb, runtimeId)
	require.NoError(t, err)
	require.True(t, httpflow.RuntimeId == runtimeId)
	defer func() {
		yakit.DeleteHTTPFlow(projectDb, &ypb.DeleteHTTPFlowRequest{Id: []int64{int64(httpflow.ID)}})
	}()
	client, err2 := NewLocalClient(true)
	require.NoError(t, err2)
	_, err2 = client.GetHTTPFlowBodyById(context.Background(), &ypb.GetHTTPFlowBodyByIdRequest{
		RuntimeId: runtimeId,
	})
	require.NoError(t, err2)
}

func TestGetHttpFlowProcessName(t *testing.T) {
	projectDb := consts.GetGormProjectDatabase()
	processName := uuid.NewString()
	flow := &schema.HTTPFlow{
		Url:         "http://www.example.com",
		ProcessName: processName,
	}
	err := yakit.SaveHTTPFlow(projectDb, flow)
	require.NoError(t, err)
	require.NotEmpty(t, flow.ID)
	defer func() {
		yakit.DeleteHTTPFlow(projectDb, &ypb.DeleteHTTPFlowRequest{Id: []int64{int64(flow.ID)}})
	}()
	_, httpflows, err := yakit.QueryHTTPFlow(projectDb, &ypb.QueryHTTPFlowRequest{
		ProcessName: []string{processName},
	})
	require.NoError(t, err)
	require.Len(t, httpflows, 1)
	require.Equal(t, processName, httpflows[0].ProcessName)
}
