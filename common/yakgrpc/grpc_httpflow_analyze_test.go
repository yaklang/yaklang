package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

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

func TestMUSTPASS_AnalyzeHTTPFlow_ReplacerRule_MatchRequest(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	token := uuid.NewString()
	req := `POST /post HTTP/1.1
Host: %s
` + fmt.Sprintf(`
%s
`, token)
	err, deleteFlow := createHTTPFlow("http://www.baidu.com", req, "abc")
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
			},
		},
	})
	require.NoError(t, err)

	var (
		resultId     string
		finalProcess float64
		finalMatch   string
	)
	{
		// 测试进度条
		// 等待所有消息处理完成
		for {
			rsp, err := stream.Recv()
			if err != nil {
				// 当流结束时，err 会是 io.EOF
				break
			}
			resultId = rsp.ExecResult.GetRuntimeID()
			result := rsp.GetExecResult().GetMessage()
			var msg msg
			json.Unmarshal(result, &msg)
			if msg.Type == "progress" {
				finalProcess = msg.Content.Process
			}
			if msg.Type == "log" {
				var contentData contentData
				json.Unmarshal([]byte(msg.Content.Data), &contentData)
				if contentData.ID == "符合条件数" {
					finalMatch = contentData.Data
				}
			}

			ruleData := rsp.GetRuleData()
			if ruleData != nil {
				fmt.Println(ruleData)
			}
		}

		// 确保最终进度为1
		require.Equal(t, float64(1), finalProcess)
		require.Equal(t, "1", finalMatch)
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

func TestMUSTPASS_AnalyzeHTTPFlow_ReplacerRule_MatchResponse(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	token := uuid.NewString()
	req := `GET /get HTTP/1.1
Host: %s
`
	err, deleteFlow := createHTTPFlow("www.baidu.com", req, "HTTP/1.1 200 OK\n\n"+token+"\n"+token)
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
			},
		},
	})
	require.NoError(t, err)
	var (
		resultId       string
		finalProcess   float64
		finalMatch     string
		finalExtracted string
	)
	{
		// 测试进度条
		// 等待所有消息处理完成
		for {
			rsp, err := stream.Recv()
			if err != nil {
				// 当流结束时，err 会是 io.EOF
				break
			}
			resultId = rsp.ExecResult.GetRuntimeID()
			result := rsp.GetExecResult().GetMessage()
			var msg msg
			json.Unmarshal(result, &msg)
			if msg.Type == "progress" {
				finalProcess = msg.Content.Process
			}
			if msg.Type == "log" {
				var contentData contentData
				json.Unmarshal([]byte(msg.Content.Data), &contentData)
				if contentData.ID == "符合条件数" {
					finalMatch = contentData.Data
				}
				if contentData.ID == "提取数据" {
					finalExtracted = contentData.Data
				}
			}

			ruleData := rsp.GetRuleData()
			if ruleData != nil {
				fmt.Println(ruleData)
			}
		}

		// 确保最终进度为1
		require.Equal(t, float64(1), finalProcess)
		require.Equal(t, "1", finalMatch)
		require.Equal(t, "2", finalExtracted)
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

func TestMUSTPASS_AnalyzeHTTPFlow_MutliHTTPFlow(t *testing.T) {
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

	for _, flow := range flows {
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
			},
		},
	})
	var (
		resultId       string
		finalProcess   float64
		finalMatch     string
		finalExtracted string
	)
	{
		// 测试进度条
		// 等待所有消息处理完成
		for {
			rsp, err := stream.Recv()
			if err != nil {
				// 当流结束时，err 会是 io.EOF
				break
			}
			resultId = rsp.ExecResult.GetRuntimeID()
			result := rsp.GetExecResult().GetMessage()
			var msg msg
			json.Unmarshal(result, &msg)
			if msg.Type == "progress" {
				finalProcess = msg.Content.Process
			}
			if msg.Type == "log" {
				var contentData contentData
				json.Unmarshal([]byte(msg.Content.Data), &contentData)
				if contentData.ID == "符合条件数" {
					finalMatch = contentData.Data
				}
				if contentData.ID == "提取数据" {
					finalExtracted = contentData.Data
				}
			}

			ruleData := rsp.GetRuleData()
			if ruleData != nil {
				fmt.Println(ruleData)
			}
		}
		// 确保最终进度为1
		require.Equal(t, float64(1), finalProcess)
		require.Equal(t, "1", finalMatch)
		require.Equal(t, "1", finalExtracted)
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

func TestMUSTPASS_AnalyzeHTTPFlow_HotPatch(t *testing.T) {
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

	for _, flow := range flows {
		err, deleteFlow := createHTTPFlow(flow.url, flow.req, flow.rsp)
		require.NoError(t, err)
		defer deleteFlow()
	}

	stream, err := client.AnalyzeHTTPFlow(context.Background(), &ypb.AnalyzeHTTPFlowRequest{
		HotPatchCode: hotPatchCode,
	})
	var (
		resultId     string
		finalProcess float64
		finalMatch   string
		finalHandled string
	)
	{
		// 测试进度条
		// 等待所有消息处理完成
		for {
			rsp, err := stream.Recv()
			if err != nil {
				// 当流结束时，err 会是 io.EOF
				break
			}
			resultId = rsp.ExecResult.GetRuntimeID()
			result := rsp.GetExecResult().GetMessage()
			var msg msg
			json.Unmarshal(result, &msg)
			if msg.Type == "progress" {
				finalProcess = msg.Content.Process
			}
			if msg.Type == "log" {
				var contentData contentData
				json.Unmarshal([]byte(msg.Content.Data), &contentData)
				if contentData.ID == "符合条件数" {
					finalMatch = contentData.Data
				}
				if contentData.ID == "已处理数/总数" {
					finalHandled = contentData.Data
				}
			}

			ruleData := rsp.GetRuleData()
			if ruleData != nil {
				fmt.Println(ruleData)
			}
		}

		// 确保最终进度为1
		require.Equal(t, float64(1), finalProcess)
		require.Equal(t, "1", finalMatch)
		split := strings.Split(finalHandled, "/")
		require.Equal(t, 2, len(split))
		require.Equal(t, split[0], split[1])
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

func TestMUSTPASS_AnalyzeHTTPFlow_SourceType_Database(t *testing.T) {
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
			},
		},
		Source: &ypb.AnalyzedDataSource{
			SourceType: "database",
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

func TestMUSTPASS_AnalyzeHTTPFlow_SourceType_RawPacket(t *testing.T) {

}

func TestMUSTPASS_AnalyzeHTTPFlow_Data_Dedup(t *testing.T) {

}
