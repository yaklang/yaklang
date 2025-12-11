package yakgrpc

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/yaklib"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"

	"github.com/yaklang/yaklang/common/har"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/model"
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

// TestExportHTTPFlows_RequestLength 测试 request_length 字段导出
func TestExportHTTPFlows_RequestLength(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	// 创建测试数据，设置 RequestLength
	reqRaw := lowhttp.FixHTTPRequest([]byte(`POST /test HTTP/1.1
Host: www.example.com
Content-Length: 10

test body`))
	rsp, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 OK
Content-Length: 5

hello`))

	flow, err := yakit.CreateHTTPFlowFromHTTPWithBodySavedFromRaw(true, reqRaw, rsp, "test", "https://www.example.com/test", "")
	require.NoError(t, err)

	// 设置明确的 RequestLength 值用于测试
	expectedRequestLength := int64(1234)
	flow.RequestLength = expectedRequestLength
	flow.CalcHash()
	consts.GetGormProjectDatabase().Save(flow)

	t.Cleanup(func() {
		consts.GetGormProjectDatabase().Unscoped().Where("id = ?", flow.ID).Delete(&schema.HTTPFlow{})
	})

	// 测试 ExportHTTPFlows 包含 request_length 字段
	// 注意：必须包含 id 字段，否则无法正确匹配和转换
	response, err := client.ExportHTTPFlows(context.Background(), &ypb.ExportHTTPFlowsRequest{
		ExportWhere: &ypb.QueryHTTPFlowRequest{
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 20,
			},
			Full: true,
		},
		Ids:       []int64{int64(flow.ID)},
		FieldName: []string{"id", "request_length", "url", "method"},
	})
	require.NoError(t, err, "export httpFlows error")
	require.NotEmpty(t, response.Data, "response data should not be empty")

	// 验证 request_length 字段存在且值正确
	found := false
	testFlowID := uint64(flow.ID)
	for _, exportedFlow := range response.Data {
		if exportedFlow.GetId() == testFlowID {
			found = true
			require.Equal(t, expectedRequestLength, exportedFlow.RequestLength, "request_length should match")
			break
		}
	}
	require.True(t, found, "test flow should be found in export result")
}

// TestExportHTTPFlowStream_CSV_RequestLength 测试 CSV 导出中的 request_length 字段
func TestExportHTTPFlowStream_CSV_RequestLength(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	// 创建测试数据
	reqRaw := lowhttp.FixHTTPRequest([]byte(`POST /test HTTP/1.1
Host: www.example.com
Content-Length: 15

request body 123`))
	rsp, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 OK
Content-Length: 5

hello`))

	flow, err := yakit.CreateHTTPFlowFromHTTPWithBodySavedFromRaw(true, reqRaw, rsp, "test", "https://www.example.com/test", "")
	require.NoError(t, err)

	// 设置明确的 RequestLength 值用于测试
	expectedRequestLength := int64(5678)
	flow.RequestLength = expectedRequestLength
	flow.CalcHash()
	consts.GetGormProjectDatabase().Save(flow)

	t.Cleanup(func() {
		consts.GetGormProjectDatabase().Unscoped().Where("id = ?", flow.ID).Delete(&schema.HTTPFlow{})
	})

	// 测试 CSV 导出
	tmpFile := filepath.Join(t.TempDir(), "test_request_length.csv")
	stream, err := client.ExportHTTPFlowStream(context.Background(), &ypb.ExportHTTPFlowStreamRequest{
		Filter: &ypb.QueryHTTPFlowRequest{
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 20,
			},
		},
		FieldName:  []string{"id", "request_length", "url", "method"},
		ExportType: "csv",
		TargetPath: tmpFile,
	})
	require.NoError(t, err)

	// 等待导出完成
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if resp.Percent >= 1.0 {
			break
		}
	}

	// 验证 CSV 文件存在
	require.FileExists(t, tmpFile)

	// 读取并解析 CSV
	data, err := os.ReadFile(tmpFile)
	require.NoError(t, err)

	reader := csv.NewReader(strings.NewReader(string(data)))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	require.Greater(t, len(records), 1, "CSV should have header and at least one data row")

	// 验证表头包含 request_length
	headers := records[0]
	requestLengthIdx := -1
	for i, header := range headers {
		if header == "request_length" {
			requestLengthIdx = i
			break
		}
	}
	require.NotEqual(t, -1, requestLengthIdx, "request_length column should be found in CSV header")

	// 验证数据行包含正确的值
	found := false
	for i := 1; i < len(records); i++ {
		row := records[i]
		if len(row) > requestLengthIdx {
			// 检查 ID 是否匹配（如果 ID 列存在）
			idIdx := -1
			for j, h := range headers {
				if h == "id" {
					idIdx = j
					break
				}
			}
			if idIdx >= 0 && len(row) > idIdx {
				idStr := row[idIdx]
				if idStr == strconv.FormatUint(uint64(flow.ID), 10) {
					found = true
					require.Equal(t, strconv.FormatInt(expectedRequestLength, 10), row[requestLengthIdx], "request_length value should match")
					break
				}
			}
		}
	}
	require.True(t, found, "test flow should be found in CSV export")
}

