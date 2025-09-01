package yakit

import (
	"database/sql"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/segmentio/ksuid"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/model"

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

func TestQueryHttpFlowFromPlugin(t *testing.T) {
	hash := uuid.NewString()
	httpflow := &schema.HTTPFlow{
		FromPlugin: "abcabc",
	}
	err := CreateOrUpdateHTTPFlow(consts.GetGormProjectDatabase().Debug(), hash, httpflow)
	require.NoError(t, err)
	defer func() {
		DeleteHTTPFlowByID(consts.GetGormProjectDatabase().Debug(), int64(httpflow.ID))
	}()
	paging, httpflows, err := QueryHTTPFlow(consts.GetGormProjectDatabase().Debug(), &ypb.QueryHTTPFlowRequest{
		FromPlugin: "abc",
	})
	require.NoError(t, err)
	require.True(t, paging.TotalRecord != 0)
	var flag bool
	for _, httpflow := range httpflows {
		if httpflow.FromPlugin == "abcabc" {
			flag = true
		}
	}
	require.True(t, flag)
}

func TestHTTPFlow_StatusCode(t *testing.T) {
	token := utils.RandString(10)
	var ids []int64
	defer func() {
		if len(ids) > 0 {
			DeleteHTTPFlow(consts.GetGormProjectDatabase(), &ypb.DeleteHTTPFlowRequest{
				Id: ids,
			})
		}
	}()
	for i := 200; i < 205; i++ {
		httpsFlow := &schema.HTTPFlow{
			Url:        fmt.Sprintf("https://exxample.com:443?a=%s", token),
			StatusCode: int64(i),
		}
		err := InsertHTTPFlow(consts.GetGormProjectDatabase(), httpsFlow)
		require.NoError(t, err)
		ids = append(ids, int64(httpsFlow.ID))
	}

	t.Run("number", func(t *testing.T) {
		_, flows, err := QueryHTTPFlow(
			consts.GetGormProjectDatabase(),
			&ypb.QueryHTTPFlowRequest{
				Keyword:    token,
				StatusCode: "200",
			})
		require.NoError(t, err)
		require.Len(t, flows, 1)
	})

	t.Run("number range", func(t *testing.T) {
		_, flows, err := QueryHTTPFlow(
			consts.GetGormProjectDatabase(),
			&ypb.QueryHTTPFlowRequest{
				Keyword:    token,
				StatusCode: "200-204",
			})
		require.NoError(t, err)
		require.Len(t, flows, 5)
	})

	t.Run("number range and number", func(t *testing.T) {
		_, flows, err := QueryHTTPFlow(
			consts.GetGormProjectDatabase(),
			&ypb.QueryHTTPFlowRequest{
				Keyword:    token,
				StatusCode: "200-203,204",
			})
		require.NoError(t, err)
		require.Len(t, flows, 5)
	})

	t.Run("prefix with -", func(t *testing.T) {
		_, flows, err := QueryHTTPFlow(
			consts.GetGormProjectDatabase(),
			&ypb.QueryHTTPFlowRequest{
				Keyword:    token,
				StatusCode: "-200",
			})
		require.NoError(t, err)
		require.Len(t, flows, 0)
	})

	t.Run("suffix with -", func(t *testing.T) {
		_, flows, err := QueryHTTPFlow(
			consts.GetGormProjectDatabase(),
			&ypb.QueryHTTPFlowRequest{
				Keyword:    token,
				StatusCode: "200-",
			})
		require.NoError(t, err)
		require.Len(t, flows, 0)
	})

	// Test ExcludeStatusCode field
	t.Run("exclude single status code", func(t *testing.T) {
		_, flows, err := QueryHTTPFlow(
			consts.GetGormProjectDatabase(),
			&ypb.QueryHTTPFlowRequest{
				Keyword:           token,
				ExcludeStatusCode: "200",
			})
		require.NoError(t, err)
		require.Len(t, flows, 4) // Should exclude 200, keep 201,202,203,204
		for _, flow := range flows {
			require.NotEqual(t, int64(200), flow.StatusCode)
		}
	})

	t.Run("exclude multiple status codes", func(t *testing.T) {
		_, flows, err := QueryHTTPFlow(
			consts.GetGormProjectDatabase(),
			&ypb.QueryHTTPFlowRequest{
				Keyword:           token,
				ExcludeStatusCode: "200,201,202",
			})
		require.NoError(t, err)
		require.Len(t, flows, 2) // Should exclude 200,201,202, keep 203,204
		for _, flow := range flows {
			require.NotContains(t, []int64{200, 201, 202}, flow.StatusCode)
		}
	})

	t.Run("exclude status code range", func(t *testing.T) {
		_, flows, err := QueryHTTPFlow(
			consts.GetGormProjectDatabase(),
			&ypb.QueryHTTPFlowRequest{
				Keyword:           token,
				ExcludeStatusCode: "200-202",
			})
		require.NoError(t, err)
		require.Len(t, flows, 2) // Should exclude 200,201,202, keep 203,204
		for _, flow := range flows {
			require.NotContains(t, []int64{200, 201, 202}, flow.StatusCode)
		}
	})

	t.Run("exclude range and single code", func(t *testing.T) {
		_, flows, err := QueryHTTPFlow(
			consts.GetGormProjectDatabase(),
			&ypb.QueryHTTPFlowRequest{
				Keyword:           token,
				ExcludeStatusCode: "200-201,204",
			})
		require.NoError(t, err)
		require.Len(t, flows, 2) // Should exclude 200,201,204, keep 202,203
		for _, flow := range flows {
			require.NotContains(t, []int64{200, 201, 204}, flow.StatusCode)
		}
	})

	t.Run("exclude all status codes", func(t *testing.T) {
		_, flows, err := QueryHTTPFlow(
			consts.GetGormProjectDatabase(),
			&ypb.QueryHTTPFlowRequest{
				Keyword:           token,
				ExcludeStatusCode: "200-204",
			})
		require.NoError(t, err)
		require.Len(t, flows, 0) // Should exclude all
	})

	t.Run("exclude non-existent status code", func(t *testing.T) {
		_, flows, err := QueryHTTPFlow(
			consts.GetGormProjectDatabase(),
			&ypb.QueryHTTPFlowRequest{
				Keyword:           token,
				ExcludeStatusCode: "404",
			})
		require.NoError(t, err)
		require.Len(t, flows, 5) // Should exclude nothing, keep all 200-204
	})

	t.Run("exclude with prefix -", func(t *testing.T) {
		_, flows, err := QueryHTTPFlow(
			consts.GetGormProjectDatabase(),
			&ypb.QueryHTTPFlowRequest{
				Keyword:           token,
				ExcludeStatusCode: "-200",
			})
		require.NoError(t, err)
		require.Len(t, flows, 5) // Invalid format should return normal results
	})

	t.Run("exclude with suffix -", func(t *testing.T) {
		_, flows, err := QueryHTTPFlow(
			consts.GetGormProjectDatabase(),
			&ypb.QueryHTTPFlowRequest{
				Keyword:           token,
				ExcludeStatusCode: "200-",
			})
		require.NoError(t, err)
		require.Len(t, flows, 5) // Invalid format should return normal results
	})

	t.Run("exclude invalid format", func(t *testing.T) {
		_, flows, err := QueryHTTPFlow(
			consts.GetGormProjectDatabase(),
			&ypb.QueryHTTPFlowRequest{
				Keyword:           token,
				ExcludeStatusCode: "abc",
			})
		require.NoError(t, err)
		require.Len(t, flows, 5) // Invalid format should return normal results
	})

	t.Run("exclude combined with include status code", func(t *testing.T) {
		_, flows, err := QueryHTTPFlow(
			consts.GetGormProjectDatabase(),
			&ypb.QueryHTTPFlowRequest{
				Keyword:           token,
				StatusCode:        "200-204",
				ExcludeStatusCode: "202",
			})
		require.NoError(t, err)
		require.Len(t, flows, 4) // Include 200-204, exclude 202, should get 200,201,203,204
		for _, flow := range flows {
			require.NotEqual(t, int64(202), flow.StatusCode)
			require.Contains(t, []int64{200, 201, 203, 204}, flow.StatusCode)
		}
	})

}

func TestQueryHTTPFlowsProcessNames(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		processNames := make([]string, 0, 5)
		ids := make([]int64, 0, 5)
		defer func() {
			if len(ids) > 0 {
				DeleteHTTPFlow(consts.GetGormProjectDatabase(), &ypb.DeleteHTTPFlowRequest{
					Id: ids,
				})
			}
		}()
		for i := 0; i < 5; i++ {
			processName := utils.RandString(16)
			processNames = append(processNames, processName)
			flow := &schema.HTTPFlow{
				Url: uuid.NewString(),
				ProcessName: sql.NullString{
					String: processName,
					Valid:  true,
				},
			}
			err := InsertHTTPFlow(consts.GetGormProjectDatabase(), flow)
			require.NoError(t, err)
			require.NotEmpty(t, flow.ID)
			ids = append(ids, int64(flow.ID))
		}

		db := consts.GetGormProjectDatabase()
		got, err := QueryHTTPFlowsProcessNames(db, &ypb.QueryHTTPFlowRequest{
			ProcessName: processNames,
		})
		require.NoError(t, err)
		require.ElementsMatch(t, processNames, got)
	})

	t.Run("distinct", func(t *testing.T) {
		processNames := make([]string, 0, 5)
		ids := make([]int64, 0, 5)
		defer func() {
			if len(ids) > 0 {
				DeleteHTTPFlow(consts.GetGormProjectDatabase(), &ypb.DeleteHTTPFlowRequest{
					Id: ids,
				})
			}
		}()
		processName := utils.RandString(16)
		for i := 0; i < 5; i++ {
			processNames = append(processNames, processName)
			flow := &schema.HTTPFlow{
				Url: uuid.NewString(),
				ProcessName: sql.NullString{
					String: processName,
					Valid:  true,
				},
			}
			err := InsertHTTPFlow(consts.GetGormProjectDatabase(), flow)
			require.NoError(t, err)
			require.NotEmpty(t, flow.ID)
			ids = append(ids, int64(flow.ID))
		}

		db := consts.GetGormProjectDatabase()
		got, err := QueryHTTPFlowsProcessNames(db, &ypb.QueryHTTPFlowRequest{
			ProcessName: append(processNames, processNames...),
		})
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, processName, got[0])
	})

	t.Run("Empty and Null", func(t *testing.T) {
		runtimeID := uuid.NewString()
		ids := make([]int64, 0, 2)
		defer DeleteHTTPFlow(consts.GetGormProjectDatabase(), &ypb.DeleteHTTPFlowRequest{
			Id: ids,
		})
		flow := &schema.HTTPFlow{
			Url:       uuid.NewString(),
			RuntimeId: runtimeID,
			ProcessName: sql.NullString{
				String: "",
				Valid:  true,
			},
		}
		err := InsertHTTPFlow(consts.GetGormProjectDatabase(), flow)
		require.NoError(t, err)
		require.NotEmpty(t, flow.ID)
		ids = append(ids, int64(flow.ID))
		flow2 := &schema.HTTPFlow{
			Url:       uuid.NewString(),
			RuntimeId: runtimeID,
			ProcessName: sql.NullString{
				Valid: false,
			},
		}
		err = InsertHTTPFlow(consts.GetGormProjectDatabase(), flow2)
		require.NoError(t, err)
		require.NotEmpty(t, flow2.ID)
		ids = append(ids, int64(flow2.ID))

		db := consts.GetGormProjectDatabase()
		got, err := QueryHTTPFlowsProcessNames(db, &ypb.QueryHTTPFlowRequest{
			RuntimeId: runtimeID,
		})
		require.NoError(t, err)
		require.Len(t, got, 0)
	})
}

