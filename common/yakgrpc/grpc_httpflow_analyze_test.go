package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type contentData struct {
	ID   string `json:"id"`
	Data string `json:"data"`
	Tags string `json:"tags"`
}

func createHTTPFlow(url, req, rsp string) (error, func()) {
	flow, err := yakit.CreateHTTPFlow(
		yakit.CreateHTTPFlowWithURL(url),
		yakit.CreateHTTPFlowWithRequestRaw([]byte(req)),
		yakit.CreateHTTPFlowWithFixResponseRaw([]byte(rsp)),
	)
	if err != nil {
		return err, nil
	}
	err = yakit.SaveHTTPFlow(consts.GetGormProjectDatabase(), flow)
	if err != nil {
		return err, func() {}
	}
	return nil, func() {
		yakit.DeleteHTTPFlowByID(consts.GetGormProjectDatabase(), int64(flow.ID))
	}
}

func TestGRPCMUSTPASS_AnalyzeHTTPFlow_ReplacerRule_MatchRequest(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	token := uuid.NewString()
	req := `POST /post HTTP/1.1
Host: %s
` + fmt.Sprintf(`
%s
`, token)

	url := fmt.Sprintf("http://www.baidu.com?%s", token)
	err, deleteFlow := createHTTPFlow(url, req, "abc")
	defer deleteFlow()

	require.NoError(t, err)
	ruleVerboseName := uuid.NewString()
	tag := uuid.NewString()
	color := "red"
	stream, err := client.AnalyzeHTTPFlow(context.Background(), &ypb.AnalyzeHTTPFlowRequest{
		HotPatchCode: "",
		Replacers: []*ypb.MITMContentReplacer{
			{
				EnableForRequest: true,
				Rule:             token,
				VerboseName:      ruleVerboseName,
				Color:            color,
				ExtraTag:         []string{tag},
				EnableForHeader:  true,
				EnableForBody:    true,
				NoReplace:        true,
			},
		},
		Source: &ypb.AnalyzedDataSource{
			SourceType: AnalyzeHTTPFlowSourceDatabase,
			HTTPFlowFilter: &ypb.QueryHTTPFlowRequest{
				SearchURL: url,
			},
		},
	})
	require.NoError(t, err)

	var resultId string
	{
		for {
			rsp, err := stream.Recv()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			resultId = rsp.ExecResult.GetRuntimeID()
			result := rsp.GetExecResult().GetMessage()
			var msg msg
			json.Unmarshal(result, &msg)
			ruleData := rsp.GetRuleData()
			if ruleData != nil {
				fmt.Println(ruleData)
			}
		}

	}

	var result *schema.AnalyzedHTTPFlow
	{
		results := yakit.QueryAnalyzedHTTPFlowRule(consts.GetGormProjectDatabase(), []string{resultId})
		require.NoError(t, err)
		fmt.Println(results)
		require.Equal(t, 1, len(results))
		result = results[0]
		require.Equal(t, ruleVerboseName, result.RuleVerboseName)

		httpflowId := result.HTTPFlowId
		queryFlow, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
			IncludeId: []int64{httpflowId},
		})
		require.NoError(t, err)
		require.Equal(t, 1, len(queryFlow.Data))
		fmt.Println(queryFlow.Data[0])
		// test color and tag
		require.Contains(t, queryFlow.Data[0].Tags, tag)
		require.Contains(t, queryFlow.Data[0].Tags, schema.COLORPREFIX+strings.ToUpper(color))
	}

	{
		// Query HTTPFlow by analyzed flow id
		flows, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
			AnalyzedIds: []int64{int64(result.ID)},
		})
		require.NoError(t, err)
		require.Equal(t, 1, len(flows.Data))
	}

}

