package lowhttp

import (
	"bytes"
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