func TestColorFilter(t *testing.T) {
	token := ksuid.New().String()
	colorToken := ksuid.New().String()
	jsFlow := &schema.HTTPFlow{
		Url:  fmt.Sprintf("https://example.com/%s.js", token),
		Path: fmt.Sprintf("https://example.com/%s.js", token),
		Tags: fmt.Sprintf("SQL注入测试点|%s|SQL注入测试点", colorToken),
	}
	InsertHTTPFlow(consts.GetGormProjectDatabase(), jsFlow)
	db := FilterHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{
		Color: []string{colorToken},
	})
	res := []*schema.HTTPFlow{}
	db.Find(&res)
	assert.Len(t, res, 1)
}

func TestExcludeKeywords(t *testing.T) {
	sameToken, token := uuid.NewString(), uuid.NewString()
	ids := make([]int64, 0, 2)
	defer DeleteHTTPFlow(consts.GetGormProjectDatabase(), &ypb.DeleteHTTPFlowRequest{
		Id: ids,
	})
	flow := &schema.HTTPFlow{
		Path: token,
		Url:  sameToken,
	}
	err := InsertHTTPFlow(consts.GetGormProjectDatabase(), flow)
	require.NoError(t, err)
	require.Greater(t, flow.ID, uint(0))
	ids = append(ids, int64(flow.ID))
	flow2 := &schema.HTTPFlow{
		Url: sameToken,
	}
	err = InsertHTTPFlow(consts.GetGormProjectDatabase(), flow2)
	require.NoError(t, err)
	require.Greater(t, flow.ID, uint(0))
	ids = append(ids, int64(flow.ID))

	db := consts.GetGormProjectDatabase().Debug()
	start := time.Now()
	_, httpflows, err := QueryHTTPFlow(db, &ypb.QueryHTTPFlowRequest{
		Keyword:         sameToken,
		ExcludeKeywords: []string{token},
	})
	t.Logf("query with exclude keywords cost: %v", time.Since(start))
	require.NoError(t, err)
	require.Len(t, httpflows, 1)
}