func TestGRPCMUSTPASS_AnalyzeHTTPFlow_ReplacerRule_MatchResponse(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	token := uuid.NewString()
	req := `GET /get HTTP/1.1
Host: %s
`
	url := fmt.Sprintf("http://www.baidu.com?%s", token)
	err, deleteFlow := createHTTPFlow(url, req, "HTTP/1.1 200 OK\n\n"+token+"\n"+token)
	defer deleteFlow()
	require.NoError(t, err)
	ruleVerboseName := uuid.NewString()
	tag := uuid.NewString()
	color := "red"
	stream, err := client.AnalyzeHTTPFlow(context.Background(), &ypb.AnalyzeHTTPFlowRequest{
		HotPatchCode: "",
		Replacers: []*ypb.MITMContentReplacer{
			{
				EnableForBody:     true,
				EnableForResponse: true,
				Rule:              token,
				VerboseName:       ruleVerboseName,
				Color:             color,
				ExtraTag:          []string{tag},
				NoReplace:         true,
			},
		},
		Source: &ypb.AnalyzedDataSource{
			SourceType: AnalyzeHTTPFlowSourceDatabase,
			HTTPFlowFilter: &ypb.QueryHTTPFlowRequest{
				SearchURL: url,
			},
		},
	})
	require.NoError(t, err)

	var resultId string
	{
		for {
			rsp, err := stream.Recv()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			resultId = rsp.ExecResult.GetRuntimeID()
			result := rsp.GetExecResult().GetMessage()
			var msg msg
			json.Unmarshal(result, &msg)
			ruleData := rsp.GetRuleData()
			if ruleData != nil {
				fmt.Println(ruleData)
			}
		}
	}

	{
		result := yakit.QueryAnalyzedHTTPFlowRule(consts.GetGormProjectDatabase(), []string{resultId})
		require.NoError(t, err)
		fmt.Println(result)
		require.Equal(t, 1, len(result))
		require.Equal(t, ruleVerboseName, result[0].RuleVerboseName)

		httpflowId := result[0].HTTPFlowId
		queryFlow, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
			IncludeId: []int64{int64(httpflowId)},
		})
		require.NoError(t, err)
		require.Equal(t, 1, len(queryFlow.Data))
		fmt.Println(queryFlow.Data[0])
		// test color and tag
		require.Contains(t, queryFlow.Data[0].Tags, tag)
		require.Contains(t, queryFlow.Data[0].Tags, schema.COLORPREFIX+strings.ToUpper(color))
	}

}

func TestGRPCMUSTPASS_AnalyzeHTTPFlow_MutliHTTPFlow(t *testing.T) {
	urlToken := uuid.NewString()
	rspToken := uuid.NewString()

	flows := []struct {
		url string
		req string
		rsp string
	}{
		{"http://www.baidu.com", `POST /post HTTP/1.1`, "HTTP/1.1 200 OK\n\nabc\nabc" + rspToken},
		{"http://www.abc.com", `GET /get HTTP/1.1`, "HTTP/1.1 200 OK\n\nabc\nabc"},
		{"http://www.cab.com", `POST /post HTTP/1.1`, fmt.Sprintf("HTTP/1.1 200 OK\n\n%s", rspToken)},
		{fmt.Sprintf("http://www.bac%s.com", urlToken), `GET /get HTTP/1.1`, "HTTP/1.1 200 OK\n\nabc\nabc"},
		{fmt.Sprintf("http://www.cab%s.com", urlToken), `POST /post HTTP/1.1 `, "abc"},
		{fmt.Sprintf("http://www.cab%s.com", urlToken), `POST /post HTTP/1.1`, fmt.Sprintf("HTTP/1.1 200 OK\n\n%s", rspToken)},
	}

	var urls []string
	for _, flow := range flows {
		urls = append(urls, flow.url)
		err, deleteFlow := createHTTPFlow(flow.url, flow.req, flow.rsp)
		require.NoError(t, err)
		defer deleteFlow()
	}

	client, err := NewLocalClient()
	require.NoError(t, err)

	ruleVerboseName := uuid.NewString()
	tag := uuid.NewString()
	color := "red"
	stream, err := client.AnalyzeHTTPFlow(context.Background(), &ypb.AnalyzeHTTPFlowRequest{
		HotPatchCode: "",
		Replacers: []*ypb.MITMContentReplacer{
			{
				EnableForBody:     true,
				EnableForResponse: true,
				EffectiveURL:      urlToken,
				Rule:              rspToken,
				VerboseName:       ruleVerboseName,
				Color:             color,
				ExtraTag:          []string{tag},
				NoReplace:         true,
			},
		},
		Source: &ypb.AnalyzedDataSource{
			SourceType: AnalyzeHTTPFlowSourceDatabase,
			HTTPFlowFilter: &ypb.QueryHTTPFlowRequest{
				SearchURL: urlToken,
			},
		},
	})

	var resultId string
	{
		for {
			rsp, err := stream.Recv()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			resultId = rsp.ExecResult.GetRuntimeID()
			result := rsp.GetExecResult().GetMessage()
			var msg msg
			json.Unmarshal(result, &msg)
			ruleData := rsp.GetRuleData()
			if ruleData != nil {
				fmt.Println(ruleData)
			}
		}

	}
	var analyzeId int64
	{
		result := yakit.QueryAnalyzedHTTPFlowRule(consts.GetGormProjectDatabase(), []string{resultId})
		require.NoError(t, err)
		fmt.Println(result)
		require.Equal(t, 1, len(result))
		require.Equal(t, ruleVerboseName, result[0].RuleVerboseName)

		analyzeId = int64(result[0].ID)
		httpflowId := result[0].HTTPFlowId
		queryFlow, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
			IncludeId: []int64{int64(httpflowId)},
		})
		require.NoError(t, err)
		require.Equal(t, 1, len(queryFlow.Data))
		fmt.Println(queryFlow.Data[0])
		// test color and tag
		require.Contains(t, queryFlow.Data[0].Tags, tag)
		require.Contains(t, queryFlow.Data[0].Tags, schema.COLORPREFIX+strings.ToUpper(color))
	}

	{
		// Query HTTPFlow by analyzed flow id
		flows, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
			AnalyzedIds: []int64{analyzeId},
		})
		require.NoError(t, err)
		require.Equal(t, 1, len(flows.Data))
	}
}

