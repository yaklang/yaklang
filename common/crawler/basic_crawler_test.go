package crawler

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCrawler_Run(t *testing.T) {
	crawler, err := NewCrawler(
		"http://127.0.0.1:8787/misc/response/javascript-ssa-ir-basic/basic-fetch.html",
		WithOnRequest(func(req *Req) {
			println(req.Url())
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	err = crawler.Run()
	if err != nil {
		t.Fatal(err)
	}
}

type buildHttpRequestTestCase struct {
	req         []byte
	https       bool
	urlString   string
	rsp         []byte
	expectHttps bool
	expectReq   []byte
	noPacket    bool
}

func TestNewHTTPRequest(t *testing.T) {
	baseReq := []byte("GET / HTTP/1.1\r\nHost: www.example.com\r\n\r\n")

	testcases := []*buildHttpRequestTestCase{
		{
			req:         baseReq,
			https:       true,
			urlString:   "//baidu.com/abc",
			rsp:         nil,
			expectHttps: true,
			expectReq:   []byte("GET /abc HTTP/1.1\r\nHost: baidu.com\r\nReferer: https://www.example.com/\r\n\r\n"),
		},
		{
			req:       baseReq,
			https:     true,
			urlString: "javascript:void(0)",
			rsp:       nil,
			noPacket:  true,
		},
		{
			req:         baseReq,
			https:       true,
			urlString:   "http://baidu.com/abc",
			rsp:         nil,
			expectHttps: false,
			expectReq:   []byte("GET /abc HTTP/1.1\r\nHost: baidu.com\r\nReferer: https://www.example.com/\r\n\r\n"),
		},
		{
			req:         baseReq,
			https:       true,
			urlString:   "/abc",
			rsp:         nil,
			expectHttps: true,
			expectReq:   []byte("GET /abc HTTP/1.1\r\nHost: www.example.com\r\nReferer: https://www.example.com/\r\n\r\n"),
		},
	}

	for _, testcase := range testcases {
		builtHttps, builtReq, err := NewHTTPRequest(testcase.https, testcase.req, testcase.rsp, testcase.urlString)
		if testcase.noPacket {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.Equal(t, testcase.expectHttps, builtHttps)
			require.Equal(t, string(testcase.expectReq), string(builtReq))
		}
	}

}