// TestExportHTTPFlows_FieldMapping 测试字段映射（request_length 字段名映射）
func TestExportHTTPFlows_FieldMapping(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	// 创建测试数据
	reqRaw := lowhttp.FixHTTPRequest([]byte(`GET /test HTTP/1.1
Host: www.example.com
`))
	rsp, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 OK
Content-Length: 5

hello`))

	flow, err := yakit.CreateHTTPFlowFromHTTPWithBodySavedFromRaw(true, reqRaw, rsp, "test", "https://www.example.com/test", "")
	require.NoError(t, err)

	expectedRequestLength := int64(9999)
	flow.RequestLength = expectedRequestLength
	flow.CalcHash()
	consts.GetGormProjectDatabase().Save(flow)

	t.Cleanup(func() {
		consts.GetGormProjectDatabase().Unscoped().Where("id = ?", flow.ID).Delete(&schema.HTTPFlow{})
	})

	// 测试字段名映射：前端传递 "request_length"，后端应该能正确查询数据库的 request_length 列
	// 注意：必须包含 id 字段，否则无法正确匹配和转换
	response, err := client.ExportHTTPFlows(context.Background(), &ypb.ExportHTTPFlowsRequest{
		ExportWhere: &ypb.QueryHTTPFlowRequest{
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 20,
			},
			Full: false, // 测试非 Full 模式下的字段映射
		},
		Ids:       []int64{int64(flow.ID)},
		FieldName: []string{"id", "request_length", "body_length", "url"}, // 测试多个字段，包含 id 用于匹配
	})
	require.NoError(t, err, "export httpFlows error")
	require.NotEmpty(t, response.Data, "response data should not be empty")

	// 验证字段映射正确
	found := false
	for _, exportedFlow := range response.Data {
		if exportedFlow.GetId() == uint64(flow.ID) {
			found = true
			// 验证 request_length 字段正确映射
			require.Equal(t, expectedRequestLength, exportedFlow.RequestLength, "request_length field mapping should be correct")
			// 验证其他字段也存在
			require.NotEmpty(t, exportedFlow.Url, "url field should be present")
			require.Greater(t, exportedFlow.BodyLength, int64(0), "body_length field should be present")
			break
		}
	}
	require.True(t, found, "test flow should be found in export result")
}

// TestExportHTTPFlows_FixedFieldList 测试固定字段列表是否包含 request_length
func TestExportHTTPFlows_FixedFieldList(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	// 创建测试数据
	reqRaw := lowhttp.FixHTTPRequest([]byte(`GET /test HTTP/1.1
Host: www.example.com
`))
	rsp, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 OK
Content-Length: 5

hello`))

	flow, err := yakit.CreateHTTPFlowFromHTTPWithBodySavedFromRaw(true, reqRaw, rsp, "test", "https://www.example.com/test", "")
	require.NoError(t, err)

	expectedRequestLength := int64(8888)
	flow.RequestLength = expectedRequestLength
	flow.CalcHash()
	consts.GetGormProjectDatabase().Save(flow)

	t.Cleanup(func() {
		consts.GetGormProjectDatabase().Unscoped().Where("id = ?", flow.ID).Delete(&schema.HTTPFlow{})
	})

	// 测试在非 Full 模式下，固定字段列表应该包含 request_length
	// 注意：在非 Full 模式下，BuildHTTPFlowQuery 会先设置固定字段列表（包含 request_length）
	// 然后 Select(params.FieldName) 会覆盖它，所以我们需要包含 id 字段用于匹配
	// 这个测试验证：即使只选择了 request_length，由于它在固定字段列表中，也能被正确查询
	response, err := client.ExportHTTPFlows(context.Background(), &ypb.ExportHTTPFlowsRequest{
		ExportWhere: &ypb.QueryHTTPFlowRequest{
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 20,
			},
			Full: false, // 非 Full 模式，使用固定字段列表
		},
		Ids:       []int64{int64(flow.ID)},
		FieldName: []string{"id", "request_length", "url"}, // 包含 id 用于匹配，request_length 用于验证，url 用于验证其他字段
	})
	require.NoError(t, err, "export httpFlows error")
	require.NotEmpty(t, response.Data, "response data should not be empty")

	// 验证 request_length 字段能够正确查询（说明它在固定字段列表中或能被正确选择）
	found := false
	testFlowID := uint64(flow.ID)
	for _, exportedFlow := range response.Data {
		if exportedFlow.GetId() == testFlowID {
			found = true
			// 验证 request_length 字段值正确（说明它在固定字段列表中，能被正确查询）
			require.Equal(t, expectedRequestLength, exportedFlow.RequestLength, "request_length should be available in fixed field list")
			// 验证其他字段也存在
			require.NotEmpty(t, exportedFlow.Url, "url field should be present")
			break
		}
	}
	if !found {
		// 如果没找到，打印一些调试信息
		t.Logf("Expected flow ID: %d", testFlowID)
		t.Logf("Found %d flows in response", len(response.Data))
		for i, f := range response.Data {
			t.Logf("Flow %d: ID=%d, RequestLength=%d, Url=%s", i, f.GetId(), f.RequestLength, f.Url)
		}
	}
	require.True(t, found, "test flow should be found in export result")
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

