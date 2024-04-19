package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"testing"
	"time"
)

func TestFuzzer_Smuggle(t *testing.T) {
	req := []byte("POST / HTTP/1.1\r\n" +
		"Host: 127.0.0.1:8888\r\n" +
		"User-Agent: python-requests/2.31.0\r\n" +
		"Accept-Encoding: identity\r\n" +
		"Transfer-Encoding: chunked\r\n\r\n" +
		"60\r\nGET /flag HTTP/1.1\r\n" +
		"Host: 127.0.0.1:8888\r\n" +
		"Upgrade: h2c\r\n" +
		"Http2-settings: AAMAAABkAARAAAAAAAIAAAAA\r\n" +
		"0\r\n\r\n")
	rsp := []byte("HTTP/1.1 200 OK\r\n" +
		"Date: Fri, 19 Apr 2024 03:19:38 GMT\r\n" +
		"Transfer-Encoding: chunked\r\n\r\n" +
		"6f\r\nReceived data: GET /flag HTTP/1.1\r\n" +
		"Host: 127.0.0.1:8888\r\n" +
		"Upgrade: h2c\r\n" +
		"Http2-settings: AAMAAABkAARAAAAAAAIAAAAA\r\n0")
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
		writer.Write(rsp)
		time.Sleep(1 * time.Second)
	})
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request:                  string(req),
		ForceFuzz:                false,
		PerRequestTimeoutSeconds: 0.6,
		ActualAddr:               utils.HostPort(host, port),
		NoFixContentLength:       true,
	})
	if err != nil {
		t.Fatal(err)
	}

	pass := false
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if len(rsp.ResponseRaw) > 0 {
			pass = true
		}
	}
	if !pass {
		t.Fatal("no response")
	}
}