func TestGRPCMUSTPASS_AnalyzeHTTPFlow_HotPatch(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	urlToken := uuid.NewString()
	rspToken := uuid.NewString()
	color := "Blue"
	ruleVerboseName := uuid.NewString()
	tagName := uuid.NewString()
	hotPatchCode := fmt.Sprintf(`
	analyzeHTTPFlow = func(flow,extract){
    if str.Contains(flow.Url, "%s")  && str.Contains(string(flow.Response),"%s"){
        flow.%s()
		flow.AddTag("%s")
		extract("%s",flow)
    }
}
	`, urlToken, rspToken, color, tagName, ruleVerboseName)

	flows := []struct {
		url string
		req string
		rsp string
	}{
		{"http://www.baidu.com", `POST /post HTTP/1.1`, "HTTP/1.1 200 OK\n\nabc\nabc" + rspToken},
		{"http://www.abc.com", `GET /get HTTP/1.1`, "HTTP/1.1 200 OK\n\nabc\nabc"},
		{"http://www.cab.com", `POST /post HTTP/1.1`, fmt.Sprintf("HTTP/1.1 200 OK\n\n%s", rspToken)},
		{fmt.Sprintf("http://www.bac%s.com", urlToken), `GET /get HTTP/1.1`, "HTTP/1.1 200 OK\n\nabc\nabc"},
		{fmt.Sprintf("http://www.cab%s.com", urlToken), `POST /post HTTP/1.1 `, "abc"},
		{fmt.Sprintf("http://www.cab%s.com", urlToken), `POST /post HTTP/1.1`, fmt.Sprintf("HTTP/1.1 200 OK\n\n%s", rspToken)},
	}

	var urls []string
	for _, flow := range flows {
		urls = append(urls, flow.url)
		err, deleteFlow := createHTTPFlow(flow.url, flow.req, flow.rsp)
		require.NoError(t, err)
		defer deleteFlow()
	}

	stream, err := client.AnalyzeHTTPFlow(context.Background(), &ypb.AnalyzeHTTPFlowRequest{
		HotPatchCode: hotPatchCode,
	})

	var resultId string
	{
		for {
			rsp, err := stream.Recv()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			resultId = rsp.ExecResult.GetRuntimeID()
			result := rsp.GetExecResult().GetMessage()
			var msg msg
			json.Unmarshal(result, &msg)
			ruleData := rsp.GetRuleData()
			if ruleData != nil {
				fmt.Println(ruleData)
			}
		}

	}

	{
		result := yakit.QueryAnalyzedHTTPFlowRule(consts.GetGormProjectDatabase(), []string{resultId})
		require.NoError(t, err)
		fmt.Println(result)
		require.Equal(t, 1, len(result))
		require.Equal(t, ruleVerboseName, result[0].RuleVerboseName)

		httpflowId := result[0].HTTPFlowId
		queryFlow, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
			IncludeId: []int64{int64(httpflowId)},
		})
		require.NoError(t, err)
		require.Equal(t, 1, len(queryFlow.Data))
		fmt.Println(queryFlow.Data[0])
		// test color and tag
		require.Contains(t, queryFlow.Data[0].Tags, schema.COLORPREFIX+strings.ToUpper(color))
	}
}