func generateTestHTTPFlowData(db *gorm.DB, num int, url string) (string, []int64) {
	token := utils.RandStringBytes(16)
	ids := make([]int64, 0, num)
	host, port, _ := utils.ParseStringToHostPort(url)
	for i := 0; i < num; i++ {

		flow, _ := yakit.CreateHTTPFlow(
			yakit.CreateHTTPFlowWithURL(url),
			yakit.CreateHTTPFlowWithRequestRaw([]byte(
				fmt.Sprintf(
					"GET / HTTP/1.1\r\nHost: %s:%d\r\n\r\n%s",
					host, port,
					utils.RandStringBytes(16),
				),
			),
			),
			yakit.CreateHTTPFlowWithResponseRaw([]byte("HTTP/1.1 200 OK\r\nContent-Length: 16\r\n\r\n"+token)),
		)
		err := yakit.InsertHTTPFlow(db, flow)
		if err == nil {
			ids = append(ids, int64(flow.ID))
		}
	}
	return token, ids
}

// func generateTestLargeHTTPFlowData(db *gorm.DB, url string) (string, int64) {
// 	token := utils.RandStringBytes(16)
// 	host, port, _ := utils.ParseStringToHostPort(url)
// 	dataSize := 10 * 1024 * 1024
// 	data := strings.Repeat("a", dataSize)

// 	flow, _ := yakit.CreateHTTPFlow(
// 		yakit.CreateHTTPFlowWithURL(url),
// 		yakit.CreateHTTPFlowWithRequestRaw([]byte(
// 			fmt.Sprintf(
// 				"GET /?a=%s HTTP/1.1\r\nHost: %s:%d\r\n\r\n%s",
// 				token,
// 				host, port,
// 				utils.RandStringBytes(16),
// 			),
// 		),
// 		),
// 		yakit.CreateHTTPFlowWithResponseRaw([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", dataSize, data))),
// 	)
// 	yakit.InsertHTTPFlow(db, flow)
// 	return token, int64(flow.ID)
// }

// func TestLARGEGRPCMUSTPASS_Export_Large_HTTPFlow(t *testing.T) {
// 	client, err := NewLocalClient()
// 	require.NoError(t, err)
// 	ctx := utils.TimeoutContextSeconds(10)

// 	db := consts.GetGormProjectDatabase()
// 	dataSize := 15 * 1024 * 1024
// 	host, port := utils.DebugMockHTTP([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", dataSize, strings.Repeat("a", dataSize))))
// 	token := utils.RandStringBytes(16)
// 	_, _, err = poc.DoGET(fmt.Sprintf("http://%s:%d?a=%s", host, port, token), poc.WithSave(true))
// 	require.NoError(t, err)

// 	t.Cleanup(func() {
// 		yakit.DeleteHTTPFlow(db, &ypb.DeleteHTTPFlowRequest{
// 			Filter: &ypb.QueryHTTPFlowRequest{
// 				Keyword: token,
// 			},
// 		})
// 	})
// 	// wait until httpflow save
// 	filter := &ypb.QueryHTTPFlowRequest{
// 		Keyword: token,
// 	}
// 	_, err = QueryHTTPFlows(ctx, client, filter, 1)
// 	require.NoError(t, err)

// 	fn := filepath.Join(t.TempDir(), "test.har")
// 	stream, err := client.ExportHTTPFlowStream(ctx, &ypb.ExportHTTPFlowStreamRequest{
// 		Filter:     filter,
// 		ExportType: "har",
// 		TargetPath: fn,
// 	})
// 	require.NoError(t, err)

// 	progress := 0.0
// 	for {
// 		msg, err := stream.Recv()
// 		spew.Dump(msg)
// 		if err != nil {
// 			break
// 		}
// 		progress = msg.Percent
// 	}

// 	// check export
// 	require.Equal(t, 1.0, progress)
// 	fh, err := os.Open(fn)
// 	defer fh.Close()
// 	require.NoError(t, err)