func TestFilterHTTPFlowByDomain(t *testing.T) {
	db := consts.GetGormProjectDatabase()

	// 创建测试数据
	token := utils.RandString(10)
	var ids []int64

	testFlows := []*schema.HTTPFlow{
		{Url: fmt.Sprintf("https://baidu.com/%s", token)},
		{Url: fmt.Sprintf("http://127.0.0.1:8080/%s", token)},
		{Url: fmt.Sprintf("https://www.google.com/%s", token)},
	}

	// 插入测试数据
	for _, flow := range testFlows {
		err := InsertHTTPFlow(db, flow)
		require.NoError(t, err)
		ids = append(ids, int64(flow.ID))
	}

	// 清理测试数据
	defer func() {
		if len(ids) > 0 {
			DeleteHTTPFlow(db, &ypb.DeleteHTTPFlowRequest{Id: ids})
		}
	}()

	// 测试模糊匹配功能
	t.Run("模糊匹配 - du 匹配 baidu.com", func(t *testing.T) {
		filteredDB := FilterHTTPFlowByDomain(db.Where("url LIKE ?", "%"+token+"%"), "du")
		var results []*schema.HTTPFlow
		err := filteredDB.Find(&results).Error
		require.NoError(t, err)
		require.Len(t, results, 1, "应该匹配到1个包含'du'的域名")

		parsedURL, err := url.Parse(results[0].Url)
		require.NoError(t, err)
		require.Equal(t, "baidu.com", parsedURL.Host, "应该匹配到baidu.com")
	})

	t.Run("模糊匹配 - 127 匹配 127.0.0.1", func(t *testing.T) {
		filteredDB := FilterHTTPFlowByDomain(db.Where("url LIKE ?", "%"+token+"%"), "127")
		var results []*schema.HTTPFlow
		err := filteredDB.Find(&results).Error
		require.NoError(t, err)
		require.Len(t, results, 1, "应该匹配到1个包含'127'的IP地址")

		parsedURL, err := url.Parse(results[0].Url)
		require.NoError(t, err)
		require.Equal(t, "127.0.0.1:8080", parsedURL.Host, "应该匹配到127.0.0.1:8080")
	})

	t.Run("模糊匹配 - google 匹配 www.google.com", func(t *testing.T) {
		filteredDB := FilterHTTPFlowByDomain(db.Where("url LIKE ?", "%"+token+"%"), "google")
		var results []*schema.HTTPFlow
		err := filteredDB.Find(&results).Error
		require.NoError(t, err)
		require.Len(t, results, 1, "应该匹配到1个包含'google'的域名")

		parsedURL, err := url.Parse(results[0].Url)
		require.NoError(t, err)
		require.Equal(t, "www.google.com", parsedURL.Host, "应该匹配到www.google.com")
	})
}

