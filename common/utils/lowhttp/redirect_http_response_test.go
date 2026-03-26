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

// TestNormalizeLocationHeader 验证 Chrome 兼容的多斜杠/反斜杠规范化逻辑。
//
// Chrome 的 DoParseAfterSpecialScheme 调用 CountConsecutiveSlashesOrBackslashes，
// 会消耗 scheme 后任意数量的 '/' 或 '\' 混合序列。当 Location 相对引用以 ≥2 个
// 此类字符开头时，WHATWG URL 解析器进入 authority 状态，剩余内容成为 host。
// 我们将其规范化为 "//" + 剩余，以便后续 UrlJoin 能继承原始请求的 scheme。
// 单个 '/' 或 '\' 保持不变（前者是标准绝对路径引用，后者在相对引用中也进入 path 状态）。
func TestNormalizeLocationHeader(t *testing.T) {
	test := assert.New(t)

	cases := []struct {
		input string
		want  string
	}{
		// ≥2 个纯斜杠 → 收缩为 "//" + 剩余
		{"///baidu.com", "//baidu.com"},
		{"////baidu.com/path", "//baidu.com/path"},
		{"/////foo.com/a/b?q=1", "//foo.com/a/b?q=1"},
		// ≥2 个纯反斜杠（Chrome 同等对待）
		{`\\baidu.com`, "//baidu.com"},
		{`\\\baidu.com/path`, "//baidu.com/path"},
		// 混合 '/' 和 '\'，共 ≥2 个
		{`/\baidu.com`, "//baidu.com"},
		{`\/baidu.com`, "//baidu.com"},
		{`//\baidu.com`, "//baidu.com"},
		{`\\/baidu.com`, "//baidu.com"},
		{`/\/baidu.com/x`, "//baidu.com/x"},
		// 恰好两个斜杠 → 已是协议相对 URL，不变
		{"//baidu.com", "//baidu.com"},
		{"//example.com/path", "//example.com/path"},
		// 单个 '/' → 标准绝对路径，不变
		{"/path/to/page", "/path/to/page"},
		// 单个 '\' → 相对路径字符，不变
		{`\path`, `\path`},
		// 零个前导斜杠 → 相对路径，不变
		{"relative/path", "relative/path"},
		// 绝对 URL → 不变（首字符不是 '/' 或 '\'）
		{"http://baidu.com", "http://baidu.com"},
		{"https://baidu.com/path", "https://baidu.com/path"},
		// 空字符串 → 不变
		{"", ""},
	}

	for _, c := range cases {
		got := normalizeLocationHeader(c.input)
		test.Equalf(c.want, got, "normalizeLocationHeader(%q)", c.input)
	}
}

