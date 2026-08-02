package crawler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
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

func TestCrawler_StripsURLFragmentsBeforeRequestAndDiscovery(t *testing.T) {
	var mu sync.Mutex
	var requestURIs []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestURIs = append(requestURIs, r.RequestURI)
		mu.Unlock()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			fmt.Fprint(w, `<a href="#overview">overview</a>
<a href="/home#systems">systems</a>
<a href="/home#energy">energy</a>`)
		case "/home":
			fmt.Fprint(w, `<a href="#details">home details</a>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var foundURLs []string
	c, err := NewCrawler(
		server.URL+"/#start",
		WithForbiddenFromParent(true),
		WithConcurrent(1),
		WithMaxDepth(2),
		WithOnUrlFound(func(rawURL string) {
			foundURLs = append(foundURLs, rawURL)
		}),
	)
	require.NoError(t, err)
	require.NoError(t, c.Run())

	mu.Lock()
	gotRequestURIs := append([]string(nil), requestURIs...)
	mu.Unlock()
	require.Equal(t, []string{"/", "/home"}, gotRequestURIs)
	require.Equal(t, []string{server.URL + "/home"}, foundURLs)
	for _, rawURL := range foundURLs {
		require.False(t, strings.Contains(rawURL, "#"), "fragment leaked into discovered URL: %s", rawURL)
	}
}

func TestNewCrawler_DefaultAggressiveTransport(t *testing.T) {
	crawler, err := NewCrawler("https://example.com")
	require.NoError(t, err)
	require.NotNil(t, crawler)
	require.False(t, crawler.config.verifyCertificate)
	require.True(t, crawler.config.httpsToHttpFallback)
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
		{
			req:         baseReq,
			https:       true,
			urlString:   "/home#section-systems",
			expectHttps: true,
			expectReq:   []byte("GET /home HTTP/1.1\r\nHost: www.example.com\r\nReferer: https://www.example.com/\r\n\r\n"),
		},
		{
			req:         []byte("GET /home HTTP/1.1\r\nHost: www.example.com\r\n\r\n"),
			https:       true,
			urlString:   "#section-energy",
			expectHttps: true,
			expectReq:   []byte("GET /home HTTP/1.1\r\nHost: www.example.com\r\nReferer: https://www.example.com/home\r\n\r\n"),
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
