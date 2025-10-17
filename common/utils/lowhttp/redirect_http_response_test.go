package lowhttp

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
)

func TestRedirectWithCookieAndAuthentication(t *testing.T) {
	// Test 1: 同源情况下 Cookie 和 Authorization 的处理
	t.Run("SameOrigin", func(t *testing.T) {
		host1, port1 := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.RequestURI {
			case "/":
				// 首次请求应携带原始 Cookie 和 Authorization
				if request.Header.Get("Cookie") != "a=b" {
					writer.Header().Set("Bingo", "no-cookie-on-first (a=b)")
					writer.WriteHeader(400)
					return
				}
				if request.Header.Get("Authorization") != "Bearer token123" {
					writer.Header().Set("Bingo", "no-auth-on-first")
					writer.WriteHeader(400)
					return
				}
				writer.Header().Set("Bingo", "has-both-on-first")
				writer.Header().Set("Location", "/next")
				writer.WriteHeader(302)
				return
			case "/next":
				// 同源重定向后的请求应继续携带原始 Cookie 和 Authorization
				if request.Header.Get("Cookie") == "a=b" && request.Header.Get("Authorization") == "Bearer token123" {
					writer.Header().Set("Bingo", "has-both-on-redirect")
					writer.Header().Set("Set-Cookie", "c=d")
					writer.Header().Set("Location", "/next2")
					writer.WriteHeader(302)
				} else {
					writer.Header().Set("Bingo", "missing-credentials-on-redirect")
					writer.WriteHeader(200)
				}
				return
			case "/next2":
				// cookie 应该新增一个，Authorization 应该保持
				if request.Header.Get("Cookie") != "a=b; c=d" {
					writer.Header().Set("Bingo", "no-cookie-on-second (a=b; c=d)")
					writer.WriteHeader(400)
					return
				}
				if request.Header.Get("Authorization") != "Bearer token123" {
					writer.Header().Set("Bingo", "no-auth-on-second")
					writer.WriteHeader(400)
					return
				}
				writer.Header().Set("Bingo", "has-both-on-second")
				writer.WriteHeader(200)
				return
			}
		})

		err := utils.WaitConnect(utils.HostPort(host1, port1), 5)
		if err != nil {
			t.Fatal(err)
		}

		req := "GET / HTTP/1.1\r\nHost: " + utils.HostPort(host1, port1) + "\r\nCookie: a=b\r\nAuthorization: Bearer token123\r\n\r\n"
		rspIns, err := HTTP(
			WithRequest(req),
			WithTimeoutFloat(3),
			WithRedirectTimes(4),
			WithJsRedirect(false),
			WithRedirectHandler(func(isHttps bool, req []byte, rsp []byte) bool { return true }),
		)
		if err != nil {
			t.Fatal(err)
		}
		rsp := rspIns.RawPacket
		println(string(rsp))

		if !bytes.Contains(rsp, []byte(`Bingo: has-both-on-second`)) {
			t.Fatalf("same origin redirect should carry both cookie and authorization, response: %s", string(rsp))
		}
	})

	// Test 2: 跨源，但同host情况下的处理
	t.Run("CrossOrigin", func(t *testing.T) {
		// 目标服务器 - 不同域名
		host2, port2 := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.RequestURI == "/next" {
				// 跨源重定向不应该携带 Cookie 和 Authorization
				if request.Header.Get("Cookie") == "" {
					writer.Header().Set("Bingo", "no-cookie-cross-origin")
					writer.WriteHeader(400)
					return
				}
				if request.Header.Get("Authorization") != "" {
					writer.Header().Set("Bingo", "has-authorization-cross-origin")
					writer.WriteHeader(400)
					return
				}
				writer.Header().Set("Bingo", "no-credentials-cross-origin")
				writer.WriteHeader(200)
				return
			}
		})

		// 源服务器
		host1, port1 := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Location", "http://"+utils.HostPort(host2, port2)+"/next")
			writer.WriteHeader(302)
		})

		err := utils.WaitConnect(utils.HostPort(host1, port1), 5)
		if err != nil {
			t.Fatal(err)
		}
		err = utils.WaitConnect(utils.HostPort(host2, port2), 5)
		if err != nil {
			t.Fatal(err)
		}

		req := "GET / HTTP/1.1\r\nHost: " + utils.HostPort(host1, port1) + "\r\nCookie: a=b\r\nAuthorization: Bearer token123\r\n\r\n"
		rspIns, err := HTTP(
			WithRequest(req),
			WithTimeoutFloat(3),
			WithRedirectTimes(4),
			WithJsRedirect(false),
			WithRedirectHandler(func(isHttps bool, req []byte, rsp []byte) bool { return true }),
		)
		if err != nil {
			t.Fatal(err)
		}
		rsp := rspIns.RawPacket
		println(string(rsp))

		if !bytes.Contains(rsp, []byte(`Bingo: no-credentials-cross-origin`)) {
			t.Fatalf("cross origin redirect should not carry credentials, response: %s", string(rsp))
		}
	})
}