// 	har.ImportHTTPArchiveStream(fh, func(h *har.HAREntry) error {
// 		require.Equal(t, dataSize, len(h.Response.Content.Text))
// 		return nil
// 	})
// }

func TestGRPCMUSTPASS_Export_And_ImportHAR(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	wantCount := 16
	wantURL := "http://example.com/"
	db := consts.GetGormProjectDatabase()
	token, ids := generateTestHTTPFlowData(db, wantCount, wantURL)

	t.Cleanup(func() {
		yakit.DeleteHTTPFlow(db, &ypb.DeleteHTTPFlowRequest{
			Id: ids,
		})
	})

	// export
	fn := filepath.Join(t.TempDir(), "test.har")
	stream, err := client.ExportHTTPFlowStream(utils.TimeoutContextSeconds(10), &ypb.ExportHTTPFlowStreamRequest{
		Filter: &ypb.QueryHTTPFlowRequest{
			Keyword: token,
		},
		FieldName: []string{
			"request", "response",
		},
		ExportType: "har",
		TargetPath: fn,
	})
	require.NoError(t, err)
	progress := 0.0
	for {
		msg, err := stream.Recv()
		if err != nil {
			break
		}
		progress = msg.Percent
	}

	// check export
	require.Equal(t, 1.0, progress)
	count := 0
	fh, err := os.Open(fn)
	require.NoError(t, err)
	t.Cleanup(func() {
		fh.Close()
	})
	har.ImportHTTPArchiveStream(fh, func(h *har.HAREntry) error {
		count++
		require.NotNil(t, h.Request)
		require.Equal(t, wantURL, h.Request.URL)
		require.NotNil(t, h.Request.PostData)
		require.Greater(t, len(h.Request.PostData.Text), 0)
		require.NotNil(t, h.Response)
		require.NotNil(t, h.Response.Content)
		require.Equal(t, token, h.Response.Content.Text)
		return nil
	})
	require.Equal(t, wantCount, count)

	// delete before import
	err = yakit.DeleteHTTPFlow(db, &ypb.DeleteHTTPFlowRequest{
		Id: ids,
	})
	require.NoError(t, err)

	// import
	importStream, err := client.ImportHTTPFlowStream(utils.TimeoutContextSeconds(10), &ypb.ImportHTTPFlowStreamRequest{
		InputPath: fn,
	})
	require.NoError(t, err)
	progress = 0.0
	for {
		msg, err := importStream.Recv()
		if err != nil {
			log.Error(err)
			break
		}
		progress = msg.Percent
	}
	// check import
	require.Equal(t, 1.0, progress)
	_, flows, err := yakit.QueryHTTPFlow(db, &ypb.QueryHTTPFlowRequest{
		Keyword: token,
	})
	require.NoError(t, err)
	for _, flow := range flows {
		require.Equal(t, wantURL, flow.Url)
		require.NotEmpty(t, flow.Request)
		require.Contains(t, flow.Response, token)
	}
	require.Equal(t, wantCount, len(flows))
}

func TestGRPCMUSTPASS_Export_CSV(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	wantCount := 16
	wantURL := "http://example.com/"
	db := consts.GetGormProjectDatabase()
	fieldNames := []string{"method", "url", "request", "response"}
	token, ids := generateTestHTTPFlowData(db, wantCount, wantURL)

	t.Cleanup(func() {
		yakit.DeleteHTTPFlow(db, &ypb.DeleteHTTPFlowRequest{
			Id: ids,
		})
	})
	fn := filepath.Join(t.TempDir(), "test.csv")
	require.NoError(t, err)

	stream, err := client.ExportHTTPFlowStream(utils.TimeoutContextSeconds(10), &ypb.ExportHTTPFlowStreamRequest{
		FieldName: fieldNames,
		Filter: &ypb.QueryHTTPFlowRequest{
			Keyword: token,
		},
		ExportType: "csv",
		TargetPath: fn,
	})
	require.NoError(t, err)
	progress := 0.0
	for {
		msg, err := stream.Recv()
		if err != nil {
			break
		}
		progress = msg.Percent
	}

	// check export
	require.Equal(t, 1.0, progress)

	fh, err := os.Open(fn)
	require.NoError(t, err)
	t.Cleanup(func() {
		fh.Close()
	})
	reader := csv.NewReader(fh)
	gotFieldNames, err := reader.Read()
	require.NoError(t, err)
	// export will add a "id" field
	fieldNames = append([]string{"id"}, fieldNames...)
	require.ElementsMatch(t, fieldNames, gotFieldNames)
	records, err := reader.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, wantCount)
	for _, record := range records {
		require.NotEmpty(t, record[0])        // id
		require.Equal(t, "GET", record[1])    // method
		require.Equal(t, wantURL, record[2])  // url
		require.NotEmpty(t, record[3])        // request
		require.Contains(t, record[4], token) // response
	}
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