func TestGRPCMUSTPASS_AnalyzeHTTPFlow_SourceType_Database(t *testing.T) {
	token := uuid.NewString()
	flows := []struct {
		url string
		req string
		rsp string
	}{
		// URL和响应体都有token的请求
		{"http://www.example.com/search?token=" + token, `GET /search HTTP/1.1`, "HTTP/1.1 200 OK\n\n{\"access_token\":\"" + token + "\",\"expires_in\":3600}"},
		{"http://www.api.com/data?auth=" + token, `GET /data HTTP/1.1`, "HTTP/1.1 200 OK\n\n{\"token\":\"" + token + "\",\"type\":\"bearer\"}"},

		// 只有URL有token的请求
		{"http://www.secure.com/profile?session=" + token, `GET /profile HTTP/1.1`, "HTTP/1.1 200 OK\n\nprofile data"},
		{"http://www.user.com/info?access_token=" + token, `GET /info HTTP/1.1`, "HTTP/1.1 200 OK\n\nuser info"},

		// 只有响应体有token的请求
		{"http://www.token.com", `GET /token HTTP/1.1`, "HTTP/1.1 200 OK\n\n{\"jwt\":\"" + token + "\",\"valid\":true}"},

		// 只有请求体有token的请求
		{"http://www.auth.com", `POST /auth HTTP/1.1
Content-Type: application/json

{"token":"` + token + `","username":"admin"}`, "HTTP/1.1 200 OK\n\nauth success"},
	}
	for _, flow := range flows {
		err, deleteFlow := createHTTPFlow(flow.url, flow.req, flow.rsp)
		require.NoError(t, err)
		t.Cleanup(func() {
			deleteFlow()
		})
	}

	client, err := NewLocalClient()
	require.NoError(t, err)

	ruleVerboseName := uuid.NewString()
	tag := uuid.NewString()
	color := "red"
	stream, err := client.AnalyzeHTTPFlow(context.Background(), &ypb.AnalyzeHTTPFlowRequest{
		HotPatchCode: "",
		Replacers: []*ypb.MITMContentReplacer{
			{
				EnableForBody:     true,
				EnableForResponse: true,
				Rule:              token, // 匹配响应体有token
				VerboseName:       ruleVerboseName,
				Color:             color,
				ExtraTag:          []string{tag},
				NoReplace:         true,
			},
		},
		Source: &ypb.AnalyzedDataSource{
			SourceType: AnalyzeHTTPFlowSourceDatabase,
			HTTPFlowFilter: &ypb.QueryHTTPFlowRequest{
				SearchURL: token, // 匹配url有token
			},
		},
	})
	require.NoError(t, err)

	var (
		resultId string
	)
	{
		for {
			rsp, err := stream.Recv()
			if err != nil {
				// 当流结束时，err 会是 io.EOF
				break
			}
			resultId = rsp.ExecResult.GetRuntimeID()
			ruleData := rsp.GetRuleData()
			if ruleData != nil {
				fmt.Println(ruleData)
			}
		}
	}

	var analyzeIds []int64
	{
		results := yakit.QueryAnalyzedHTTPFlowRule(consts.GetGormProjectDatabase(), []string{resultId})
		require.NoError(t, err)
		fmt.Println(results)
		require.Equal(t, 2, len(results))
		for _, result := range results {
			require.Equal(t, ruleVerboseName, result.RuleVerboseName)
			analyzeIds = append(analyzeIds, int64(result.ID))
		}
	}

	{
		// Query HTTPFlow by analyzed flow id
		queryHTTPFlows, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
			AnalyzedIds: analyzeIds,
		})
		require.NoError(t, err)
		require.Equal(t, 2, len(queryHTTPFlows.Data))
	}
}

