package yakgrpc

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_HTTPFUZZER_BatchTarget(t *testing.T) {
	var newTarget []string
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	for i := 0; i < 2; i++ {
		host, port := utils.DebugMockHTTPHandlerFuncContext(ctx, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("abc"))
		})
		newTarget = append(newTarget, utils.HostPort(host, port))
		if i%2 == 1 {
			tlsHost, tlsPort := utils.DebugMockHTTPSKeepAliveEx(func(req []byte) []byte {
				return []byte(`HTTP/1.1 200 OK
Content-Length: 3

abc`)
			})
			newTarget = append(newTarget, "https://"+utils.HostPort(tlsHost, tlsPort))
		}
	}

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	firstTarget := newTarget[0]
	count := 0
	stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request:         "GET /ab HTTP/1.1\r\nHost: " + firstTarget + "\r\n\r\n",
		BatchTargetFile: false,
		BatchTarget:     []byte(strings.Join(newTarget, "\n")),
	})
	if err != nil {
		panic(err)
	}
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		body := lowhttp.GetHTTPPacketBody(rsp.ResponseRaw)
		if string(body) != "abc" {
			t.Fatalf("body not match. body:\n%s", string(body))
		}
		count++
	}
	t.Logf("BatchTarget + origin total count: %v", count)
	if count != 4 {
		t.Fatalf("expect 4, got %v", count)
	}

	count = 0
	fp, err := consts.TempFile("batchTarget-*")
	if err != nil {
		t.Fatal(err)
	}
	fp.WriteString(strings.Join(newTarget, "\n"))
	fp.Close()
	stream, err = client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request:         "GET /ab HTTP/1.1\r\nHost: " + firstTarget + "\r\n\r\n",
		BatchTargetFile: true,
		BatchTarget:     []byte(fp.Name()),
	})
	if err != nil {
		t.Fatal(err)
	}
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		body := lowhttp.GetHTTPPacketBody(rsp.ResponseRaw)
		if string(body) != "abc" {
			t.Fatalf("body not match. body:\n%s", string(body))
		}
		count++
	}
	t.Logf("BatchTarget + origin total count: %v", count)
	if count != 4 {
		t.Fatalf("expect 4, got %v", count)
	}
}

// when batchTarget has full url without port, it make Host header same as batchTarget
// Input:
// batchTarget = "http://127.0.0.1"
// Expect:
// Host: 127.0.0.1
// Got:
// Host: http://127.0.0.1
func TestGRPCMUSTPASS_HTTPFUZZER_BatchTarget_FixBUG(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := NewLocalClient()
	require.NoError(t, err)
	mitmHost, mitmPort := "127.0.0.1", utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort(mitmHost, mitmPort)
	batchTarget := "http://127.0.0.1"
	recvRequest := false

	RunMITMTestServerEx(client, ctx,
		func(stream ypb.Yak_MITMClient) {
			stream.Send(&ypb.MITMRequest{
				Host: mitmHost,
				Port: uint32(mitmPort),
			})
			stream.Send(&ypb.MITMRequest{
				SetAutoForward:   true,
				AutoForwardValue: false,
			})
		},
		func(stream ypb.Yak_MITMClient) {
			// Wait for SetAutoForward configuration to take effect before sending request
			time.Sleep(200 * time.Millisecond)
			fuzzerStream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
				Request:         "GET / HTTP/1.1\r\nHost: www.example.com\r\n\r\n",
				BatchTargetFile: false,
				BatchTarget:     []byte(batchTarget), // cause bug
				Proxy:           proxy,
			})
			require.NoError(t, err)

			for {
				_, err := fuzzerStream.Recv()
				if err != nil {
					break
				}
			}
			cancel()
		}, func(stream ypb.Yak_MITMClient, grpcRsp *ypb.MITMResponse) {
			if reqRaw := grpcRsp.GetRequest(); len(reqRaw) > 0 {
				stream.Send(&ypb.MITMRequest{
					Id:   grpcRsp.GetId(),
					Drop: true,
				})
				if strings.Contains(string(reqRaw), "www.example.com") {
					// do not check raw packet
					return
				}
				recvRequest = true
				require.NotContains(t, string(reqRaw), "Host: "+batchTarget, "batchTarget should not be used as Host header")
				require.Contains(t, string(reqRaw), "Host: 127.0.0.1", "host should be 127.0.0.1")
			}
		})
	require.True(t, recvRequest, "mitm not recv fuzzer request")
}
