package yakgrpc

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"strings"
	"testing"
)

func TestGRPCMUSTPASS_MITM_InvalidUTF8RequestDetail(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(40))
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

	token := utils.RandSecret(100)
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
			packet := []byte(fmt.Sprintf(`POST /post HTTP/1.1
Host: %s
Connection: keep-alive
content-type: multipart/form-data; boundary=1fcd4320db1b046c72582c29ff18e36c

--1fcd4320db1b046c72582c29ff18e36c
Content-Disposition: form-data; name="files"; filename="1.xlsx"

%s
--1fcd4320db1b046c72582c29ff18e36c--`,
				utils.HostPort(host, port),
				"\xff\xff\xff\xff",
			))
			_, err := lowhttp.HTTP(
				lowhttp.WithPacketBytes(packet),
				lowhttp.WithProxy("http://"+utils.HostPort("127.0.0.1", mitmPort)),
				lowhttp.WithHost("127.0.0.1"),
				lowhttp.WithPort(mitmPort),
			)
			if err != nil {
				spew.Dump(err)
				t.Fatal("lowhttp mitm proxy failed")
			}
			cancel()
			break
		}
	}
	flows, err := QueryHTTPFlows(utils.TimeoutContextSeconds(3), client, &ypb.QueryHTTPFlowRequest{
		Keyword: token,
		Pagination: &ypb.Paging{
			Page:  1,
			Limit: 1,
		},
	}, 1)
	require.NoError(t, err)

	if len(flows.Data) == 0 {
		t.Fatal("httpflow not found")
	}

	if !strings.Contains(flows.Data[0].SafeHTTPRequest, `{{unquote("\xff\xff\xff\xff")}}`) {
		t.Fatalf("safe HTTP request not found quote tags: %#v", flows.Data[0].SafeHTTPRequest)
	}
}

func TestGRPCMUSTPASS_MITMV2_InvalidUTF8RequestDetail(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(40))
	defer cancel()
	stream, err := client.MITMV2(ctx)
	if err != nil {
		t.Fatal(err)
	}

	mitmPort := utils.GetRandomAvailableTCPPort()
	stream.Send(&ypb.MITMV2Request{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})

	token := utils.RandSecret(100)
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
			packet := []byte(fmt.Sprintf(`POST /post HTTP/1.1
Host: %s
Connection: keep-alive
content-type: multipart/form-data; boundary=1fcd4320db1b046c72582c29ff18e36c

--1fcd4320db1b046c72582c29ff18e36c
Content-Disposition: form-data; name="files"; filename="1.xlsx"

%s
--1fcd4320db1b046c72582c29ff18e36c--`,
				utils.HostPort(host, port),
				"\xff\xff\xff\xff",
			))
			_, err := lowhttp.HTTP(
				lowhttp.WithPacketBytes(packet),
				lowhttp.WithProxy("http://"+utils.HostPort("127.0.0.1", mitmPort)),
				lowhttp.WithHost("127.0.0.1"),
				lowhttp.WithPort(mitmPort),
			)
			if err != nil {
				spew.Dump(err)
				t.Fatal("lowhttp mitm proxy failed")
			}
			cancel()
			break
		}
	}
	flows, err := QueryHTTPFlows(utils.TimeoutContextSeconds(3), client, &ypb.QueryHTTPFlowRequest{
		Keyword: token,
		Pagination: &ypb.Paging{
			Page:  1,
			Limit: 1,
		},
	}, 1)
	require.NoError(t, err)

	if len(flows.Data) == 0 {
		t.Fatal("httpflow not found")
	}

	if !strings.Contains(flows.Data[0].SafeHTTPRequest, `{{unquote("\xff\xff\xff\xff")}}`) {
		t.Fatalf("safe HTTP request not found quote tags: %#v", flows.Data[0].SafeHTTPRequest)
	}
}
