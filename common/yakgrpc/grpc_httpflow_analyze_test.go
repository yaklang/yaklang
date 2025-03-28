package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
)

type contentData struct {
	ID   string `json:"id"`
	Data string `json:"data"`
	Tags string `json:"tags"`
}

func createHTTPFlow(url, req, rsp string) (error, func()) {
	flow := &schema.HTTPFlow{
		Request:  req,
		Response: rsp,
		Url:      url,
	}
	err := yakit.SaveHTTPFlow(consts.GetGormProjectDatabase(), flow)
	if err != nil {
		return err, func() {}
	}
	return nil, func() {
		yakit.DeleteHTTPFlowByID(consts.GetGormProjectDatabase(), int64(flow.ID))
	}
}

func TestMUSTPASS_AnalyzeHTTPFlow(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	t.Run("test analyze http flow:regex request", func(t *testing.T) {
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
			// 测试进度条和分析结果
			for {
				rsp, err := stream.Recv()
				if err != nil {
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
	})

	t.Run("test analyze http flow :regex response", func(t *testing.T) {
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
			for {
				rsp, err := stream.Recv()
				if err != nil {
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

	})
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
		for {
			rsp, err := stream.Recv()
			if err != nil {
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
		for {
			rsp, err := stream.Recv()
			if err != nil {
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
