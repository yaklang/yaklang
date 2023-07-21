package lowhttp

import (
	"bytes"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
