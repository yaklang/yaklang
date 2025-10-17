package lowhttp

import (
	"bytes"
	"github.com/davecgh/go-spew/spew"
	"strings"
	"testing"
)

func TestFixHTTPResponse2(t *testing.T) {
	rsp, body, err := FixHTTPResponse([]byte(`HTTP/1.1 200 OK
Transfer-Encoding: chunked

0` + "\r\n\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != "" {
		t.Fatal("body should be empty")
	}
	if !bytes.HasSuffix(rsp, []byte("Content-Length: 0\r\n\r\n")) {
		t.Fatal("rsp should end with Content-Length: 0\\r\\n\\r\\n")
	}
}

func TestFixHTTPResponse_100Continue(t *testing.T) {
	rsp, body, err := FixHTTPResponse([]byte("HTTP/1.1 100 Continue\r\n\r\n" + `HTTP/1.1 200 OK
Transfer-Encoding: chunked

0` + "\r\n\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != "" {
		t.Fatal("body should be empty")
	}
	if !bytes.HasSuffix(rsp, []byte("Content-Length: 0\r\n\r\n")) {
		t.Fatal("rsp should end with Content-Length: 0\\r\\n\\r\\n")
	}
	if bytes.Contains(rsp, []byte("HTTP/1.1 100 Continue")) {
		t.Fatal("rsp should not contain HTTP/1.1 100 Continue")
	}
}

func TestFixHTTPResponse3(t *testing.T) {
	rsp, body, err := FixHTTPResponse([]byte(`HTTP/1.1 200 OK
Transfer-Encoding: chunked

1` + "\r\na\r\n" + `0` + "\r\n\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != "a" {
		t.Fatal("body not right")
	}
	if !bytes.HasSuffix(rsp, []byte("Content-Length: 1\r\n\r\na")) {
		t.Fatal("rsp should end with Content-Length: 1\\r\\n\\r\\na")
	}
}

func TestFixHTTPResponse4(t *testing.T) {
	packet2 := "  HTTP/1.1 200 OK\r\n" +
		"    Server: nginx/abc.111\r\n" +
		"    Content-Length: 3\r\n" +
		"    \r\n" +
		"    abc"
	rsp, body, err := FixHTTPResponse([]byte(packet2))
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(rsp)
	spew.Dump(body)
}