func TestGRPCMUSTPASS_AnalyzeHTTPFlow_SourceType_RawPacket(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	token := uuid.NewString()
	req := fmt.Sprintf(`GET /get HTTP/1.1
Host: 127.0.0.1
Connection: keep-alive
Cookie: %s
`, token)
	rsp := fmt.Sprintf(`HTTP/1.1 200 OK
%s`, token)

	color := "green"
	stream, err := client.AnalyzeHTTPFlow(context.Background(), &ypb.AnalyzeHTTPFlowRequest{
		Replacers: []*ypb.MITMContentReplacer{
			{
				EnableForBody:     true,
				EnableForResponse: true,
				EnableForHeader:   true,
				EnableForRequest:  true,
				Rule:              token,
				VerboseName:       token,
				Color:             color,
				NoReplace:         true,
			},
		},
		Source: &ypb.AnalyzedDataSource{
			SourceType:  AnalyzeHTTPFlowSourceRawPacket,
			RawRequest:  req,
			RawResponse: rsp,
		},
	})
	require.NoError(t, err)

	var analyzedIds []int64
	{
		for {
			rsp, err := stream.Recv()
			if err != nil {
				break
			}
			ruleData := rsp.GetRuleData()
			analyzedId := ruleData.GetId()
			analyzedIds = append(analyzedIds, analyzedId)
			if ruleData != nil {
				fmt.Println(ruleData)
			}
		}
	}

	flows, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
		AnalyzedIds: analyzedIds,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(flows.Data))
	require.Equal(t, flows.Data[0].Tags, schema.COLORPREFIX+strings.ToUpper(color))
	_, err = client.DeleteHTTPFlows(context.Background(), &ypb.DeleteHTTPFlowRequest{
		Id: []int64{int64(flows.Data[0].Id)},
	})
	require.NoError(t, err)
}

func TestGRPCMUSTPASS_AnalyzeHTTPFlow_Data_Dedup(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	token := uuid.NewString()
	req := `GET /get HTTP/1.1
Host: 127.0.0.1
Connection: keep-alive
`
	rsp := fmt.Sprintf(`HTTP/1.1 200 OK
%s 
%s
`, token, token)

	color := "green"
	stream, err := client.AnalyzeHTTPFlow(context.Background(), &ypb.AnalyzeHTTPFlowRequest{
		Replacers: []*ypb.MITMContentReplacer{
			{
				EnableForBody:     true,
				EnableForResponse: true,
				EnableForHeader:   true,
				Rule:              token,
				VerboseName:       token,
				Color:             color,
				NoReplace:         true,
			},
		},
		Source: &ypb.AnalyzedDataSource{
			SourceType:  AnalyzeHTTPFlowSourceRawPacket,
			RawRequest:  req,
			RawResponse: rsp,
		},
		Config: &ypb.AnalyzeHTTPFlowConfig{
			EnableDeduplicate: true,
		},
	})
	require.NoError(t, err)

	var analyzedIds []int64
	var ruleDatas []*ypb.HTTPFlowRuleData
	{
		for {
			rsp, err := stream.Recv()
			if err != nil {
				break
			}
			ruleData := rsp.GetRuleData()
			if ruleData != nil {
				ruleDatas = append(ruleDatas, ruleData)
				analyzedId := ruleData.GetId()
				analyzedIds = append(analyzedIds, analyzedId)
				fmt.Println(ruleData)
			}
		}
	}

	// 测试去重
	require.Equal(t, 1, len(ruleDatas))

	flows, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
		AnalyzedIds: analyzedIds,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(flows.Data))
	flow := flows.Data[0]
	require.Equal(t, flow.Tags, schema.COLORPREFIX+strings.ToUpper(color))
	defer func() {
		_, err = client.DeleteHTTPFlows(context.Background(), &ypb.DeleteHTTPFlowRequest{
			Id: []int64{int64(flow.Id)},
		})
		require.NoError(t, err)
	}()
}

