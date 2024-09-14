package yakit

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/model"
	"testing"

	"github.com/go-rod/rod/lib/utils"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestHTTPFlowToGRPCModelBase64(t *testing.T) {
	test := assert.New(t)
	reqInst, err := lowhttp.ParseBytesToHttpRequest(lowhttp.FixHTTPRequest([]byte(`POST / HTTP/1.1
Content-Type: application/x-www-form-urlencoded
Host: www.example.com
Content-Length: 9

key=MQ==`)))
	test.NoError(err, "parse request failed")
	flow, err := CreateHTTPFlowFromHTTPWithNoRspSaved(true, reqInst, "", "https://example.com", "127.0.0.1")
	test.NoError(err, "create http flow failed")
	m, err := model.ToHTTPFlowGRPCModel(flow, true)
	test.NoError(err, "convert to grpc model failed")
	for _, param := range m.PostParams {
		if param.Position == "post-query" {
			test.Equal("key", param.ParamName)
			test.Equal("MQ==", param.OriginValue)
		} else if param.Position == "post-query-base64" {
			test.Equal("key", param.ParamName)
			test.Equal("1", param.OriginValue)
		}
	}
}

//func TestYieldHTTPUrl(t *testing.T) {
//	forest := assets.NewWebsiteForest(10000)
//
//	db := consts.GetGormProjectDatabase()
//	db = db.Where("url LIKE '%baidu.com%'").Limit(10)
//	res := YieldHTTPUrl(
//		db, context.Background())
//	count := 0
//	for r := range res {
//		count++
//		println(r.Url)
//		forest.AddNode(r.Url)
//		if count > 10 {
//			break
//		}
//	}
//	raw, err := json.Marshal(forest.Output())
//	if err != nil {
//		return
//	}
//	println(string(raw))
//}
//
//func TestDeleteHTTPFlow(t *testing.T) {
//	DeleteHTTPFlow(consts.GetGormProjectDatabase().Debug(), &ypb.DeleteHTTPFlowRequest{URLPrefix: "https://github.com"})
//}
//
//func TestConvertFuzzerResponse(t *testing.T) {
//	FuzzerResponseToHTTPFlow(nil, &ypb.FuzzerResponse{
//		RequestRaw: []byte(`POST / HTTP/1.1
//Content-Type: application/json
//Host: www.example.com
//
//{"key": "value"}`),
//	})
//}

func TestHTTPFlow_Inset_FixUrl(t *testing.T) {
	token := utils.RandString(10)
	httpsFlow := &schema.HTTPFlow{
		Url: fmt.Sprintf("https://baidu.com:443?a=%s", token),
	}
	InsertHTTPFlow(consts.GetGormProjectDatabase().Debug(), httpsFlow)

	httpFlow := &schema.HTTPFlow{
		Url: fmt.Sprintf("http://baidu.com:80?a=%s", token),
	}
	InsertHTTPFlow(consts.GetGormProjectDatabase().Debug(), httpFlow)

	_, flows, err := QueryHTTPFlow(consts.GetGormProjectDatabase().Debug(), &ypb.QueryHTTPFlowRequest{Keyword: token})
	if err != nil {
		t.Fatal(err)
	}
	for _, flow := range flows {
		if flow.ID == httpsFlow.ID {
			if flow.Url != "https://baidu.com?a="+token {
				t.Fatal("insert fix https url error")
			}
			CreateOrUpdateHTTPFlow(consts.GetGormProjectDatabase().Debug(), flow.Hash, &schema.HTTPFlow{
				Url: fmt.Sprintf("https://baidu.com:443?a=%s", token),
			})
		}

		if flow.ID == httpFlow.ID {
			if flow.Url != "http://baidu.com?a="+token {
				t.Fatal("insert fix http url error")
			}
			CreateOrUpdateHTTPFlow(consts.GetGormProjectDatabase().Debug(), flow.Hash, &schema.HTTPFlow{
				Url: fmt.Sprintf("http://baidu.com:80?a=%s", token),
			})
		}
	}

	for _, flow := range flows {
		if flow.ID == httpsFlow.ID {
			if flow.Url != "https://baidu.com?a="+token {
				t.Fatal("update fix https url error")
			}
		}

		if flow.ID == httpFlow.ID {
			if flow.Url != "http://baidu.com?a="+token {
				t.Fatal("update fix http url error")
			}
		}
	}
}

func TestQueryFilterHTTPFlow(t *testing.T) {
	token := utils.RandString(10)
	jsFlow := &schema.HTTPFlow{
		Url:  fmt.Sprintf("https://example.com/%s.js", token),
		Path: fmt.Sprintf("https://example.com/%s.js", token),
	}
	InsertHTTPFlow(consts.GetGormProjectDatabase().Debug(), jsFlow)
	customFlow := &schema.HTTPFlow{
		Url:  fmt.Sprintf("https://example.com/%s", token),
		Path: fmt.Sprintf("https://example.com/%s", token),
	}
	InsertHTTPFlow(consts.GetGormProjectDatabase().Debug(), customFlow)
	_, flows, err := QueryHTTPFlow(consts.GetGormProjectDatabase().Debug(), &ypb.QueryHTTPFlowRequest{
		ExcludeSuffix: []string{".js"},
	})
	if err != nil {
		panic(err)
	}
	var flag bool
	for _, flow := range flows {
		if flow.ID == jsFlow.ID {
			panic("filter fail")
		}
		if flow.ID == customFlow.ID {
			flag = true
		}
	}
	assert.True(t, flag)
}

func TestCreateOrUpdateHTTPFlow(t *testing.T) {
	token := utils.RandString(10)
	token1 := utils.RandString(10)
	flow := &schema.HTTPFlow{
		SourceType: token,
	}
	err := InsertHTTPFlow(consts.GetGormProjectDatabase().Debug(), flow)
	require.NoError(t, err)

	defer DeleteHTTPFlowByID(consts.GetGormProjectDatabase().Debug(), int64(flow.ID))

	err = CreateOrUpdateHTTPFlow(consts.GetGormProjectDatabase().Debug(), flow.Hash, &schema.HTTPFlow{
		SourceType: token1,
	})
	require.NoError(t, err)

	newFlow, err := GetHTTPFlowByIDOrHash(consts.GetGormProjectDatabase().Debug(), int64(flow.ID), "")
	require.NoError(t, err)
	require.Equal(t, token1, newFlow.SourceType, "create or update http flow error")
}
