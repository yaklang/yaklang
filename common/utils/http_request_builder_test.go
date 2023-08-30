package utils

import (
	"fmt"
	"strings"
	"testing"
)

func TestHTTPRequestBuilderForConnect(t *testing.T) {
	for _, i := range []string{
		"CONNECT baidu.com:80 HTTP/1.1\r\nHost: baidu.com:80\r\n\r\n",
		"CONNECT :80 HTTP/1.1\r\nHost: baidu.com:80\r\n\r\n",
		"CONNECT baidu.com:80 HTTP/1.1\r\nHost: :80\r\n\r\n",
		"CONNECT / HTTP/1.1\r\nHost: baidu:80\r\n\r\n",
		"CONNECT / HTTP/1.1\r\nHost: baidu:80\r\n\r\n",
	} {
		req, err := ReadHTTPRequestFromBytes([]byte(i))
		if err != nil {
			panic(err)
		}
		if req.Host == "" && req.URL.Host == "" {
			t.Error("host is empty")
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
		}
	}
}
