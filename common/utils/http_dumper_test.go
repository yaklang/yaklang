package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"io"
	"net/http"
	"net/http/httputil"
	"strings"
	"testing"
)

func TestHTTPRequestDumper_BodyIsLager(t *testing.T) {
	packet := `GET / HTTP/1.1` + CRLF +
		`Host: www.example.com` + CRLF +
		`Content-Length: 3` + CRLF + CRLF + "abccccddef"
	req, err := ReadHTTPRequestFromBytes([]byte(packet))
	if err != nil {
		panic(err)
	}
	if req.ContentLength == 3 {
		t.Fatal("ContentLength should be 10")
	}
}

func TestHTTPRequestDumper_BodyIsSmall(t *testing.T) {
	packet := `GET / HTTP/1.1` + CRLF +
		`Host: www.example.com` + CRLF +
		`Content-Length: 13` + CRLF + CRLF + "abccccddef"
	req, err := ReadHTTPRequestFromBytes([]byte(packet))
	if err != nil {
		panic(err)
	}
	if req.ContentLength == 13 {
		t.Fatal("ContentLength should be 10")
	}
}

func TestHTTPRequestDumper_Cookie(t *testing.T) {
	packet := `GET / HTTP/1.1` + CRLF +
		`Host: www.example.com` + CRLF +
		`Cookie: name=value; name=value` + CRLF +
		`Cookie: jsession=abc` + CRLF +
		`Content-Length: 13` + CRLF + CRLF + "abccccddef"
	req, err := ReadHTTPRequestFromBytes([]byte(packet))
	if err != nil {
		panic(err)
	}
	if req.ContentLength == 13 {
		t.Fatal("ContentLength should be 10")
	}
	if len(req.Cookies()) != 3 {
		t.Fatal("should be 3 cookies")
	}

	req.URL.Scheme = "http"
	req.URL.Host = "www.example.com"
	req.Host = "www.example.com"
	raw, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		t.Fatal(err)
	}
	if !MatchAllOfSubString(string(raw), `jsession=abc`, "name=value") {
		t.Fatal("should contains jsession=abc")
	}
	fmt.Println(string(raw))

	raw, err = DumpHTTPRequest(req, true)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(raw))
	if !MatchAllOfSubString(string(raw), `jsession=abc`, "name=value") {
		t.Fatal("should contains jsession=abc")
	}
}

func TestGetHeaderValueList(t *testing.T) {
	var header = make(http.Header)
	header.Add("Cookie", "name=value; name=value")
	header.Add("Cookie", "name=va111lue; name=valu1e")
	var a = getHeaderValueAll(header, "Cookie")
	if !MatchAllOfSubString(a, "value", "name=valu1e", "va111lue") {
		t.Fatal("GetHeaderValueUnexpect")
	}
	println(a)
}

func TestHTTPRequestDumper_Cookie2(t *testing.T) {
	packet := `GET / HTTP/1.1` + CRLF +
		`Host: www.example.com` + CRLF +
		`Cookie: name=value; name=value; name=value; name=value; JSESSIONID=ChIBvh-RZPgigQb3VuLlUk_AtmXcITf_dVcA; ADAM_SSO_TOKEN=ST-106856-C7w-waLEhuYKCOfWJb1TV3AkA-Q-host-10-18-127-7; b-user-id=a3ae6003-dbbc-8b3e-c0b6-cc10ab622cec` + CRLF +
		`Cookie: jsession=abc` + CRLF +
		`Content-Length: 13` + CRLF + CRLF + "abccccddef"
	req, err := ReadHTTPRequestFromBytes([]byte(packet))
	if err != nil {
		panic(err)
	}
	if req.ContentLength == 13 {
		t.Fatal("ContentLength should be 10")
	}
	if len(req.Cookies()) != 8 {
		spew.Dump(req.Cookies())
		t.Fatal("should be 8 cookies")
	}

	raw, err := DumpHTTPRequest(req, true)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(raw))
	if !strings.Contains(string(raw), `jsession=abc`) {
		t.Fatal("should contains jsession=abc")
	}

	if !MatchAllOfSubString(string(raw), "56-C7w-waLEhuYKCOfWJb1TV3AkA-Q-host-", "ssion=abc", "-RZPgigQb3VuLlUk_AtmXcITf_dVcA; ADAM_SSO_TOKE") {
		t.Fatal("should contains 56-C7w-waLEhuYKCOfWJb1TV3AkA-Q-host-")
	}
}

func TestHTTPRequestDumper_Stream_BodyIsLager(t *testing.T) {
	packet := `GET / HTTP/1.1` + CRLF +
		`Host: www.example.com` + CRLF +
		`Content-Length: 3` + CRLF + CRLF + "abccccddef"
	req, err := ReadHTTPRequestFromBufioReader(bufio.NewReader(bytes.NewBufferString(packet)))
	if err != nil {
		panic(err)
	}
	if req.ContentLength != 3 {
		t.Fatal("ContentLength should be 3")
	}
}

