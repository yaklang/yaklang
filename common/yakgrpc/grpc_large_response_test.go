package yakgrpc

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp"

	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/model"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestLARGEGRPCMUSTPASS_LARGE_RESPONSE_FOR_WEBFUZZER_NEGATIVE(t *testing.T) {
	var port int
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(60))
	defer cancel()
	addr, err := vulinbox.NewVulinServerEx(ctx, true, false, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	host, port, _ := utils.ParseStringToHostPort(addr)
	vulinboxAddr := utils.HostPort(host, port)

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	token := ksuid.New().String()
	expectedCL := 4 * 1000 * 1000
	limit := int64(expectedCL + 1000)
	stream, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
		Request: `GET /misc/response/content_length?cl=` + codec.Itoa(expectedCL) + `&c=` + token + ` HTTP/1.1
Host: ` + vulinboxAddr + "\r\n\r\n",
		MaxBodySize: limit,
	})
	if err != nil {
		t.Fatal(err)
	}
	rsp, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}

	if rsp.IsTooLargeResponse {
		t.Fatal("too-large-response tag not right")
	}

	if rsp.TooLargeResponseHeaderFile != "" {
		t.Fatal("too-large-response header file not right")
	}

	if rsp.TooLargeResponseBodyFile != "" {
		t.Fatal("too-large-response body file not right")
	}

	dataResponse, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{Keyword: token})
	if err != nil {
		t.Fatal(err)
	}
	if len(dataResponse.Data) != 1 {
		t.Fatal("query taged flow failed(count is not right)")
	}

	if len(dataResponse.Data[0].Response) != 0 {
		t.Fatal("large response is not show(500K)")
	}
	id := dataResponse.Data[0].Id
	if id == 0 {
		t.Fatal("id is 0")
	}

	response, err := client.GetHTTPFlowById(context.Background(), &ypb.GetHTTPFlowByIdRequest{
		Id: int64(id),
	})
	if err != nil {
		t.Fatal(err)
	}
	if l := len(response.Response); l > 0 && l > expectedCL && int64(l) < limit {
		return
	} else {
		t.Fatal("response is not right")
	}
}

func TestLARGEGRPCMUSTPASS_LARGE_RESPONSE_FOR_WEBFUZZER_POSITIVE(t *testing.T) {
	// use debug Mock HTTP instead of vulinbox, because debug Mock HTTP will close connection immediately after response
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(60))
	defer cancel()

	client, err := NewLocalClient()
	require.NoError(t, err)

	token := uuid.NewString()
	expectedCL := 4 * 1000 * 1000
	largeText := strings.Repeat("a", expectedCL)
	// start Mock HTTP server
	host, port := utils.DebugMockHTTP([]byte(fmt.Sprintf(`HTTP/1.1?token=%s 200 OK
Server: test
Content-Length: %d

%s`, token, len(largeText), largeText)))

	// start fuzz
	stream, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
		Request: `GET / HTTP/1.1
Host: ` + utils.HostPort(host, port),
		MaxBodySize: int64(expectedCL - 1),
	})
	require.NoError(t, err)
	httpflowID := int64(0)
	// delete httpflow after test
	defer func() {
		if httpflowID == 0 {
			return
		}
		client.DeleteHTTPFlows(ctx, &ypb.DeleteHTTPFlowRequest{
			Id: []int64{httpflowID},
		})
	}()
	// check return
	rsp, err := stream.Recv()
	require.NoError(t, err)

	fuzzerBody := lowhttp.GetHTTPPacketBody(rsp.ResponseRaw)
	// check fuzzerBody
	require.Equal(t, len(fuzzerBody), expectedCL-1, "response is not right")
	// check large response
	require.True(t, rsp.IsTooLargeResponse)
	require.NotEmpty(t, rsp.TooLargeResponseBodyFile)
	require.NotEmpty(t, rsp.TooLargeResponseHeaderFile)

	// check large response file
	raw, _ := os.ReadFile(rsp.TooLargeResponseBodyFile)
	require.GreaterOrEqual(t, len(raw), expectedCL, "too-large-response body file not right")

	// check database
	dataResponse, err := QueryHTTPFlows(ctx, client, &ypb.QueryHTTPFlowRequest{Keyword: token}, 1)
	require.NoError(t, err)
	// get httpflowID
	httpflowID = int64(dataResponse.Data[0].Id)
	// check response length
	require.LessOrEqual(t, len(dataResponse.Data[0].Response), expectedCL-10, "response is too large")
	// check response truncated
	require.Contains(t, string(dataResponse.Data[0].Response), `[[response too large`)
	// check large response
	require.True(t, dataResponse.Data[0].IsTooLargeResponse)
	require.Equal(t, rsp.TooLargeResponseHeaderFile, dataResponse.Data[0].TooLargeResponseHeaderFile)
	require.Equal(t, rsp.TooLargeResponseBodyFile, dataResponse.Data[0].TooLargeResponseBodyFile)
}

