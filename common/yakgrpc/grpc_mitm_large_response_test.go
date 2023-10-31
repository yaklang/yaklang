package yakgrpc

import (
	"context"
	"fmt"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"os"
	"strings"
	"testing"
)

func TestLARGEGRPCMUSTPASS_LARGE_RESPONSE_FOR_WEBFUZZER_NEGATIVE(t *testing.T) {
	var port int
	var ctx, cancel = context.WithCancel(utils.TimeoutContextSeconds(60))
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
	var port int
	var ctx, cancel = context.WithCancel(utils.TimeoutContextSeconds(60))
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
	stream, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
		Request: `GET /misc/response/content_length?cl=` + codec.Itoa(expectedCL) + `&c=` + token + ` HTTP/1.1
Host: ` + vulinboxAddr + "\r\n\r\n",
		MaxBodySize: int64(expectedCL - 1),
	})
	if err != nil {
		t.Fatal(err)
	}
	rsp, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if rsp.IsTooLargeResponse && rsp.TooLargeResponseHeaderFile != "" && rsp.TooLargeResponseBodyFile != "" {
		raw, _ := os.ReadFile(rsp.TooLargeResponseBodyFile)
		if len(raw) >= expectedCL {
			dataResponse, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{Keyword: token})
			if err != nil {
				t.Fatal(err)
			}
			if len(dataResponse.Data) != 1 {
				t.Fatal("query taged flow failed(count is not right)")
			}
			if len(dataResponse.Data[0].Response) > expectedCL-10 {
				t.Fatal("response is too large")
			}
			if !strings.Contains(string(dataResponse.Data[0].Response), `find more in web fuzzer history`) {
				t.Fatal("response is not right")
			}
			return
		} else {
			t.Fatal("too-large-response body file not right")
		}
	}
	t.Fatal("too-large-response not found(MaxBodySize is not right)")
}

func TestLARGEGRPCMUSTPASS_LARGE_RESPOSNE_NEGATIVE(t *testing.T) {
	var port int
	var ctx, cancel = context.WithCancel(utils.TimeoutContextSeconds(60))
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
			_, data, err := yakit.QueryHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{
				Keyword: token,
			})
			if err != nil {
				t.Fatal("query taged flow failed", err)
			}
			if len(data) != 1 {
				t.Fatal("query taged flow failed(count is not right)")
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
			ins, _ := raw.ToGRPCModel(true)
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
	var ctx, cancel = context.WithCancel(utils.TimeoutContextSeconds(60))
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
assert len(rsp) > 1111100`,
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
			if err != nil {
				t.Fatal("query taged flow failed", err)
			}
			if len(data) != 1 {
				t.Fatal("query taged flow failed(count is not right)")
			}
			if data[0].BodyLength < 111110000 {
				t.Fatal("query taged flow failed")
			}
			if !data[0].IsTooLargeResponse {
				t.Fatal("too-large-response tag not found")
			}
			bodyFile := data[0].TooLargeResponseBodyFile
			if bodyFile == "" {
				t.Fatal("too-large-response body file not found")
			}
			rawBody, _ := os.ReadFile(bodyFile)
			if len(rawBody) < 111110000 {
				t.Fatal("too-large-response body file not found")
			}
			headerFile := data[0].TooLargeResponseHeaderFile
			if headerFile == "" {
				t.Fatal("too-large-response header file not found")
			}
			rawHeader, _ := os.ReadFile(headerFile)
			if len(rawHeader) < 100 {
				t.Fatal("too-large-response header file not found")
			}
			raw, err := yakit.GetHTTPFlow(consts.GetGormProjectDatabase(), int64(data[0].ID))
			if err != nil {
				t.Fatal("query taged flow failed", err)
			}
			ins, _ := raw.ToGRPCModel(true)
			if len(ins.Response) > 111110000 {
				t.Fatal("query taged flow failed")
			}
			if len(ins.Response) == 0 {
				t.Fatal("query taged flow failed")
			}
			if !(ins.TooLargeResponseBodyFile != "" && ins.TooLargeResponseHeaderFile != "" && ins.IsTooLargeResponse) {
				t.Fatal("too-large-response is not effect")
			}
		}),
	)
}