func TestHTTPRequestDumper_C1(t *testing.T) {
	packet := `GET https://example.com/bac HTTP/1.1` + CRLF +
		`Host: www.example.com` + CRLF +
		`Content-Length: 3` + CRLF + CRLF + "abccccddef"
	req, err := ReadHTTPRequestFromBytes([]byte(packet))
	if err != nil {
		panic(err)
	}
	raw, _ := DumpHTTPRequest(req, true)
	fmt.Println(string(raw))
	if !bytes.HasPrefix(raw, []byte(`GET /bac HTTP/1.1`)) {
		t.Fatal("should be GET /bac HTTP/1.1")
	}
}

func TestHTTPRequestDumper_CONNECT(t *testing.T) {
	packet := `CONNECT example.com:443 HTTP/1.1` + CRLF +
		`Host: example.com:443` + CRLF +
		`Content-Length: 3` + CRLF + CRLF + "abccccddef"
	req, err := ReadHTTPRequestFromBytes([]byte(packet))
	if err != nil {
		panic(err)
	}
	raw, _ := DumpHTTPRequest(req, true)
	fmt.Println(string(raw))
	if !bytes.HasPrefix(raw, []byte(`CONNECT example.com:443 HTTP/1.1`)) {
		t.Fatal("should be GET /bac HTTP/1.1")
	}
}

func TestHTTPRequestDumper_Stream_BodyIsSmall(t *testing.T) {
	packet := `GET / HTTP/1.1` + CRLF +
		`Host: www.example.com` + CRLF +
		`Content-Length: 13` + CRLF + CRLF + "abccccddef"
	req, err := ReadHTTPRequestFromBufioReader(bufio.NewReader(bytes.NewBufferString(packet)))
	if err != nil {
		panic(err)
	}
	if req.ContentLength != 13 {
		t.Fatal("ContentLength should be 13")
	}
	raw, _ := io.ReadAll(req.Body)
	if string(raw) != "abccccddef   " && len(string(raw)) != 13 {
		spew.Dump(raw)
		t.Fatal("body should be abcccddef[SP][SP][SP]")
	}
}

func TestHTTPResponseDumper_BodyIsLager(t *testing.T) {
	packet := `HTTP/1.1 200 OK` + CRLF +
		`Server: Test-ABC` + CRLF +
		`Content-Length: 3` + CRLF + CRLF + "abccccddef"
	req, err := ReadHTTPRequestFromBytes([]byte(packet))
	if err != nil {
		panic(err)
	}
	if req.ContentLength == 3 {
		t.Fatal("ContentLength should be 10")
	}
}

func TestHTTPResponseDumper_BodyIsSmall(t *testing.T) {
	packet := `HTTP/1.1 200 OK` + CRLF +
		`Server: Test-ABC` + CRLF +
		`Content-Length: 13` + CRLF + CRLF + "abccccddef"
	req, err := ReadHTTPRequestFromBytes([]byte(packet))
	if err != nil {
		panic(err)
	}
	if req.ContentLength == 13 {
		t.Fatal("ContentLength should be 10")
	}
}

func TestHTTPResponseDumper_Stream_BodyIsLager(t *testing.T) {
	packet := `HTTP/1.1 200 OK` + CRLF +
		`Server: Test-ABC` + CRLF +
		`Content-Length: 3` + CRLF + CRLF + "abccccddef"
	req, err := ReadHTTPRequestFromBufioReader(bufio.NewReader(bytes.NewBufferString(packet)))
	if err != nil {
		panic(err)
	}
	if req.ContentLength != 3 {
		t.Fatal("ContentLength should be 3")
	}
}

func TestHTTPResponseDumper_Stream_BodyIsSmall(t *testing.T) {
	packet := `HTTP/1.1 200 OK` + CRLF +
		`Server: Test-ABC` + CRLF +
		`Content-Length: 13` + CRLF + CRLF + "abccccddef"
	req, err := ReadHTTPRequestFromBufioReader(bufio.NewReader(bytes.NewBufferString(packet)))
	if err != nil {
		panic(err)
	}
	if req.ContentLength != 13 {
		t.Fatal("ContentLength should be 13")
	}
	raw, _ := io.ReadAll(req.Body)
	if string(raw) != "abccccddef   " && len(string(raw)) != 13 {
		spew.Dump(raw)
		t.Fatal("body should be abcccddef[SP][SP][SP]")
	}
}

func TestDumpHTTPResponse(t *testing.T) {
	rsp, err := ReadHTTPResponseFromBytes([]byte(`HTTP/1.1 302 Found
Server: nginx/1.21.6
Date: Fri, 08 Sep 2023 04:17:28 GMT
Content-Type: text/html; charset=utf-8
X-XSS-Protection: 1; mode=block
X-Content-Type-Options: nosniff
Content-Security-Policy: media-src 'self'
Set-Cookie: name=value; HttpOnly
Location: http://59.255.61.116/mh/login.jsp
Set-Cookie: JSESSIONID=ChIBvh-RZPqgoK9M0mDr10MliU9w_SPinloA; path=/mh; HttpOnly

123`), nil)
	if err != nil {
		panic(err)
	}
	spew.Dump(rsp.Header)
	raw, _ := DumpHTTPResponse(rsp, true)
	fmt.Println(string(raw))
}
