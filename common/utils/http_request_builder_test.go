package utils

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
)

func TestExtractHost(t *testing.T) {
	for _, i := range []string{
		"example.com:443",
		"example.com:80",
		"example.com:443/ab",
		"https://user:pass@example.com:443/ab",
	} {
		ret := ExtractHost(i)
		if ret != "example.com" {
			t.Fatal(ret)
		}
	}
}

func TestParseStringToUrlBadURI(t *testing.T) {
	t.Run("query include uri", func(t *testing.T) {
		test := assert.New(t)
		ret := ParseStringToUrl("https://example.com/login?curl=https://example.com:443/")
		test.Equal("https", ret.Scheme, "scheme invalid")
		test.Equal("example.com", ret.Host, "host invalid")
		test.Equal("/login", ret.Path, "path invalid")
		test.Equal("curl=https://example.com:443/", ret.RawQuery, "query invalid")
	})
}

func TestParseStringToUrl(t *testing.T) {
	if ret := ParseStringToUrl(`example.com?c=1`); !(ret.Host == "example.com" && ret.RawQuery == "c=1") {
		t.Fatal(ret)
		t.FailNow()
	}

	for _, i := range []string{
		"://example.com",
		"user:pass@example.com",
		"example.com/path",
		"example.com:",
		"example.com:",
		"http://example.com",
		"http://example.com?a=1",
		"https://example.com",
		"https://example.com:",
		"http://example.com:",
		"http_tls://example.com:",
		"http-.+tls://example.com:",
	} {
		if ParseStringToUrl(i).Host != "example.com" {
			fmt.Println(i)
			t.Logf("origin: %v -> %v   Host: %v Path: %v Query: %v", i, ParseStringToUrl(i), ParseStringToUrl(i).Host, ParseStringToUrl(i).Path, ParseStringToUrl(i).RawQuery)
			spew.Dump(ParseStringToUrl(i))
			t.FailNow()
		} else {
			t.Logf("origin: %v -> %v   Host: %v Path: %v Query: %v", i, ParseStringToUrl(i), ParseStringToUrl(i).Host, ParseStringToUrl(i).Path, ParseStringToUrl(i).RawQuery)
		}
	}

	for _, i := range []string{
		"example.com:3342",
		"example.com:3342",
		"example.com:3342/path",
		"example.com:3342#ab",
		"://example.com:3342/ab?#ad",
		"http://example.com:3342/ab?#ad",
		"https://example.com:3342",
		"https://example.com:3342",
		"http://example.com:3342",
		"http_tls://example.com:3342",
		"http-.+tls://example.com:3342",
	} {
		if ParseStringToUrl(i).Host != "example.com:3342" {
			fmt.Println(i)
			spew.Dump(ParseStringToUrl(i))
			t.FailNow()
		} else {
			t.Logf("origin: %v -> %v   Host: %v Path: %v Query: %v", i, ParseStringToUrl(i), ParseStringToUrl(i).Host, ParseStringToUrl(i).Path, ParseStringToUrl(i).RawQuery)
		}
	}
}

func TestReadHTTPRequestFromBytesBadURI1(t *testing.T) {
	req, err := ReadHTTPRequestFromBytes([]byte("GET baidu/a?b=1 HTTP/1.1\r\nHost: www.example.com"))
	if err != nil {
		panic(err)
	}
	if req.Host != "www.example.com" {
		t.Fatal(req.Host)
	}
}

func TestReadHTTPRequestFromBytesBadURI2(t *testing.T) {
	req, err := ReadHTTPRequestFromBytes([]byte("GET http://baidu.com HTTP/1.1\r\nHost: www.example.com"))
	if err != nil {
		panic(err)
	}
	if req.Host != "baidu.com" {
		t.Fatal(req.Host)
	}
}

func TestReadHTTPRequestFromBytesBadURI3(t *testing.T) {
	req, err := ReadHTTPRequestFromBytes([]byte("GET //baidu.com HTTP/1.1\r\nHost: www.example.com"))
	if err != nil {
		panic(err)
	}
	if req.Host != "www.example.com" {
		t.Fatal(req.Host)
	}
}

func TestHTTPRequestBuilderForConnect(t *testing.T) {
	for _, i := range []string{
		"CONNECT baidu.com HTTP/1.1\r\nHost: :80\r\n\r\n",
		"CONNECT baidu.com:80 HTTP/1.1\r\n\r\n",
		"CONNECT / HTTP/1.1\r\nHost: baidu.com:80\r\n\r\n",
		"CONNECT :80 HTTP/1.1\r\nHost: baidu.com:80\r\n\r\n",
		"CONNECT :80 HTTP/1.1\r\nHost: baidu.com:80\r\n\r\n",
		"CONNECT baidu.com:80 HTTP/1.1\r\nHost: baidu.com:80\r\n\r\n",
		"CONNECT baidu.com:80 HTTP/1.1\r\nHost: :80\r\n\r\n",
		"CONNECT / HTTP/1.1\r\nHost: baidu.com:80\r\n\r\n",
		"GET http://baidu.com:80/123 HTTP/1.1\r\nHost: 192.168.1.1:8083\r\n\r\n",
	} {
		req, err := ReadHTTPRequestFromBytes([]byte(i))
		if err != nil {
			fmt.Println(i)
			panic(err)
		}
		if req.Host == "" && req.URL.Host == "" {
			t.Error("host is empty")
			fmt.Println(i)
			t.FailNow()
		} else {
			var host string
			if req.Host != "" {
				host = req.Host
			} else {
				host = req.URL.Host
			}
			if !strings.Contains(host, "baidu") {
				fmt.Println(i)
				t.FailNow()
			}
			t.Logf("Host: %v\n", host)

			if host != "baidu.com:80" {
				t.Fatal("host is not baidu.com:80")
				t.FailNow()
			}
		}
	}
}

type respBodyTestCase struct {
	packet  []byte
	hasBody bool
	req     *http.Request
}

func TestHTTP_RESP_ReadBody(t *testing.T) {
	testCase := []respBodyTestCase{
		{
			packet:  []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\nX-Content-Type-Options: nosniff\r\n\r\n123"), // has cl
			hasBody: false,
		},
		{
			packet:  []byte("HTTP/1.1 200 OK\r\nX-Content-Type-Options: nosniff\r\n\r\n123"), // no cl , has entity header
			hasBody: true,
		},
		{
			packet:  []byte("HTTP/1.1 304 Not Modified\r\nX-Content-Type-Options: nosniff\r\n\r\n123"), // no cl, not 200 ok,has entity header
			hasBody: false,
		},
		{
			packet:  []byte("HTTP/1.1 200 OK\r\n\r\n123"), // no cl , no entity header , 200 ok
			hasBody: false,
		},
		{
			packet:  []byte("HTTP/1.1 200 OK\r\nContent-Length: 30\r\n\r\n123"), // head req, has cl
			hasBody: false,
			req: &http.Request{
				Method: http.MethodHead,
			},
		},
	}

	for _, bodyTestCase := range testCase {
		respReader := bytes.NewReader(bodyTestCase.packet)
		respIns, err := ReadHTTPResponseFromBufioReader(respReader, bodyTestCase.req)
		if err != nil {
			t.Fatal(err)
		}
		if (respIns.Body != http.NoBody) != bodyTestCase.hasBody {
			spew.Dump(bodyTestCase.packet)
			t.Fatal("body not match, expect: ", bodyTestCase.hasBody, " got: ", respIns.Body != http.NoBody)
		}
	}
}