func TestLARGEGRPCMUSTPASS_LARGE_RESPONSE_FOR_WEBFUZZER_CHUNKED_POSITIVE(t *testing.T) {
	// use debug Mock HTTP instead of vulinbox, because debug Mock HTTP will close connection immediately after response
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(60))
	defer cancel()

	client, err := NewLocalClient()
	require.NoError(t, err)

	token := uuid.NewString()
	expectedCL := 4 * 1000 * 1000
	largeText := bytes.Repeat([]byte("z"), expectedCL)
	// start Mock HTTP server
	host, port := utils.DebugMockHTTP([]byte(fmt.Sprintf(`HTTP/1.1?token=%s 200 OK
Server: test
Transfer-Encoding: chunked

%s`, token, codec.HTTPChunkedEncode([]byte(largeText)))))

	// start fuzz
	stream, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
		Request: `GET / HTTP/1.1
Host: ` + utils.HostPort(host, port),
		MaxBodySize: int64(expectedCL - 1),
	})
	require.NoError(t, err)
	httpflowID := int64(0)
	// delete httpflow after test
	defer func() {
		if httpflowID == 0 {
			return
		}
		client.DeleteHTTPFlows(ctx, &ypb.DeleteHTTPFlowRequest{
			Id: []int64{httpflowID},
		})
	}()
	// check return
	rsp, err := stream.Recv()
	require.NoError(t, err)

	fuzzerBody := lowhttp.GetHTTPPacketBody(rsp.ResponseRaw)
	// check fuzzerBody
	require.NotEmpty(t, fuzzerBody, "response is empty")
	// check no chunked
	chunkedHeader := lowhttp.GetHTTPPacketHeaders(rsp.ResponseRaw)
	require.NotContains(t, chunkedHeader, "transfer-encoding", "response is chunked")
	require.NotContains(t, chunkedHeader, "Transfer-Encoding", "response is chunked")
	reader := bufio.NewReader(bytes.NewReader([]byte(fuzzerBody)))
	bodyFirstLine, _, err := reader.ReadLine()
	require.NoError(t, err)
	i, err := strconv.ParseInt(string(bodyFirstLine), 16, 64)
	require.Error(t, err, "response is chunked")
	require.EqualValues(t, i, 0, "response is chunked")
	// check large response
	require.True(t, rsp.IsTooLargeResponse)
	require.NotEmpty(t, rsp.TooLargeResponseHeaderFile, "too-large-response header file not found")
	require.NotEmpty(t, rsp.TooLargeResponseBodyFile, "too-large-response body file not found")

	// check large response file
	raw, _ := os.ReadFile(rsp.TooLargeResponseBodyFile)
	require.GreaterOrEqual(t, len(raw), expectedCL, "too-large-response body file not right")

	// check database
	dataResponse, err := QueryHTTPFlows(ctx, client, &ypb.QueryHTTPFlowRequest{Keyword: token}, 1)
	require.NoError(t, err)
	// get httpflowID
	httpflowID = int64(dataResponse.Data[0].Id)
	// check response length
	require.LessOrEqual(t, len(dataResponse.Data[0].Response), expectedCL-10, "response is too large")
	// check response truncated
	require.Contains(t, string(dataResponse.Data[0].Response), `[[response too large`)
	// check large response
	require.True(t, dataResponse.Data[0].IsTooLargeResponse)
	require.Equal(t, rsp.TooLargeResponseHeaderFile, dataResponse.Data[0].TooLargeResponseHeaderFile)
	require.Equal(t, rsp.TooLargeResponseBodyFile, dataResponse.Data[0].TooLargeResponseBodyFile)
}

