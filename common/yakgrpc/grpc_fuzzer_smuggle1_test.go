package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestFuzzer_LowhttpFixRequest(t *testing.T) {
	req := []byte("POST / HTTP/1.1\r\n" +
		"Host: 127.0.0.1:8888\r\n" +
		"User-Agent: python-requests/2.31.0\r\n" +
		"Accept-Encoding: gzip, deflate\r\n" +
		"Accept: */*\r\n" +
		"Transfer-Encoding: chunked\r\n\r\n" +
		"60\r\nGET /flag HTTP/1.1\r\n" + // not 60, instead of 62
		"Host: 127.0.0.1:8888\r\n" +
		"Upgrade: h2c\r\n" +
		"Http2-settings: AAMAAABkAARAAAAAAAIAAAAA\r\n\r\n" +
		"0\r\n\r\n")
	if ret := lowhttp.FixHTTPRequest(req); bytes.Contains(ret, []byte("660")) {
		t.Fatal("fix request failed")
	} else {
		spew.Dump(ret)
		fmt.Println(string(ret))
	}
}

func TestFuzzer_LowhttpFixRequest2(t *testing.T) {
	req := []byte("POST / HTTP/1.1\r\n" +
		"Host: 127.0.0.1:8888\r\n" +
		"User-Agent: python-requests/2.31.0\r\n" +
		"Accept-Encoding: gzip, deflate\r\n" +
		"Accept: */*\r\n" +
		"Transfer-Encoding: chunked\r\n\r\n" +
		"3\r\naaa\r\n" +
		"0\r\n")
	if ret := lowhttp.FixHTTPRequest(req); bytes.Contains(ret, []byte("33")) {
		t.Fatal("fix request failed")
	} else {
		spew.Dump(ret)
		if !bytes.HasSuffix(ret, []byte("\r\n0\r\n\r\n")) {
			t.Fatal("fix request failed")
		}
		fmt.Println(string(ret))
	}
}

func TestFuzzer_LowhttpFixRequest3(t *testing.T) {
	req := []byte("POST / HTTP/1.1\r\n" +
		"Host: 127.0.0.1:8888\r\n" +
		"User-Agent: python-requests/2.31.0\r\n" +
		"Accept-Encoding: gzip, deflate\r\n" +
		"Accept: */*\r\n" +
		"Transfer-Encoding: chunked\r\n\r\n" +
		"3\r\naaa\r\n" +
		"0")
	if ret := lowhttp.FixHTTPRequest(req); bytes.Contains(ret, []byte("33")) {
		t.Fatal("fix request failed")
	} else {
		spew.Dump(ret)
		if !bytes.HasSuffix(ret, []byte("\r\n0\r\n\r\n")) {
			t.Fatal("fix request failed")
		}
		fmt.Println(string(ret))
	}
}

func TestFuzzer_LowhttpFixRequest4(t *testing.T) {
	req := []byte("POST / HTTP/1.1\r\n" +
		"Host: 127.0.0.1:8888\r\n" +
		"User-Agent: python-requests/2.31.0\r\n" +
		"Accept-Encoding: gzip, deflate\r\n" +
		"Accept: */*\r\n" +
		"Transfer-Encoding: chunked\r\n\r\n" +
		"3\r\naaa\r\n" +
		"0\r\n\r\nabc")
	if ret := lowhttp.FixHTTPRequest(req); bytes.Contains(ret, []byte("33")) {
		t.Fatal("fix request failed")
	} else {
		spew.Dump(ret)
		if !bytes.HasSuffix(ret, []byte("\r\n0\r\n\r\nabc")) {
			t.Fatal("fix request failed")
		}
		fmt.Println(string(ret))
	}
}

func TestFuzzer_LowhttpFixRequest5(t *testing.T) {
	req := []byte("POST / HTTP/1.1\r\n" +
		"Host: 127.0.0.1:8888\r\n" +
		"User-Agent: python-requests/2.31.0\r\n" +
		"Accept-Encoding: gzip, deflate\r\n" +
		"Accept: */*\r\n" +
		"Transfer-Encoding: chunked\r\n\r\n" +
		"60\r\naaa\r\n" +
		"0\r\n\r\nabc")
	fixed := lowhttp.FixHTTPRequest(req)
	require.NotContains(t, fixed, []byte("33"), "fix request failed")
	spew.Dump(fixed)
	require.True(t, bytes.HasSuffix(fixed, []byte("\r\n60\r\naaa\r\n0\r\n\r\nabc")), "fix request failed")
}

func TestFuzzer_Smuggle(t *testing.T) {
	req := []byte("POST / HTTP/1.1\r\n" +
		"Host: 127.0.0.1:8888\r\n" +
		"User-Agent: python-requests/2.31.0\r\n" +
		"Accept-Encoding: gzip, deflate\r\n" +
		"Accept: */*\r\n" +
		"Transfer-Encoding: chunked\r\n\r\n" +
		"60\r\nGET /flag HTTP/1.1\r\n" +
		"Host: 127.0.0.1:8888\r\n" +
		"Upgrade: h2c\r\n" +
		"Http2-settings: AAMAAABkAARAAAAAAAIAAAAA\r\n\r\n" +
		"0\r\n\r\n")

	r := mux.NewRouter()
	r.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 设置Header以支持Chunked编码的响应
		w.Header().Set("Transfer-Encoding", "chunked")

		// 读取请求体
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading request body", http.StatusInternalServerError)
			return
		}

		// 输出接收到的数据
		log.Printf("Received request with data: %s", body)

		// 向客户端写入响应
		w.Write([]byte("Received data: "))
		w.Write(body)
	}))
	target := utils.GetRandomLocalAddr()
	lis, err := net.Listen("tcp", target)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		if err := http.Serve(lis, r); err != nil {
			log.Error(err)
		}
	}()
	err = utils.WaitConnect(target, 2)
	if err != nil {
		t.Fatal(err)
	}

	//rsp := []byte("HTTP/1.1 200 OK\r\n" +
	//	"Date: Fri, 19 Apr 2024 03:19:38 GMT\r\n" +
	//	"Transfer-Encoding: chunked\r\n\r\n" +
	//	"6f\r\nReceived data: GET /flag HTTP/1.1\r\n" +
	//	"Host: 127.0.0.1:8888\r\n" +
	//	"Upgrade: h2c\r\n" +
	//	"Http2-settings: AAMAAABkAARAAAAAAAIAAAAA\r\n0")
	//host, port := utils.DebugMockTCPEx(func(ctx context.Context, lis net.Listener, conn net.Conn) {
	//	conn.Write(rsp)
	//	time.Sleep(time.Second)
	//})
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(target)
	stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request:                  string(req),
		ForceFuzz:                false,
		PerRequestTimeoutSeconds: 0.6,
		ActualAddr:               target,
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
		fmt.Println(string(req))
		fmt.Println("------------------------------------")
		spew.Dump(lowhttp.FixHTTPRequest(req))
		fmt.Println("------------------------------------")
		fmt.Println(string(lowhttp.FixHTTPRequest(req)))
		fmt.Println("------------------------------------")
		spew.Dump(rsp.GetResponseRaw())
		if len(rsp.ResponseRaw) > 0 {
			pass = true
		}
	}
	if !pass {
		t.Fatal("no response")
	}
}
