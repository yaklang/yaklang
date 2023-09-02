package utils

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"net/http"
	"testing"
)

func TestHTTPRequestReaderWithBareBytes_1(t *testing.T) {
	packet := []byte(`GET / HTTP/1.1` + CRLF +
		`Host: www.example.com` + CRLF +
		`Content-Length: 3` + CRLF + CRLF + "abccccddef")
	req, err := ReadHTTPRequestFromBytes(packet)
	if err != nil {
		t.Fatal(err)
	}
	if req.ContentLength == 3 {
		t.Fatal("ContentLength should be 10")
	}
	fmt.Println(string(httpctx.GetBareRequestBytes(req)))
	spew.Dump(httpctx.GetBareRequestBytes(req))
	if !MatchAllOfSubString(httpctx.GetBareRequestBytes(req), "Content-Length: 3\r\n\r\nabc", "abccccddef") {
		t.Fatal("Content-Length: 3 and abccccddef should be in the raw request")
	}
}

func TestHTTPResponseReaderWithBareBytes_1(t *testing.T) {
	var req = new(http.Request)
	packet := []byte(`HTTP/1.1 200 OK` + CRLF +
		`Server: www.example.com` + CRLF +
		`Content-Length: 3` + CRLF + CRLF + "abccccddef")
	rsp, err := ReadHTTPResponseFromBytes(packet, req)
	if err != nil {
		t.Fatal(err)
	}
	_ = rsp
	if req.ContentLength == 3 {
		t.Fatal("ContentLength should be 10")
	}
	fmt.Println(string(httpctx.GetBareResponseBytes(req)))
	spew.Dump(httpctx.GetBareResponseBytes(req))
	if !MatchAllOfSubString(httpctx.GetBareResponseBytes(req), "Content-Length: 3\r\n\r\nabc", "abccccddef") {
		t.Fatal("Content-Length: 3 and abccccddef should be in the raw request")
	}
}

func TestHTTPRequestReaderWithBareBytes_2(t *testing.T) {
	packet := []byte(`GET / HTTP/1.1` + CRLF +
		`Host: www.example.com` + CRLF +
		`Content-Length: 3` + CRLF + CRLF + "abc")
	req, err := ReadHTTPRequestFromBytes(packet)
	if err != nil {
		t.Fatal(err)
	}
	if req.ContentLength != 3 {
		t.Fatal("ContentLength should be 3")
	}
	fmt.Println(string(httpctx.GetBareRequestBytes(req)))
	spew.Dump(httpctx.GetBareRequestBytes(req))
	if !MatchAllOfSubString(httpctx.GetBareRequestBytes(req), "Content-Length: 3\r\n\r\nabc") {
		t.Fatal("TestHTTPRequestReaderWithBareBytes_2")
	}
}

func TestHTTPResponseReaderWithBareBytes_2(t *testing.T) {
	var req = new(http.Request)
	packet := []byte(`HTTP/1.1 200 OK` + CRLF +
		`Server: www.example.com` + CRLF +
		`Content-Length: 3` + CRLF + CRLF + "abc")
	rsp, err := ReadHTTPResponseFromBytes(packet, req)
	if err != nil {
		t.Fatal(err)
	}
	_ = rsp
	if rsp.ContentLength != 3 {
		t.Fatal("ContentLength invalid")
	}
	fmt.Println(string(httpctx.GetBareResponseBytes(req)))
	spew.Dump(httpctx.GetBareResponseBytes(req))
	if !MatchAllOfSubString(httpctx.GetBareResponseBytes(req), "Content-Length: 3\r\n\r\nabc") {
		t.Fatal("Content-Length: 3 and abccccddef should be in the raw request")
	}
}