// TestGetRedirectFromHTTPResponse_MultiSlashLocation 验证 Location 值含多个
// 前导斜杠或反斜杠时，GetRedirectFromHTTPResponse 能正确提取并规范化目标 URL，
// 且后续 MergeUrlFromHTTPRequest 能得到与 Chrome 行为一致的完整跳转地址，
// 包括 scheme 在 http/https 两种原始请求下均正确继承。
func TestGetRedirectFromHTTPResponse_MultiSlashLocation(t *testing.T) {
	// http 原始请求：scheme 应继承为 http
	httpReq := []byte("GET / HTTP/1.1\r\nHost: origin.com\r\n\r\n")
	// https 原始请求：scheme 应继承为 https
	httpsReq := []byte("GET / HTTP/1.1\r\nHost: origin.com\r\n\r\n")

	cases := []struct {
		name       string
		packet     string
		isHttps    bool
		baseReq    []byte
		wantResult string // GetRedirectFromHTTPResponse 返回值（规范化后）
		wantMerged string // MergeUrlFromHTTPRequest 最终合并结果（含正确 scheme）
	}{
		// ── 纯斜杠 ──────────────────────────────────────────────────────────
		{
			name:       "triple slash, http origin → http://baidu.com",
			packet:     "HTTP/1.1 302 Found\r\nLocation: ///baidu.com\r\n\r\n",
			isHttps:    false,
			baseReq:    httpReq,
			wantResult: "//baidu.com",
			wantMerged: "http://baidu.com",
		},
		{
			name:       "triple slash, https origin → https://baidu.com",
			packet:     "HTTP/1.1 302 Found\r\nLocation: ///baidu.com\r\n\r\n",
			isHttps:    true,
			baseReq:    httpsReq,
			wantResult: "//baidu.com",
			wantMerged: "https://baidu.com",
		},
		{
			name:       "quad slash with path, http origin",
			packet:     "HTTP/1.1 301 Moved Permanently\r\nLocation: ////example.com/foo/bar\r\n\r\n",
			isHttps:    false,
			baseReq:    httpReq,
			wantResult: "//example.com/foo/bar",
			wantMerged: "http://example.com/foo/bar",
		},
		{
			name:       "five slashes with query, http origin",
			packet:     "HTTP/1.1 302 Found\r\nLocation: /////target.com/page?a=1\r\n\r\n",
			isHttps:    false,
			baseReq:    httpReq,
			wantResult: "//target.com/page?a=1",
			wantMerged: "http://target.com/page?a=1",
		},
		// ── 混合反斜杠 ───────────────────────────────────────────────────────
		{
			name:       "slash+backslash mix, http origin → http://baidu.com",
			packet:     "HTTP/1.1 302 Found\r\nLocation: /\\baidu.com\r\n\r\n",
			isHttps:    false,
			baseReq:    httpReq,
			wantResult: "//baidu.com",
			wantMerged: "http://baidu.com",
		},
		{
			name:       "backslash+slash mix, https origin → https://baidu.com",
			packet:     "HTTP/1.1 302 Found\r\nLocation: \\/baidu.com\r\n\r\n",
			isHttps:    true,
			baseReq:    httpsReq,
			wantResult: "//baidu.com",
			wantMerged: "https://baidu.com",
		},
		{
			name:       "double backslash, http origin → http://baidu.com",
			packet:     "HTTP/1.1 302 Found\r\nLocation: \\\\baidu.com\r\n\r\n",
			isHttps:    false,
			baseReq:    httpReq,
			wantResult: "//baidu.com",
			wantMerged: "http://baidu.com",
		},
		// ── 正常双斜杠（协议相对 URL）────────────────────────────────────────
		{
			name:       "double slash, http origin",
			packet:     "HTTP/1.1 302 Found\r\nLocation: //baidu.com/path\r\n\r\n",
			isHttps:    false,
			baseReq:    httpReq,
			wantResult: "//baidu.com/path",
			wantMerged: "http://baidu.com/path",
		},
		{
			name:       "double slash, https origin",
			packet:     "HTTP/1.1 302 Found\r\nLocation: //baidu.com/path\r\n\r\n",
			isHttps:    true,
			baseReq:    httpsReq,
			wantResult: "//baidu.com/path",
			wantMerged: "https://baidu.com/path",
		},
		// ── 单斜杠相对路径（host 来自原始请求）───────────────────────────────
		{
			name:       "single slash relative, http origin",
			packet:     "HTTP/1.1 302 Found\r\nLocation: /redirect\r\n\r\n",
			isHttps:    false,
			baseReq:    httpReq,
			wantResult: "/redirect",
			wantMerged: "http://origin.com/redirect",
		},
		{
			name:       "single slash relative, https origin",
			packet:     "HTTP/1.1 302 Found\r\nLocation: /redirect\r\n\r\n",
			isHttps:    true,
			baseReq:    httpsReq,
			wantResult: "/redirect",
			wantMerged: "https://origin.com/redirect",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			test := assert.New(t)
			r := GetRedirectFromHTTPResponse([]byte(c.packet), false)
			test.Equalf(c.wantResult, r, "GetRedirectFromHTTPResponse result mismatch")

			merged := MergeUrlFromHTTPRequest(c.baseReq, r, c.isHttps)
			test.Equalf(c.wantMerged, merged, "MergeUrlFromHTTPRequest result mismatch")
		})
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