func TestGRPCMUSTPASS_AnalyzeHTTPFlow_WebSocketFlow(t *testing.T) {
	// create websocket flow
	ctx, cancel := context.WithCancel(context.Background())
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

	ruleVerboseName := uuid.NewString()
	color := "red"
	tag := uuid.NewString()

	analyzeStream, err := client.AnalyzeHTTPFlow(context.Background(), &ypb.AnalyzeHTTPFlowRequest{
		Replacers: []*ypb.MITMContentReplacer{
			{
				EnableForBody:     true,
				EnableForResponse: true,
				Rule:              token,
				VerboseName:       ruleVerboseName,
				Color:             color,
				ExtraTag:          []string{tag},
				NoReplace:         true,
			},
		},
		Source: &ypb.AnalyzedDataSource{
			SourceType: "database",
			HTTPFlowFilter: &ypb.QueryHTTPFlowRequest{
				IncludeId: []int64{int64(flow.Id)},
			},
		},
	})
	require.NoError(t, err)

	{

		for {
			rsp, err := analyzeStream.Recv()
			if err != nil {
				break
			}
			ruleData := rsp.GetRuleData()
			if ruleData != nil {
				fmt.Println(ruleData)
			}
		}
	}

	var wsFlows []*schema.WebsocketFlow
	wsFlows, err = yakit.QueryAllWebsocketFlowByWebsocketHash(consts.GetGormProjectDatabase(), hash)
	spew.Dump(wsFlows)
	colorCount := 0
	tagCount := 0
	for _, wsFlow := range wsFlows {
		if strings.Contains(wsFlow.Tags, schema.COLORPREFIX+strings.ToUpper(color)) {
			colorCount++
		}
		if strings.Contains(wsFlow.Tags, tag) {
			tagCount++
		}
	}
	require.Greater(t, tagCount, 0)
	require.Greater(t, colorCount, 0)
}

func TestGRPCMUSTPASS_AnalyzeHTTPFlow_SessionKeyRegex(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	token := uuid.NewString()
	req := `GET /api/login HTTP/1.1
Host: api.example.com
Content-Type: application/json
`

	rsp := `HTTP/1.1 200 OK
Content-Type: application/json
Date: Thu, 26 Jun 2025 06:30:36 GMT
Content-Length: 116

{"code":200,"message":"success","user":{"openid":"user12345","session_id":"sess_abcdefg12345","session_key":"aaa"}}`

	url := fmt.Sprintf("https://api.example.com/login?%s", token)
	err, deleteFlow := createHTTPFlow(url, req, rsp)
	defer deleteFlow()
	require.NoError(t, err)

	ruleVerboseName := uuid.NewString()
	tag := uuid.NewString()
	color := "purple"
	regexRule := `(?i)\"session[_]?key\"`

	stream, err := client.AnalyzeHTTPFlow(context.Background(), &ypb.AnalyzeHTTPFlowRequest{
		HotPatchCode: "",
		Replacers: []*ypb.MITMContentReplacer{
			{
				EnableForBody:     true,
				EnableForResponse: true,
				Rule:              regexRule,
				VerboseName:       ruleVerboseName,
				Color:             color,
				ExtraTag:          []string{tag},
				NoReplace:         true,
			},
		},
		Source: &ypb.AnalyzedDataSource{
			SourceType: AnalyzeHTTPFlowSourceDatabase,
			HTTPFlowFilter: &ypb.QueryHTTPFlowRequest{
				SearchURL: url,
			},
		},
	})
	require.NoError(t, err)

	var resultId string
	{
		for {
			rsp, err := stream.Recv()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			resultId = rsp.ExecResult.GetRuntimeID()
			result := rsp.GetExecResult().GetMessage()
			var msg msg
			json.Unmarshal(result, &msg)
			ruleData := rsp.GetRuleData()
			if ruleData != nil {
				fmt.Printf("Found session_key match: %+v\n", ruleData)
			}
		}
	}

	var result *schema.AnalyzedHTTPFlow
	{
		results := yakit.QueryAnalyzedHTTPFlowRule(consts.GetGormProjectDatabase(), []string{resultId})
		require.NoError(t, err)
		fmt.Println(results)
		require.Equal(t, 1, len(results))
		result = results[0]
		require.Equal(t, ruleVerboseName, result.RuleVerboseName)

		httpflowId := result.HTTPFlowId
		queryFlow, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
			IncludeId: []int64{httpflowId},
		})
		require.NoError(t, err)
		require.Equal(t, 1, len(queryFlow.Data))
		flow := queryFlow.Data[0]
		fmt.Printf("Analyzed flow: %+v\n", flow)

		// 验证color和tag是否正确设置
		require.Contains(t, flow.Tags, tag, "Flow should contain the specified tag")
		require.Contains(t, flow.Tags, schema.COLORPREFIX+strings.ToUpper(color), "Flow should contain the color tag")

		// 验证响应体确实包含session_key
		require.Contains(t, string(flow.Response), "session_key", "Response should contain session_key")
		require.Contains(t, string(flow.Response), `"session_key":"aaa"`, "Response should contain the exact session_key value")
	}

	{
		// 通过analyzed flow id查询HTTPFlow
		flows, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
			AnalyzedIds: []int64{int64(result.ID)},
		})
		require.NoError(t, err)
		require.Equal(t, 1, len(flows.Data))

		t.Logf("Successfully matched session_key with regex: %s", regexRule)
		t.Logf("Flow tags: %s", flows.Data[0].Tags)
	}
}