func TestKeywordType(t *testing.T) {

	db := consts.GetGormProjectDatabase()
	insertFlowAndCheck := func(flow *schema.HTTPFlow) int64 {
		err := InsertHTTPFlow(db, flow)
		require.NoError(t, err)
		require.Greater(t, flow.ID, uint(0))
		return int64(flow.ID)
	}

	t.Run("all", func(t *testing.T) {
		t.Parallel()

		ids := make([]int64, 0, 2)
		t.Cleanup(func() {
			DeleteHTTPFlow(consts.GetGormProjectDatabase(), &ypb.DeleteHTTPFlowRequest{
				Id: ids,
			})
		})

		token := uuid.NewString()
		ids = append(ids, insertFlowAndCheck(&schema.HTTPFlow{
			Path: token,
		}))
		ids = append(ids, insertFlowAndCheck(&schema.HTTPFlow{
			Url: token,
		}))

		_, httpflows, err := QueryHTTPFlow(db, &ypb.QueryHTTPFlowRequest{
			Keyword:     token,
			KeywordType: "all",
		})
		require.NoError(t, err)
		require.Len(t, httpflows, 2)
	})

	t.Run("request", func(t *testing.T) {
		t.Parallel()

		ids := make([]int64, 0, 2)
		t.Cleanup(func() {
			DeleteHTTPFlow(consts.GetGormProjectDatabase(), &ypb.DeleteHTTPFlowRequest{
				Id: ids,
			})
		})
		token := uuid.NewString()
		ids = append(ids, insertFlowAndCheck(&schema.HTTPFlow{
			Path: token,
			Tags: token,
		}))
		ids = append(ids, insertFlowAndCheck(&schema.HTTPFlow{
			Request: token,
		}))

		_, httpflows, err := QueryHTTPFlow(db, &ypb.QueryHTTPFlowRequest{
			Keyword:     token,
			KeywordType: "request",
		})
		require.NoError(t, err)
		require.Len(t, httpflows, 1)
	})

	t.Run("response", func(t *testing.T) {
		t.Parallel()

		ids := make([]int64, 0, 2)
		t.Cleanup(func() {
			DeleteHTTPFlow(consts.GetGormProjectDatabase(), &ypb.DeleteHTTPFlowRequest{
				Id: ids,
			})
		})
		token := uuid.NewString()
		ids = append(ids, insertFlowAndCheck(&schema.HTTPFlow{
			Path: token,
			Tags: token,
		}))
		ids = append(ids, insertFlowAndCheck(&schema.HTTPFlow{
			Response: token,
		}))

		_, httpflows, err := QueryHTTPFlow(db, &ypb.QueryHTTPFlowRequest{
			Keyword:     token,
			KeywordType: "response",
		})
		require.NoError(t, err)
		require.Len(t, httpflows, 1)
	})

}
