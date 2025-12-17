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

func TestQueryFilterHTTPFlow_SuffixPrecision(t *testing.T) {
	// 测试后缀过滤的精确性：过滤 .js 不应该过滤 .json 和 .jsp
	token := utils.RandString(10)

	// 创建测试数据
	jsFlow := &schema.HTTPFlow{
		Url:  fmt.Sprintf("https://example.com/%s.js", token),
		Path: fmt.Sprintf("/%s.js", token),
	}
	InsertHTTPFlow(consts.GetGormProjectDatabase().Debug(), jsFlow)

	jsonFlow := &schema.HTTPFlow{
		Url:  fmt.Sprintf("https://example.com/%s.json", token),
		Path: fmt.Sprintf("/%s.json", token),
	}
	InsertHTTPFlow(consts.GetGormProjectDatabase().Debug(), jsonFlow)

	jspFlow := &schema.HTTPFlow{
		Url:  fmt.Sprintf("https://example.com/%s.jsp", token),
		Path: fmt.Sprintf("/%s.jsp", token),
	}
	InsertHTTPFlow(consts.GetGormProjectDatabase().Debug(), jspFlow)

	// 清理测试数据
	defer func() {
		DeleteHTTPFlow(consts.GetGormProjectDatabase(), &ypb.DeleteHTTPFlowRequest{
			Id: []int64{int64(jsFlow.ID), int64(jsonFlow.ID), int64(jspFlow.ID)},
		})
	}()

	// 测试：过滤 .js 后缀
	_, flows, err := QueryHTTPFlow(consts.GetGormProjectDatabase().Debug(), &ypb.QueryHTTPFlowRequest{
		ExcludeSuffix: []string{".js"},
	})
	require.NoError(t, err)

	// 验证结果
	var foundJs, foundJson, foundJsp bool
	for _, flow := range flows {
		if flow.ID == jsFlow.ID {
			foundJs = true
		}
		if flow.ID == jsonFlow.ID {
			foundJson = true
		}
		if flow.ID == jspFlow.ID {
			foundJsp = true
		}
	}

	// .js 应该被过滤掉（不应该找到）
	assert.False(t, foundJs, "过滤 .js 时，.js 文件应该被过滤掉")
	// .json 不应该被过滤（应该找到）
	assert.True(t, foundJson, "过滤 .js 时，.json 文件不应该被过滤")
	// .jsp 不应该被过滤（应该找到）
	assert.True(t, foundJsp, "过滤 .js 时，.jsp 文件不应该被过滤")
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
	ids := make([]int64, 0, 8)
	defer func() {
		if len(ids) > 0 {
			DeleteHTTPFlow(consts.GetGormProjectDatabase(), &ypb.DeleteHTTPFlowRequest{Id: ids})
		}
	}()

	jsFlow := &schema.HTTPFlow{
		Url:  fmt.Sprintf("https://example.com/%s.js", token),
		Path: fmt.Sprintf("https://example.com/%s.js", token),
		Tags: fmt.Sprintf("SQL注入测试点|%s|SQL注入测试点", colorToken),
	}
	require.NoError(t, InsertHTTPFlow(consts.GetGormProjectDatabase(), jsFlow))
	ids = append(ids, int64(jsFlow.ID))
	db := FilterHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{
		Keyword: token,
		Color:   []string{colorToken},
	})
	res := []*schema.HTTPFlow{}
	db.Find(&res)
	assert.Len(t, res, 1)

	noneToken := ksuid.New().String()
	noneFlow := &schema.HTTPFlow{
		Url:  fmt.Sprintf("https://example.com/%s-none", noneToken),
		Path: fmt.Sprintf("/%s-none", noneToken),
		Tags: "tag1|tag2",
	}
	require.NoError(t, InsertHTTPFlow(consts.GetGormProjectDatabase(), noneFlow))
	ids = append(ids, int64(noneFlow.ID))

	emptyTagFlow := &schema.HTTPFlow{
		Url:  fmt.Sprintf("https://example.com/%s-empty", noneToken),
		Path: fmt.Sprintf("/%s-empty", noneToken),
		Tags: "",
	}
	require.NoError(t, InsertHTTPFlow(consts.GetGormProjectDatabase(), emptyTagFlow))
	ids = append(ids, int64(emptyTagFlow.ID))

	redFlow := &schema.HTTPFlow{
		Url:  fmt.Sprintf("https://example.com/%s-red", noneToken),
		Path: fmt.Sprintf("/%s-red", noneToken),
		Tags: "tag1|YAKIT_COLOR_RED|tag2",
	}
	require.NoError(t, InsertHTTPFlow(consts.GetGormProjectDatabase(), redFlow))
	ids = append(ids, int64(redFlow.ID))

	var noneRes []*schema.HTTPFlow
	FilterHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{
		Keyword: noneToken,
		Color:   []string{"YAKIT_COLOR_NONE"},
	}).Find(&noneRes)
	assert.Len(t, noneRes, 2)

	var mixedRes []*schema.HTTPFlow
	FilterHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{
		Keyword: noneToken,
		Color:   []string{"YAKIT_COLOR_RED", "YAKIT_COLOR_NONE"},
	}).Find(&mixedRes)
	assert.Len(t, mixedRes, 3)
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