func TestGRPCMUSTPASS_AnalyzeHTTPFlow_OnFinishHotPatch(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	urlToken := uuid.NewString()
	rspToken := uuid.NewString()

	tempDir := t.TempDir()
	tempFile := path.Join(tempDir, "test_analyze_result.txt")

	hotPatchCode := fmt.Sprintf(`
	tempFile =<<<URL
%s
URL
	f = file.OpenFile(tempFile, file.O_APPEND|file.O_CREATE|file.O_RDWR, 0o777)~
	m = sync.NewMutex()
	
	analyzeHTTPFlow = func(flow, extract) {
		if str.Contains(flow.Url, "%s") && str.Contains(string(flow.Response), "%s") {
			m.Lock()
			f.WriteLine(sprintf("MATCHED: %%s", flow.Url))
			m.Unlock()
			extract("test_rule", flow, "matched_data")
		}
	}
	
	onAnalyzeHTTPFlowFinish = func(totalCount, matchedCount) {
		m.Lock()
		f.WriteLine(sprintf("FINISH: totalCount=%%d, matchedCount=%%d", totalCount, matchedCount))
		m.Unlock()
		f.Close()
	}
	`, tempFile, urlToken, rspToken)

	flows := []struct {
		url string
		req string
		rsp string
	}{
		{fmt.Sprintf("http://www.test%s.com", urlToken), `GET /test HTTP/1.1`, fmt.Sprintf("HTTP/1.1 200 OK\n\n%s", rspToken)},
		{fmt.Sprintf("http://www.test%s.com/2", urlToken), `POST /test HTTP/1.1`, fmt.Sprintf("HTTP/1.1 200 OK\n\n%s", rspToken)},
		{"http://www.other.com", `GET /other HTTP/1.1`, "HTTP/1.1 200 OK\n\nother"},
	}

	for _, flow := range flows {
		err, deleteFlow := createHTTPFlow(flow.url, flow.req, flow.rsp)
		require.NoError(t, err)
		defer deleteFlow()
	}

	stream, err := client.AnalyzeHTTPFlow(context.Background(), &ypb.AnalyzeHTTPFlowRequest{
		HotPatchCode: hotPatchCode,
	})
	require.NoError(t, err)

	var resultId string

	for {
		rsp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		if rsp.ExecResult != nil {
			resultId = rsp.ExecResult.GetRuntimeID()
		}
	}

	time.Sleep(2 * time.Second)
	stat, err := os.Stat(tempFile)
	t.Logf("Exists before reading: %v", stat)
	content, err := os.ReadFile(tempFile)
	require.NoError(t, err)

	contentStr := string(content)
	t.Log("file content:")
	t.Log(contentStr)
	lines := strings.Split(contentStr, "\n")

	matchedCount := 0
	var finishFound bool
	var totalCount, matchedCountFromFinish int64

	for _, line := range lines {
		if strings.Contains(line, "MATCHED:") {
			matchedCount++
			require.True(t, strings.Contains(line, urlToken), "Matched URL should contain token")
		}
		if strings.Contains(line, "FINISH:") {
			finishFound = true
			if strings.Contains(line, "totalCount=") && strings.Contains(line, "matchedCount=") {
				fmt.Sscanf(line, "FINISH: totalCount=%d, matchedCount=%d", &totalCount, &matchedCountFromFinish)
			}
		}
	}

	require.Equal(t, 2, matchedCount, "Should have 2 matched flows")
	require.True(t, finishFound, "Should have finish callback")
	require.Equal(t, int64(2), matchedCountFromFinish, "Should have matched count of 2")

	result := yakit.QueryAnalyzedHTTPFlowRule(consts.GetGormProjectDatabase(), []string{resultId})
	require.Equal(t, 2, len(result), "Should have 2 matched flows in database")
	t.Logf("Successfully tested onAnalyzeHTTPFlowFinish callback")
	t.Logf("File content: %s", contentStr)
}