func TestHTTPFlowFieldGroup(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)

	// create httpflow
	originTag := uuid.NewString()
	url1 := fmt.Sprintf("http://%s.com", utils.RandStringBytes(5))
	flow, err := yakit.CreateHTTPFlow(yakit.CreateHTTPFlowWithURL(url1), yakit.CreateHTTPFlowWithTags(originTag))
	require.NoError(t, err)
	err = yakit.InsertHTTPFlow(consts.GetGormProjectDatabase(), flow)
	require.NoError(t, err)

	// query and check tag
	{
		rsp, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
			IncludeHash: []string{flow.Hash},
		})
		require.NoError(t, err)
		require.Equal(t, 1, len(rsp.Data))
		require.Equal(t, flow.Tags, rsp.Data[0].Tags)
	}
	// set uuidTag and check
	uuidTag := uuid.NewString()
	{
		// test set empty for tag
		_, err = client.SetTagForHTTPFlow(context.Background(), &ypb.SetTagForHTTPFlowRequest{
			Id:   int64(flow.ID),
			Tags: nil,
		})
		require.NoError(t, err)

		// query from db
		flow, err = yakit.GetHTTPFlow(consts.GetGormProjectDatabase(), int64(flow.ID))
		require.NoError(t, err)
		require.Empty(t, flow.Tags)

		_, err = client.SetTagForHTTPFlow(context.Background(), &ypb.SetTagForHTTPFlowRequest{
			Id:   int64(flow.ID),
			Tags: []string{uuidTag},
		})
		require.NoError(t, err)

		// query from db
		flow, err = yakit.GetHTTPFlow(consts.GetGormProjectDatabase(), int64(flow.ID))
		require.NoError(t, err)
		require.Contains(t, flow.Tags, uuidTag)

		// query and check tag
		rsp, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
			IncludeHash: []string{flow.Hash},
		})
		require.NoError(t, err)
		require.Equal(t, 1, len(rsp.Data))
		require.Contains(t, rsp.Data[0].Tags, uuidTag)

		// check cache
		flow, have := model.GlobalHTTPFlowCache.Get(flow.CalcCacheHash(false))
		require.True(t, have)
		require.Contains(t, flow.Tags, uuidTag)

		// get filed group
		rsp1, err := client.HTTPFlowsFieldGroup(context.Background(), &ypb.HTTPFlowsFieldGroupRequest{})
		require.NoError(t, err)
		tags := lo.Map(rsp1.Tags, func(item *ypb.TagsCode, _ int) string { return item.Value })
		require.Contains(t, tags, uuidTag)
	}

	// delete httpflow  and check field group
	{
		spew.Dump(flow)
		_, err = client.DeleteHTTPFlows(context.Background(), &ypb.DeleteHTTPFlowRequest{
			Id: []int64{int64(flow.ID)},
		})
		require.NoError(t, err)
		// check cache
		_, have := model.GlobalHTTPFlowCache.Get(flow.CalcCacheHash(false))
		require.False(t, have)
		// check  grpc
		rsp, err := client.HTTPFlowsFieldGroup(context.Background(), &ypb.HTTPFlowsFieldGroupRequest{})
		require.NoError(t, err)
		tags := lo.Map(rsp.Tags, func(item *ypb.TagsCode, _ int) string { return item.Value })
		require.NotContains(t, tags, uuidTag)
	}
}

func TestGRPCMUSTPASS_HTTPFFlow_KeyWord_Search(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	_ = client

	t.Run("match escape content", func(t *testing.T) {
		token := `/bin\/bash` + uuid.NewString()
		flow, err := yakit.CreateHTTPFlow(
			yakit.CreateHTTPFlowWithHTTPS(true),
			yakit.CreateHTTPFlowWithRequestRaw([]byte(`GET / HTTP/1.1
Host: 127.0.0.1:8080
Accept-Encoding: gzip, deflate, br
Accept: */*
Accept-Language: en-US;q=0.9,en;q=0.8
User-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36
Cache-Control: max-age=0
`)),
			yakit.CreateHTTPFlowWithResponseRaw([]byte(fmt.Sprintf(`HTTP/1.1 200 OK
Date: Wed, 09 Apr 2025 05:23:28 GMT
Content-Type: text/plain; charset=utf-8
Content-Length: 10

%s`, token))),
		)
		require.NoError(t, err)
		err = yakit.InsertHTTPFlow(consts.GetGormProjectDatabase(), flow)
		require.NoError(t, err)
		queryFlow, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
			KeywordType: "response",
			Keyword:     token,
		})
		require.NoError(t, err)
		require.Equal(t, 1, len(queryFlow.Data))
		spew.Dump(queryFlow.Data[0])
		id := queryFlow.Data[0].Id
		_, err = client.DeleteHTTPFlows(context.Background(), &ypb.DeleteHTTPFlowRequest{
			Id: []int64{int64(id)},
		})
		require.NoError(t, err)
	})
}

