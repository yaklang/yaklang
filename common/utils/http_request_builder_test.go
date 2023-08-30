package utils

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"strings"
	"testing"
)

func TestExtractHost(t *testing.T) {
	for _, i := range []string{
		"example.com:443",
		"example.com:80",
		"example.com:443/ab",
		"https://user:pass@example.com:443/ab",
		"example.com:443/ab?a",
	} {
		ret := ExtractHost(i)
		if ret != "example.com" {
			t.Fatal(ret)
		}
	}
}

func TestForceHostPort(t *testing.T) {
}

func TestParseStringToUrl(t *testing.T) {
	for _, i := range []string{
		"user:pass@example.com",
		"user@example.com",
		"example.com",
		"example.com/path",
		"example.com?a=b",
		"example.com:",
		"example.com:",
		"://example.com",
		"http://example.com",
		"https://example.com",
		"https://example.com:",
		"http://example.com:",
		"http_tls://example.com:",
		"http-.+tls://example.com:",
	} {
		if ParseStringToUrl(i).Host != "example.com" {
			fmt.Println(i)
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
		"example.com:3342?a=b",
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

func TestHTTPRequestBuilderForConnect(t *testing.T) {
	for _, i := range []string{
		"CONNECT :80 HTTP/1.1\r\nHost: baidu.com:80\r\n\r\n",
		"CONNECT / HTTP/1.1\r\nHost: baidu.com:80\r\n\r\n",
		"CONNECT baidu.com:80 HTTP/1.1\r\n\r\n",
		"CONNECT baidu.com:80 HTTP/1.1\r\nHost: baidu.com:80\r\n\r\n",
		"CONNECT / HTTP/1.1\r\nHost: baidu.com:80\r\n\r\n",
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