func TestWithRedirectTimes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.RequestURI == "/" {
			writer.Header().Set("Location", "/abc")
			writer.WriteHeader(302)
			return
		}

		if request.RequestURI == "/abc" {
			writer.Header().Set("Location", "/abc/")
			writer.WriteHeader(302)
			return
		}

		if request.RequestURI == "/abc/" {
			writer.Header().Set("Bingo", "111")
			writer.WriteHeader(200)
			return
		}

		if request.RequestURI == "/a" {
			writer.Header().Set("Location", "b")
			writer.WriteHeader(302)
			return
		}

		if request.RequestURI == "/a/b" {
			writer.Header().Set("Location", "c.php")
			writer.WriteHeader(302)
			return
		}

		if request.RequestURI == "/a/b/c.php" {
			writer.Header().Set("Bingo", "222")
			writer.WriteHeader(200)
			return
		}
	}))
	time.Sleep(time.Second)

	spew.Dump(server.URL)
	host, port, _ := utils.ParseStringToHostPort(server.URL)
	rspIns, err := HTTP(WithRequest("GET / HTTP/1.1\r\nHost: "+utils.HostPort(host, port)), WithTimeoutFloat(3), WithRedirectTimes(4),
		WithJsRedirect(false), WithRedirectHandler(func(isHttps bool, req []byte, rsp []byte) bool {
			return true
		}))
	if err != nil {
		panic(err)
	}
	rsp := rspIns.RawPacket
	spew.Dump(rsp)

	if !bytes.Contains(rsp, []byte(`Bingo: 111`)) {
		panic("redirect failed")
	}

	rspIns, err = HTTP(WithRequest("GET /a HTTP/1.1\r\nHost: "+utils.HostPort(host, port)), WithTimeoutFloat(3), WithRedirectTimes(4),
		WithJsRedirect(false), WithRedirectHandler(func(isHttps bool, req []byte, rsp []byte) bool {
			return true
		}))
	if err != nil {
		panic(err)
	}
	rsp = rspIns.RawPacket

	if !bytes.Contains(rsp, []byte(`Bingo: 222`)) {
		panic("redirect failed")
	}
}

