package lowhttp

import (
	"bytes"
	"testing"
)

func CheckResponse(t *testing.T, raw []byte, wantReq string) {
	t.Helper()

	raw = FixHTTPRequest(raw)
	wantRaw := FixHTTPRequest([]byte(wantReq))

	reqIns, err := ParseBytesToHttpRequest(raw)
	if err != nil {
		t.Fatalf("parse request error: %v\n%s", err, string(raw))
	}
	wantReqIns, err := ParseBytesToHttpRequest(wantRaw)
	if err != nil {
		t.Fatalf("parse want request error: %v\n%s", err, string(wantRaw))
	}

	// compare method
	if reqIns.Method != wantReqIns.Method {
		t.Errorf("method Error: got:\n%s\nwant:\n%s\n", reqIns.Method, wantReqIns.Method)
	}
	// compare url
	if reqIns.URL.String() != wantReqIns.URL.String() {
		t.Errorf("url Error: got:\n%s\nwant:\n%s\n", reqIns.URL.String(), wantReqIns.URL.String())
	}
	// compare header
	if len(reqIns.Header) != len(wantReqIns.Header) {
		t.Errorf("header len Error: got:\n%d\nwant:\n%d\n", len(reqIns.Header), len(wantReqIns.Header))
	}
	for k, v := range reqIns.Header {
		if v[0] != wantReqIns.Header[k][0] {
			t.Errorf("header Error: got:\n%s\nwant:\n%s\n", v[0], wantReqIns.Header[k][0])
		}
	}

	// compare body
	if reqIns.Body != nil && wantReqIns.Body != nil {
		var buf1, buf2 bytes.Buffer
		_, _ = buf1.ReadFrom(reqIns.Body)
		_, _ = buf2.ReadFrom(wantReqIns.Body)
		if buf1.String() != buf2.String() {
			t.Errorf("body Error: got:\n%s\nwant:\n%s\n", buf1.String(), buf2.String())
		}
	}

}

func TestUrlToGetRequestPacket(t *testing.T) {
	result := UrlToGetRequestPacket("https://baidu.com/asd", []byte(`GET / HTTP/1.1
Host: baidu.com
Cookie: test=12;`), false)
	wantResult := `GET /asd HTTP/1.1
Host: baidu.com
User-Agent: Mozilla/5.0 (Windows NT 10.0; rv:68.0) Gecko/20100101 Firefox/68.0
Cookie: test=12
`
	CheckResponse(t, result, wantResult)
}

func TestUrlToGetRequestPacket302(t *testing.T) {
	resp := []byte(`HTTP/1.1 302
	Set-Cookie: test2=34;`)
	respcookies := ExtractCookieJarFromHTTPResponse(resp)
	result := UrlToGetRequestPacketWithResponse("https://baidu.com/qwe", []byte(`POST /asd HTTP/1.1
Host: baidu.com
Cookie: test=12;`), resp, false, respcookies...)
	wantResult := `GET /qwe HTTP/1.1
Host: baidu.com
User-Agent: Mozilla/5.0 (Windows NT 10.0; rv:68.0) Gecko/20100101 Firefox/68.0
Cookie: test=12
`
	CheckResponse(t, result, wantResult)
}

func TestUrlToGetRequestPacket307(t *testing.T) {
	resp := []byte(`HTTP/1.1 307` + "\r\n" + `Set-Cookie: test2=34;` + "\r\n\r\n")
	respcookies := ExtractCookieJarFromHTTPResponse(resp)
	result := UrlToGetRequestPacketWithResponse("https://baidu.com/qwe", []byte(`POST /asd HTTP/1.1
Host: baidu.com
Cookie: test=12;
Content-Length: 4

ab
`), resp, false, respcookies...)

	wantResult := `POST /qwe HTTP/1.1
Host: baidu.com
Cookie: test=12; test2=34
Content-Length: 4

ab
`
	CheckResponse(t, result, wantResult)
}