func TestDoHTTPFlowsToOnline(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	token := utils.RandStringBytes(5)
	url1 := "http://" + token + ".com"
	flow1, err := yakit.CreateHTTPFlow(
		yakit.CreateHTTPFlowWithURL(url1),
		yakit.CreateHTTPFlowWithRequestRaw([]byte("GET / HTTP/1.1\r\nHost: "+token+".com\r\n\r\n"+token)),
	)

	require.NoError(t, err)
	require.NoError(t, yakit.InsertHTTPFlow(db, flow1))
	defer yakit.DeleteHTTPFlowByID(db, int64(flow1.ID))

	mockey.PatchConvey("skip token check", t, func() {
		mockClient := new(yaklib.OnlineClient)

		// 总是成功返回
		mockey.Mock((*yaklib.OnlineClient).UploadHTTPFlowToOnline).
			To(func(_ *yaklib.OnlineClient, ctx context.Context, req *ypb.HTTPFlowsToOnlineRequest, data []byte) error {
				var tmp HTTPFlowShare
				_ = json.Unmarshal(data, &tmp)
				return nil
			}).Build()

		mockey.Mock(yaklib.NewOnlineClient).
			To(func(baseUrl string) *yaklib.OnlineClient {
				return mockClient
			}).Build()

		server := &TestServerWrapper{
			Server:       &Server{},
			onlineClient: yaklib.OnlineClient{},
		}

		toOnlineReq := &ypb.HTTPFlowsToOnlineRequest{
			Token:       "test-token",
			ProjectName: "test-project",
		}

		success, failed, err := server.DoHTTPFlowsSync(context.Background(), db, toOnlineReq)

		// 验证结果
		assert.NoError(t, err)
		assert.NotNil(t, success)
		assert.Contains(t, success, flow1.Hash)
		assert.Empty(t, failed)
	})
}