// TestExcludeKeywordsWithSpecialChars 测试排除关键字功能对特殊字符的处理
func TestExcludeKeywordsWithSpecialChars(t *testing.T) {
	baseToken := uuid.NewString()
	ids := make([]int64, 0, 10)
	defer func() {
		if len(ids) > 0 {
			DeleteHTTPFlow(consts.GetGormProjectDatabase(), &ypb.DeleteHTTPFlowRequest{
				Id: ids,
			})
		}
	}()

	// 创建包含特殊字符的测试数据
	testCases := []struct {
		name        string
		url         string
		path        string
		request     string
		response    string
		excludeKey  string
		shouldMatch bool // 是否应该被排除
	}{
		{
			name:        "normal_content",
			url:         fmt.Sprintf("https://example.com/%s/normal", baseToken),
			path:        fmt.Sprintf("/%s/normal", baseToken),
			request:     fmt.Sprintf("GET /%s/normal HTTP/1.1", baseToken),
			response:    fmt.Sprintf("HTTP/1.1 200 OK\r\nContent: %s/normal", baseToken),
			excludeKey:  "normal",
			shouldMatch: true,
		},
		{
			name:        "with_percent",
			url:         fmt.Sprintf("https://example.com/%s/50%%discount", baseToken),
			path:        fmt.Sprintf("/%s/50%%discount", baseToken),
			request:     fmt.Sprintf("GET /%s/50%%discount HTTP/1.1", baseToken),
			response:    fmt.Sprintf("HTTP/1.1 200 OK\r\nContent: %s/50%%discount", baseToken),
			excludeKey:  "50%",
			shouldMatch: true,
		},
		{
			name:        "with_underscore",
			url:         fmt.Sprintf("https://example.com/%s/file_name", baseToken),
			path:        fmt.Sprintf("/%s/file_name", baseToken),
			request:     fmt.Sprintf("GET /%s/file_name HTTP/1.1", baseToken),
			response:    fmt.Sprintf("HTTP/1.1 200 OK\r\nContent: %s/file_name", baseToken),
			excludeKey:  "file_name",
			shouldMatch: true,
		},
		{
			name:        "with_brackets",
			url:         fmt.Sprintf("https://example.com/%s/[test]", baseToken),
			path:        fmt.Sprintf("/%s/[test]", baseToken),
			request:     fmt.Sprintf("GET /%s/[test] HTTP/1.1", baseToken),
			response:    fmt.Sprintf("HTTP/1.1 200 OK\r\nContent: %s/[test]", baseToken),
			excludeKey:  "[test]",
			shouldMatch: true,
		},
		{
			name:        "with_caret",
			url:         fmt.Sprintf("https://example.com/%s/^test", baseToken),
			path:        fmt.Sprintf("/%s/^test", baseToken),
			request:     fmt.Sprintf("GET /%s/^test HTTP/1.1", baseToken),
			response:    fmt.Sprintf("HTTP/1.1 200 OK\r\nContent: %s/^test", baseToken),
			excludeKey:  "^test",
			shouldMatch: true,
		},
		{
			name:        "with_backslash",
			url:         fmt.Sprintf("https://example.com/%s/\\test", baseToken),
			path:        fmt.Sprintf("/%s/\\test", baseToken),
			request:     fmt.Sprintf("GET /%s/\\test HTTP/1.1", baseToken),
			response:    fmt.Sprintf("HTTP/1.1 200 OK\r\nContent: %s/\\test", baseToken),
			excludeKey:  "\\test",
			shouldMatch: true,
		},
		{
			name:        "mixed_special_chars",
			url:         fmt.Sprintf("https://example.com/%s/50%%_discount[2024]^test\\value", baseToken),
			path:        fmt.Sprintf("/%s/50%%_discount[2024]^test\\value", baseToken),
			request:     fmt.Sprintf("GET /%s/50%%_discount[2024]^test\\value HTTP/1.1", baseToken),
			response:    fmt.Sprintf("HTTP/1.1 200 OK\r\nContent: %s/50%%_discount[2024]^test\\value", baseToken),
			excludeKey:  "50%_discount[2024]^test\\value",
			shouldMatch: true,
		},
		{
			name:        "should_not_exclude",
			url:         fmt.Sprintf("https://example.com/%s/other", baseToken),
			path:        fmt.Sprintf("/%s/other", baseToken),
			request:     fmt.Sprintf("GET /%s/other HTTP/1.1", baseToken),
			response:    fmt.Sprintf("HTTP/1.1 200 OK\r\nContent: %s/other", baseToken),
			excludeKey:  "normal",
			shouldMatch: false,
		},
	}

	// 插入测试数据
	for _, tc := range testCases {
		flow := &schema.HTTPFlow{
			Url:      tc.url,
			Path:     tc.path,
			Request:  tc.request,
			Response: tc.response,
		}
		err := InsertHTTPFlow(consts.GetGormProjectDatabase(), flow)
		require.NoError(t, err)
		ids = append(ids, int64(flow.ID))
	}

	db := consts.GetGormProjectDatabase().Debug()

	// 测试每个特殊字符的排除功能
	for _, tc := range testCases {
		if !tc.shouldMatch {
			continue // 跳过不应该被排除的测试用例
		}

		t.Run(fmt.Sprintf("排除关键字_%s", tc.name), func(t *testing.T) {
			_, httpflows, err := QueryHTTPFlow(db, &ypb.QueryHTTPFlowRequest{
				Keyword:         baseToken,
				ExcludeKeywords: []string{tc.excludeKey},
			})
			require.NoError(t, err)

			// 验证被排除的记录不在结果中
			for _, flow := range httpflows {
				require.NotContains(t, flow.Url, tc.excludeKey, "URL不应包含被排除的关键字")
				require.NotContains(t, flow.Path, tc.excludeKey, "Path不应包含被排除的关键字")
				require.NotContains(t, string(flow.Request), tc.excludeKey, "Request不应包含被排除的关键字")
				require.NotContains(t, string(flow.Response), tc.excludeKey, "Response不应包含被排除的关键字")
			}
		})
	}

	// 测试多个排除关键字
	t.Run("排除多个特殊字符关键字", func(t *testing.T) {
		_, httpflows, err := QueryHTTPFlow(db, &ypb.QueryHTTPFlowRequest{
			Keyword:         baseToken,
			ExcludeKeywords: []string{"50%", "file_name", "[test]"},
		})
		require.NoError(t, err)

		// 验证结果中不包含任何被排除的关键字
		for _, flow := range httpflows {
			require.NotContains(t, flow.Url, "50%")
			require.NotContains(t, flow.Url, "file_name")
			require.NotContains(t, flow.Url, "[test]")
		}
	})
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