func TestGetRedirectFromHTTPResponse2(t *testing.T) {
	test := assert.New(t)
	packet := `HTTP/1.1 300 Per
Set-Cookie: asdfasdfasdf=1
Location: /target`
	r := GetRedirectFromHTTPResponse([]byte(packet), false)
	if r == "" {
		test.FailNow("emtpy target")
		return
	}

	url := MergeUrlFromHTTPRequest([]byte(`GET /bai HTTP/1.1
Host: baidu.com`), r, false)
	if url != "http://baidu.com/target" {
		test.FailNow("error for merge url")
		return
	}

	packet = `HTTP/1.1 200 Per
Set-Cookie: asdfasdfasdf
Location: /target

<meta http-equiv="refresh"   content=" URL=http://www.example.com/taaaa"
`
	r = GetRedirectFromHTTPResponse([]byte(packet), false)
	if r != "http://www.example.com/taaaa" {
		println(r)
		test.FailNow("parse meta redirect failed")
		return
	}

	url = MergeUrlFromHTTPRequest([]byte(`GET /bai HTTP/1.1
Host: baidu.com`), r, false)
	if url != "http://www.example.com/taaaa" {
		test.FailNow("error for merge url")
		return
	}

	packet = `HTTP/1.1 200 Per
Set-Cookie: asdfasdfasdf
Location: /target

<script>
window.location="http://www.example2.com/target"
<script>
`
	r = GetRedirectFromHTTPResponse([]byte(packet), true)
	if r != "http://www.example2.com/target" {
		println(r)
		test.FailNow("parse meta redirect failed")
		return
	}

	url = MergeUrlFromHTTPRequest([]byte(`GET /bai HTTP/1.1
Host: baidu.com`), r, false)
	if url != "http://www.example2.com/target" {
		test.FailNow("error for merge url")
		return
	}

	packet = `HTTP/1.1 200 Per
Set-Cookie: asdfasdfasdf

<script>
window.location="http://" + url + "/target"
<script>
`
	r = GetRedirectFromHTTPResponse([]byte(packet), true)
	if r != "" {
		println(r)
		test.FailNow("parse meta redirect failed")
		return
	}

	url = MergeUrlFromHTTPRequest([]byte(`GET /bai HTTP/1.1
Host: baidu.com`), r, false)
	if url != "http://baidu.com/bai/" {
		test.FailNow("error for merge url")
		return
	}

	packet = `HTTP/1.1 200 Per
Set-Cookie: asdfasdfasdf

<script>
window.location="aaa/bbbb"
<script>
`
	r = GetRedirectFromHTTPResponse([]byte(packet), true)
	if r != "aaa/bbbb" {
		println(r)
		test.FailNow("parse meta redirect failed")
		return
	}

	url = MergeUrlFromHTTPRequest([]byte(`GET /bai HTTP/1.1
Host: baidu.com`), r, false)
	if url != "http://baidu.com/bai/aaa/bbbb" {
		test.FailNow("error for merge url")
		return
	}

	packet = `HTTP/1.1 200 Per
Set-Cookie: asdfasdfasdf

<script>
window.location="/ccc/ddd"
<script>
`
	r = GetRedirectFromHTTPResponse([]byte(packet), true)
	if r != "/ccc/ddd" {
		println(r)
		test.FailNow("parse meta redirect failed")
		return
	}

	url = MergeUrlFromHTTPRequest([]byte(`GET /bai HTTP/1.1
Host: baidu.com`), r, false)
	if url != "http://baidu.com/ccc/ddd" {
		test.FailNow("error for merge url")
		return
	}

	packet = `HTTP/1.1 200 Per
Set-Cookie: asdfasdfasdf

<script>
window.location="${temp}html/login"
<script>
`
	r = GetRedirectFromHTTPResponse([]byte(packet), true)
	if r != "" {
		println(r)
		test.FailNow("parse meta redirect failed")
		return
	}

	url = MergeUrlFromHTTPRequest([]byte(`GET /bai HTTP/1.1
Host: baidu.com`), r, false)
	if url != "http://baidu.com/bai/" {
		test.FailNow("error for merge url")
		return
	}

	packet = `HTTP/1.1 200 Per
Set-Cookie: asdfasdfasdf

<script>
window.location="http://a.com/%G"
<script>
`
	r = GetRedirectFromHTTPResponse([]byte(packet), true)
	if r != "" {
		println(r)
		test.FailNow("parse meta redirect failed")
		return
	}
}

func TestExtractCookieJarFromHTTPResponse(t *testing.T) {
	cookies := ExtractCookieJarFromHTTPResponse([]byte(`HTTP/1.1 200 Ok
Set-Cookie: asdfasdfasdf=1; 
Set-Cookie: abc=123123123;
Location: /target

<script>
window.location="http://www.example2.com/targe11t"
<script>
`))
	if len(cookies) <= 0 {
		panic(1)
	}
	spew.Dump(cookies)
	req := UrlToGetRequestPacket("/target", []byte(`GET /abc HTTP/1.1
Host: www.baidu.com
Connection: close

`), true, cookies...)
	println(string(req))
}
