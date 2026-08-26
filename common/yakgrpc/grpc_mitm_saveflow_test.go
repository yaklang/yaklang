package yakgrpc

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func queryInvalidUTF8MITMFlow(t *testing.T, client ypb.YakClient, token string) *ypb.QueryHTTPFlowResponse {
	t.Helper()
	var flows *ypb.QueryHTTPFlowResponse
	err := utils.AttemptWithDelay(20, 250*time.Millisecond, func() error {
		var queryErr error
		flows, queryErr = client.QueryHTTPFlows(utils.TimeoutContextSeconds(2), &ypb.QueryHTTPFlowRequest{
			SearchURL: token,
			Pagination: &ypb.Paging{
				Page:  1,
				Limit: 1,
			},
		})
		if queryErr != nil {
			return queryErr
		}
		if len(flows.GetData()) != 1 {
			return utils.Errorf("expect 1, got %d", len(flows.GetData()))
		}
		return nil
	})
	if err != nil {
		var directCount int
		directErr := consts.GetGormProjectDatabase().Model(&schema.HTTPFlow{}).
			Where("url LIKE ?", "%"+token+"%").Count(&directCount).Error
		t.Fatalf("query invalid UTF-8 MITM flow: %v (direct database count=%d, direct error=%v)", err, directCount, directErr)
	}
	return flows
}

func buildInvalidUTF8MITMRequest(token, target string) []byte {
	const boundary = "1fcd4320db1b046c72582c29ff18e36c"
	body := []byte(fmt.Sprintf("--%s\r\nContent-Disposition: form-data; name=\"files\"; filename=\"1.xlsx\"\r\n\r\n%s\r\n--%s--\r\n",
		boundary,
		"\xff\xff\xff\xff",
		boundary,
	))
	header := []byte(fmt.Sprintf("POST /post?token=%s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\nContent-Type: multipart/form-data; boundary=%s\r\nContent-Length: %d\r\n\r\n",
		token,
		target,
		boundary,
		len(body),
	))
	return append(header, body...)
}

func TestGRPCMUSTPASS_MITM_InvalidUTF8RequestDetail(t *testing.T) {
	client := isolateMITMTestSideEffects(t)
	token := "mitm-invalid-utf8-" + utils.RandSecret(32)
	registerHTTPFlowTokenCleanup(t, token)
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(40))
	t.Cleanup(cancel)
	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}

	mitmPort := utils.GetRandomAvailableTCPPort()
	stream.Send(&ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})

	host, port := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(token))
	})

	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}

		msg := string(rsp.GetMessage().GetMessage())
		fmt.Println(msg)
		if strings.Contains(msg, `starting mitm server`) {
			packet := buildInvalidUTF8MITMRequest(token, utils.HostPort(host, port))
			_, err := lowhttp.HTTP(
				lowhttp.WithPacketBytes(packet),
				lowhttp.WithProxy("http://"+utils.HostPort("127.0.0.1", mitmPort)),
				lowhttp.WithHost("127.0.0.1"),
				lowhttp.WithPort(mitmPort),
				lowhttp.WithSaveHTTPFlow(false),
			)
			if err != nil {
				spew.Dump(err)
				t.Fatal("lowhttp mitm proxy failed")
			}
			// Keep the proxy alive until the Flow has been observed below.
			break
		}
	}
	flows := queryInvalidUTF8MITMFlow(t, client, token)

	if len(flows.Data) == 0 {
		t.Fatal("httpflow not found")
	}

	if !strings.Contains(flows.Data[0].SafeHTTPRequest, `{{unquote("\xff\xff\xff\xff")}}`) {
		t.Fatalf("safe HTTP request not found quote tags: %#v", flows.Data[0].SafeHTTPRequest)
	}
}

func TestGRPCMUSTPASS_MITMV2_InvalidUTF8RequestDetail(t *testing.T) {
	client := isolateMITMTestSideEffects(t)
	token := "mitm-invalid-utf8-" + utils.RandSecret(32)
	registerHTTPFlowTokenCleanup(t, token)
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(40))
	t.Cleanup(cancel)
	stream, err := client.MITMV2(ctx)
	if err != nil {
		t.Fatal(err)
	}

	mitmPort := utils.GetRandomAvailableTCPPort()
	stream.Send(&ypb.MITMV2Request{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})

	host, port := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(token))
	})

	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}

		msg := string(rsp.GetMessage().GetMessage())
		fmt.Println(msg)
		if strings.Contains(msg, `starting mitm server`) {
			packet := buildInvalidUTF8MITMRequest(token, utils.HostPort(host, port))
			_, err := lowhttp.HTTP(
				lowhttp.WithPacketBytes(packet),
				lowhttp.WithProxy("http://"+utils.HostPort("127.0.0.1", mitmPort)),
				lowhttp.WithHost("127.0.0.1"),
				lowhttp.WithPort(mitmPort),
				lowhttp.WithSaveHTTPFlow(false),
			)
			if err != nil {
				spew.Dump(err)
				t.Fatal("lowhttp mitm proxy failed")
			}
			// Keep the MITM stream alive until the asynchronous flow writer has
			// published the row. Canceling here races the save callback and makes
			// the detail assertion depend on machine/DB speed.
			break
		}
	}
	flows := queryInvalidUTF8MITMFlow(t, client, token)

	if len(flows.Data) == 0 {
		t.Fatal("httpflow not found")
	}

	if !strings.Contains(flows.Data[0].SafeHTTPRequest, `{{unquote("\xff\xff\xff\xff")}}`) {
		t.Fatalf("safe HTTP request not found quote tags: %#v", flows.Data[0].SafeHTTPRequest)
	}
}