func TestLARGEGRPCMUSTPASS_LARGE_RESPOSNE_NEGATIVE(t *testing.T) {
	var port int
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(60))
	defer cancel()
	addr, err := vulinbox.NewVulinServerEx(ctx, true, false, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	host, port, _ := utils.ParseStringToHostPort(addr)
	vulinboxAddr := utils.HostPort(host, port)
	token := uuid.NewString()
	NewMITMTestCase(
		t,
		CaseWithMaxContentLength(5*1000*1000),
		CaseWithContext(ctx),
		CaseWithPort(func(i int) {
			port = i
		}),
		CaseWithServerStart(func() {
			_, err := yak.Execute(
				`rsp, req = poc.HTTP(packet, poc.proxy(mitmProxy), poc.save(false))~;
cancel()
assert len(rsp) > 1111100`,
				map[string]any{
					"packet": `GET /misc/response/content_length?cl=4000000&c=` + token + ` HTTP/1.1
Host: ` + vulinboxAddr + "\r\n\r\n",
					`cancel`:    cancel,
					"mitmProxy": fmt.Sprintf(`http://127.0.0.1:%v`, port),
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			cancel()

			// 等待数据库写入完成（异步操作）
			var data []*schema.HTTPFlow
			maxRetries := 10
			for i := 0; i < maxRetries; i++ {
				_, data, err = yakit.QueryHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{
					Keyword: token,
				})
				if err == nil && len(data) == 1 {
					break
				}
				if i < maxRetries-1 {
					time.Sleep(500 * time.Millisecond)
				}
			}
			if err != nil {
				t.Fatal("query taged flow failed", err)
			}
			if len(data) != 1 {
				t.Fatalf("query taged flow failed(count is not right), expected 1 but got %d after %d retries", len(data), maxRetries)
			}
			if data[0].BodyLength > 4000000 {
				t.Fatal("query taged flow failed")
			}
			if data[0].IsTooLargeResponse {
				t.Fatal("too-large-response tag not right")
			}
			bodyFile := data[0].TooLargeResponseBodyFile
			if bodyFile != "" {
				t.Fatal("too-large-response body file not right")
			}

			headerFile := data[0].TooLargeResponseHeaderFile
			if headerFile != "" {
				t.Fatal("too-large-response header file not right")
			}

			raw, err := yakit.GetHTTPFlow(consts.GetGormProjectDatabase(), int64(data[0].ID))
			if err != nil {
				t.Fatal("query taged flow failed", err)
			}
			ins, _ := model.ToHTTPFlowGRPCModel(raw, true)
			if len(ins.Response) > 5*1000*1000 {
				t.Fatal("query taged flow failed")
			}
			if len(ins.Response) == 0 {
				t.Fatal("query taged flow failed")
			}
			if !ins.DisableRenderStyles {
				t.Fatal("render is disabled!")
			}
			if ins.IsTooLargeResponse {
				t.Fatal("too-large-response tag not right")
			}
		}),
	)
}

func TestLARGEGRPCMUSTPASS_LARGE_RESPOSNE(t *testing.T) {
	var port int
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(60))
	defer cancel()
	addr, err := vulinbox.NewVulinServerEx(ctx, true, false, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	host, port, _ := utils.ParseStringToHostPort(addr)
	vulinboxAddr := utils.HostPort(host, port)
	token := ksuid.New().String()
	NewMITMTestCase(
		t,
		CaseWithMaxContentLength(100),
		CaseWithContext(ctx),
		CaseWithPort(func(i int) {
			port = i
		}),
		CaseWithServerStart(func() {
			_, err := yak.Execute(
				`rsp, req = poc.HTTP(packet, poc.proxy(mitmProxy), poc.save(false))~;
cancel()
assert len(rsp) > 1111100, f"ResponseLength ${len(rsp)}"`,
				map[string]any{
					"packet": `GET /misc/response/content_length?cl=111110000&c=` + token + ` HTTP/1.1
Host: ` + vulinboxAddr + "\r\n\r\n",
					`cancel`:    cancel,
					"mitmProxy": fmt.Sprintf(`http://127.0.0.1:%v`, port),
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			cancel()

			_, data, err := yakit.QueryHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{
				Keyword: token,
			})
			require.NoError(t, err, "query taged flow failed")
			require.Len(t, data, 1, "query taged flow failed(count is not right)")
			require.GreaterOrEqual(t, data[0].BodyLength, int64(111110000), "query taged flow failed")
			require.True(t, data[0].IsTooLargeResponse, "too-large-response tag not found")

			bodyFile := data[0].TooLargeResponseBodyFile
			headerFile := data[0].TooLargeResponseHeaderFile

			require.NotEmpty(t, bodyFile, "too-large-response body file not found")
			rawBody, _ := os.ReadFile(bodyFile)
			require.GreaterOrEqual(t, len(rawBody), 111110000, "too-large-response body file not found")
			if headerFile == "" {
				t.Fatal("too-large-response header file not found")
			}
			rawHeader, _ := os.ReadFile(headerFile)
			require.GreaterOrEqual(t, len(rawHeader), 100, "too-large-response header file not found")

			raw := data[0]
			ins, _ := model.ToHTTPFlowGRPCModel(raw, true)
			require.LessOrEqual(t, len(ins.Response), 111110000, "ins.Response is too large")
			require.Greater(t, len(ins.Response), 0, "ins.Response should not be empty")
			require.NotEmpty(t, ins.TooLargeResponseHeaderFile, "ins.TooLargeResponseHeaderFile should not be empty")
			require.NotEmpty(t, ins.TooLargeResponseBodyFile, "ins.TooLargeResponseBodyFile should not be empty")
		}),
	)
}