func TestHTTPRequestReaderWithBareBytes_3_Chunked(t *testing.T) {
	packet := []byte(`GET / HTTP/1.1` + CRLF +
		`Host: www.example.com` + CRLF +
		`Transfer-Encoding: chunked` + CRLF + CRLF + "3\r\nabc\r\n4;aashiasdfhkasjdf\r\naaaa\r\n0\r\n\r\n")
	req, err := ReadHTTPRequestFromBytes(packet)
	if err != nil {
		t.Fatal(err)
	}
	if req.ContentLength > 0 {
		spew.Dump(req.ContentLength)
		t.Fatal("ContentLength unkown(chunked)")
	}
	fmt.Println(string(httpctx.GetBareRequestBytes(req)))
	spew.Dump(httpctx.GetBareRequestBytes(req))
	if !MatchAllOfSubString(httpctx.GetBareRequestBytes(req), "3\r\nabc\r\n4;aashiasdfhkasjdf\r\naaaa\r\n0\r\n\r\n", "Transfer-Encoding: chunked\r\n\r\n3\r\n") {
		t.Fatal("TestHTTPRequestReaderWithBareBytes_2")
	}
}

func TestHTTPResponseReaderWithBareBytes_3_Chunked(t *testing.T) {
	req := new(http.Request)
	packet := []byte(`HTTP/1.1 200 OK` + CRLF +
		`Server: www.example.com` + CRLF +
		`Transfer-Encoding: chunked` + CRLF + CRLF + "3\r\nabc\r\n4;aashiasdfhkasjdf\r\naaaa\r\n0\r\n\r\n")
	rsp, err := ReadHTTPResponseFromBytes(packet, req)
	if err != nil {
		t.Fatal(err)
	}
	if rsp.ContentLength > 0 {
		spew.Dump(rsp.ContentLength)
		t.Fatal("ContentLength unkown(chunked)")
	}
	fmt.Println(string(httpctx.GetBareResponseBytes(req)))
	spew.Dump(httpctx.GetBareResponseBytes(req))
	if !MatchAllOfSubString(httpctx.GetBareResponseBytes(req), "3\r\nabc\r\n4;aashiasdfhkasjdf\r\naaaa\r\n0\r\n\r\n", "Transfer-Encoding: chunked\r\n\r\n3\r\n") {
		t.Fatal("TestHTTPResponseReaderWithBareBytes_3_Chunked")
	}
}

func TestHTTPRequestReaderWithBareBytes_3_BadChunked(t *testing.T) {
	packet := []byte(`GET / HTTP/1.1` + CRLF +
		`Host: www.example.com` + CRLF +
		`Transfer-Encoding: chunked` + CRLF + CRLF + "3\r\nabc\r\n4;aashiasdfhkasjdf\r\naadaa\r\n0\r\n\r\n")
	req, err := ReadHTTPRequestFromBytes(packet)
	if err != nil {
		t.Fatal(err)
	}
	if req.ContentLength > 0 {
		spew.Dump(req.ContentLength)
		t.Fatal("ContentLength unkown(chunked)")
	}
	fmt.Println(string(httpctx.GetBareRequestBytes(req)))
	spew.Dump(httpctx.GetBareRequestBytes(req))
	if !MatchAllOfSubString(httpctx.GetBareRequestBytes(req), "3\r\nabc\r\n4;aashiasdfhkasjdf\r\naadaa\r\n0\r\n\r\n", "Transfer-Encoding: chunked\r\n\r\n3\r\n") {
		t.Fatal("TestHTTPRequestReaderWithBareBytes_2")
	}
}

func TestHTTPResponseReaderWithBareBytes_3_ChunkedBad(t *testing.T) {
	req := new(http.Request)
	packet := []byte(`HTTP/1.1 200 OK` + CRLF +
		`Server: www.example.com` + CRLF +
		`Transfer-Encoding: chunked` + CRLF + CRLF + "3\r\nabc\r\n4;aashiasdfhkasjdf\r\naaddaa\r\n0\r\n\r\n")
	rsp, err := ReadHTTPResponseFromBytes(packet, req)
	if err != nil {
		t.Fatal(err)
	}
	if rsp.ContentLength > 0 {
		spew.Dump(rsp.ContentLength)
		t.Fatal("ContentLength unkown(chunked)")
	}
	fmt.Println(string(httpctx.GetBareResponseBytes(req)))
	spew.Dump(httpctx.GetBareResponseBytes(req))
	if !MatchAllOfSubString(httpctx.GetBareResponseBytes(req), "3\r\nabc\r\n4;aashiasdfhkasjdf\r\naaddaa\r\n0\r\n\r\n", "Transfer-Encoding: chunked\r\n\r\n3\r\n") {
		t.Fatal("TestHTTPResponseReaderWithBareBytes_3_Chunked")
	}
}