// TestGRPCMUSTPASS_Export_HAR_WithFieldSelection 测试 HAR 导出的字段选择功能
// 参考 Excel 导出测试风格，验证字段选择对 HAR 导出的影响
func TestGRPCMUSTPASS_Export_HAR_WithFieldSelection(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	wantCount := 16
	wantURL := "http://example.com/"
	db := consts.GetGormProjectDatabase()
	token, ids := generateTestHTTPFlowData(db, wantCount, wantURL)

	t.Cleanup(func() {
		yakit.DeleteHTTPFlow(db, &ypb.DeleteHTTPFlowRequest{
			Id: ids,
		})
	})

	t.Run("only request packet fields", func(t *testing.T) {
		fn := filepath.Join(t.TempDir(), "test_request_only.har")
		fieldNames := []string{"request", "method", "url"}
		stream, err := client.ExportHTTPFlowStream(utils.TimeoutContextSeconds(10), &ypb.ExportHTTPFlowStreamRequest{
			FieldName: fieldNames,
			Filter: &ypb.QueryHTTPFlowRequest{
				Keyword: token,
			},
			ExportType: "har",
			TargetPath: fn,
		})
		require.NoError(t, err)
		progress := 0.0
		for {
			msg, err := stream.Recv()
			if err != nil {
				break
			}
			progress = msg.Percent
		}
		require.Equal(t, 1.0, progress)

		fh, err := os.Open(fn)
		require.NoError(t, err)
		defer fh.Close()
		count := 0
		har.ImportHTTPArchiveStream(fh, func(h *har.HAREntry) error {
			count++
			require.NotNil(t, h.Request)
			require.Equal(t, wantURL, h.Request.URL)
			require.NotNil(t, h.Request.PostData) // 应该包含请求 body
			require.Greater(t, len(h.Request.PostData.Text), 0)
			require.Nil(t, h.Response) // 没有选择 response 相关字段，应该为 nil
			return nil
		})
		require.Equal(t, wantCount, count)
	})

	t.Run("only response packet fields", func(t *testing.T) {
		fn := filepath.Join(t.TempDir(), "test_response_only.har")
		fieldNames := []string{"response", "status_code"}
		stream, err := client.ExportHTTPFlowStream(utils.TimeoutContextSeconds(10), &ypb.ExportHTTPFlowStreamRequest{
			FieldName: fieldNames,
			Filter: &ypb.QueryHTTPFlowRequest{
				Keyword: token,
			},
			ExportType: "har",
			TargetPath: fn,
		})
		require.NoError(t, err)
		progress := 0.0
		for {
			msg, err := stream.Recv()
			if err != nil {
				break
			}
			progress = msg.Percent
		}
		require.Equal(t, 1.0, progress)

		fh, err := os.Open(fn)
		require.NoError(t, err)
		defer fh.Close()
		count := 0
		har.ImportHTTPArchiveStream(fh, func(h *har.HAREntry) error {
			count++
			require.Nil(t, h.Request) // 没有选择 request 相关字段，应该为 nil
			require.NotNil(t, h.Response)
			require.NotNil(t, h.Response.Content)
			require.Contains(t, h.Response.Content.Text, token) // 应该包含响应 body
			return nil
		})
		require.Equal(t, wantCount, count)
	})

	t.Run("only metadata fields", func(t *testing.T) {
		fn := filepath.Join(t.TempDir(), "test_metadata_only.har")
		fieldNames := []string{"tags", "path"}
		stream, err := client.ExportHTTPFlowStream(utils.TimeoutContextSeconds(10), &ypb.ExportHTTPFlowStreamRequest{
			FieldName: fieldNames,
			Filter: &ypb.QueryHTTPFlowRequest{
				Keyword: token,
			},
			ExportType: "har",
			TargetPath: fn,
		})
		require.NoError(t, err)
		progress := 0.0
		for {
			msg, err := stream.Recv()
			if err != nil {
				break
			}
			progress = msg.Percent
		}
		require.Equal(t, 1.0, progress)

		fh, err := os.Open(fn)
		require.NoError(t, err)
		defer fh.Close()
		count := 0
		har.ImportHTTPArchiveStream(fh, func(h *har.HAREntry) error {
			count++
			require.Nil(t, h.Request)              // 没有选择 request 相关字段，应该为 nil
			require.Nil(t, h.Response)             // 没有选择 response 相关字段，应该为 nil
			require.NotNil(t, h.MetaData)          // 应该包含元数据
			require.Equal(t, "/", h.MetaData.Path) // 应该包含选中的字段
			return nil
		})
		require.Equal(t, wantCount, count)
	})

	t.Run("test wrong field names - missing request and response", func(t *testing.T) {
		// 测试1：不传递FieldName（应该不包含任何字段，因为 hasField 返回 false）
		fn1 := filepath.Join(t.TempDir(), "test_no_fieldname.har")
		stream1, err := client.ExportHTTPFlowStream(utils.TimeoutContextSeconds(10), &ypb.ExportHTTPFlowStreamRequest{
			// 不传递FieldName
			Filter: &ypb.QueryHTTPFlowRequest{
				Keyword: token,
			},
			ExportType: "har",
			TargetPath: fn1,
		})
		require.NoError(t, err)
		progress1 := 0.0
		for {
			msg, err := stream1.Recv()
			if err != nil {
				break
			}
			progress1 = msg.Percent
		}
		require.Equal(t, 1.0, progress1)

		// 测试2：传递错误的字段名（缺少request和response，但包含method和url会触发创建Request对象）
		fn2 := filepath.Join(t.TempDir(), "test_wrong_fieldname.har")
		wrongFieldNames := []string{"id", "method", "status_code", "url", "path", "from_plugin", "tags"}
		stream2, err := client.ExportHTTPFlowStream(utils.TimeoutContextSeconds(10), &ypb.ExportHTTPFlowStreamRequest{
			FieldName: wrongFieldNames, // ⚠️ 缺少"request"和"response"
			Filter: &ypb.QueryHTTPFlowRequest{
				Keyword: token,
			},
			ExportType: "har",
			TargetPath: fn2,
		})
		require.NoError(t, err)
		progress2 := 0.0
		for {
			msg, err := stream2.Recv()
			if err != nil {
				break
			}
			progress2 = msg.Percent
		}
		require.Equal(t, 1.0, progress2)

		// 验证两个文件的内容
		fh1, err := os.Open(fn1)
		require.NoError(t, err)
		defer fh1.Close()

		fh2, err := os.Open(fn2)
		require.NoError(t, err)
		defer fh2.Close()

		// 读取第一个文件（不传递FieldName，应该不包含任何字段）
		hasRequest1 := false
		hasResponse1 := false
		hasRequestBody1 := false
		hasResponseBody1 := false
		count1 := 0
		har.ImportHTTPArchiveStream(fh1, func(h *har.HAREntry) error {
			count1++
			if h.Request != nil {
				hasRequest1 = true
				if h.Request.PostData != nil && len(h.Request.PostData.Text) > 0 {
					hasRequestBody1 = true
				}
			}
			if h.Response != nil {
				hasResponse1 = true
				if h.Response.Content != nil && len(h.Response.Content.Text) > 0 {
					hasResponseBody1 = true
				}
			}
			return nil
		})

		// 读取第二个文件（传递错误字段名，应该不包含request和response对象）
		hasRequest2 := false
		hasResponse2 := false
		hasRequestBody2 := false
		hasResponseBody2 := false
		hasMetadata2 := false
		count2 := 0
		har.ImportHTTPArchiveStream(fh2, func(h *har.HAREntry) error {
			count2++
			if h.Request != nil {
				hasRequest2 = true
				if h.Request.PostData != nil && len(h.Request.PostData.Text) > 0 {
					hasRequestBody2 = true
				}
			}
			if h.Response != nil {
				hasResponse2 = true
				if h.Response.Content != nil && len(h.Response.Content.Text) > 0 {
					hasResponseBody2 = true
				}
			}
			if h.MetaData != nil {
				hasMetadata2 = true
			}
			return nil
		})

		require.Equal(t, wantCount, count1)
		require.Equal(t, wantCount, count2)

		// 分析结果
		t.Logf("=== HAR导出字段选择分析 ===")
		t.Logf("测试1（不传递FieldName）:")
		t.Logf("  - 包含Request对象: %v", hasRequest1)
		t.Logf("  - 包含Response对象: %v", hasResponse1)
		t.Logf("  - 包含请求体: %v", hasRequestBody1)
		t.Logf("  - 包含响应体: %v", hasResponseBody1)
		t.Logf("测试2（传递错误字段名: %v）:", wrongFieldNames)
		t.Logf("  - 包含Request对象: %v", hasRequest2)
		t.Logf("  - 包含Response对象: %v", hasResponse2)
		t.Logf("  - 包含请求体: %v", hasRequestBody2)
		t.Logf("  - 包含响应体: %v", hasResponseBody2)
		t.Logf("  - 包含MetaData对象: %v", hasMetadata2)
		t.Logf("问题分析:")
		t.Logf("  - 不传递FieldName时，hasField返回false，应该不包含任何字段")
		t.Logf("  - 传递了'method'和'url'字段 → Request对象会被创建（但只有URL字段有值）")
		t.Logf("  - 传递了'status_code'字段 → Response对象会被创建（但只有StatusCode和StatusText字段有值）")
		t.Logf("  - 传递了'path'字段 → MetaData对象应该存在")

		// 验证：测试1不应该包含任何字段，测试2不应该包含response对象
		// 注意：测试2传递了method和url字段，这些字段会触发创建Request对象（但只有URL字段有值）
		// 注意：测试2传递了status_code字段，这个字段会触发创建Response对象（但只有StatusCode和StatusText字段有值）
		require.False(t, hasRequest1, "不传递FieldName时不应该包含Request对象")
		require.False(t, hasResponse1, "不传递FieldName时不应该包含Response对象")
		require.True(t, hasRequest2, "传递了method和url字段时会创建Request对象（但只有URL字段有值）")
		require.True(t, hasResponse2, "传递了status_code字段时会创建Response对象（但只有StatusCode和StatusText字段有值）")
		require.True(t, hasMetadata2, "传递'path'字段时应该包含MetaData对象")
	})

	t.Run("test correct field names with request and response", func(t *testing.T) {
		// 这个测试验证传递正确的字段名时，应该包含body内容
		fn := filepath.Join(t.TempDir(), "test_correct_fieldname.har")
		correctFieldNames := []string{"request", "response", "path", "from_plugin", "tags"}
		stream, err := client.ExportHTTPFlowStream(utils.TimeoutContextSeconds(10), &ypb.ExportHTTPFlowStreamRequest{
			FieldName: correctFieldNames, // ✅ 包含"request"和"response"
			Filter: &ypb.QueryHTTPFlowRequest{
				Keyword: token,
			},
			ExportType: "har",
			TargetPath: fn,
		})
		require.NoError(t, err)
		progress := 0.0
		for {
			msg, err := stream.Recv()
			if err != nil {
				break
			}
			progress = msg.Percent
		}
		require.Equal(t, 1.0, progress)

		fh, err := os.Open(fn)
		require.NoError(t, err)
		defer fh.Close()

		hasRequestBody := false
		hasResponseBody := false
		hasMetadata := false
		count := 0
		har.ImportHTTPArchiveStream(fh, func(h *har.HAREntry) error {
			count++
			if h.Request != nil && h.Request.PostData != nil && len(h.Request.PostData.Text) > 0 {
				hasRequestBody = true
			}
			if h.Response != nil && h.Response.Content != nil && len(h.Response.Content.Text) > 0 {
				hasResponseBody = true
			}
			if h.MetaData != nil && h.MetaData.Path != "" {
				hasMetadata = true
			}
			return nil
		})

		require.Equal(t, wantCount, count)
		require.True(t, hasRequestBody, "传递'request'字段时应该包含请求体")
		require.True(t, hasResponseBody, "传递'response'字段时应该包含响应体")
		require.True(t, hasMetadata, "传递'path'字段时应该包含metadata")
	})
}
