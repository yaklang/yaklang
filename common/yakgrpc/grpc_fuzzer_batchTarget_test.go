package yakgrpc

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestGRPCMUSTPASS_BatchTarget(t *testing.T) {
	var newTarget []string
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	for i := 0; i < 2; i++ {
		host, port := utils.DebugMockHTTPHandlerFuncContext(ctx, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("abc"))
		})
		newTarget = append(newTarget, utils.HostPort(host, port))
		if i%2 == 1 {
			tlsHost, tlsPort := utils.DebugMockHTTPSEx(func(req []byte) []byte {
				return []byte(`HTTP/1.1 200 OK
Content-Length: 3

abc`)
			})
			newTarget = append(newTarget, "https://"+utils.HostPort(tlsHost, tlsPort))
		}
	}

	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	firstTarget := newTarget[0]
	count := 0
	spew.Dump(newTarget)
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
		fmt.Println(rsp.Url)
		fmt.Println(string(rsp.ResponseRaw))
		body := lowhttp.GetHTTPPacketBody(rsp.ResponseRaw)
		if string(body) != "abc" {
			panic("body not match")
		}
		count++
	}
	t.Logf("BatchTarget + origin total count: %v", count)
	if count != 4 {
		t.Fatalf("expect 4, got %v", count)
	}

	count = 0
	spew.Dump(newTarget)
	fp, err := consts.TempFile("batchTarget-*")
	if err != nil {
		panic(err)
	}
	fp.WriteString(strings.Join(newTarget, "\n"))
	fp.Close()
	stream, err = client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request:         "GET /ab HTTP/1.1\r\nHost: " + firstTarget + "\r\n\r\n",
		BatchTargetFile: true,
		BatchTarget:     []byte(fp.Name()),
	})
	if err != nil {
		panic(err)
	}
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		fmt.Println(rsp.Url)
		fmt.Println(string(rsp.ResponseRaw))
		body := lowhttp.GetHTTPPacketBody(rsp.ResponseRaw)
		if string(body) != "abc" {
			panic("body not match")
		}
		count++
	}
	t.Logf("BatchTarget + origin total count: %v", count)
	if count != 4 {
		t.Fatalf("expect 4, got %v", count)
	}
}
