package lowhttp

import (
	"bytes"
	"testing"
)

func CheckResponse(t *testing.T, raw []byte, wantRes string) {
	t.Helper()

	raw, _, _ = FixHTTPResponse(raw)
	wantRaw, _, _ := FixHTTPResponse([]byte(wantRes))
	if bytes.Compare(raw, wantRaw) != 0 {

		t.Errorf("got:\n%s\nwant:\n%s\n", string(raw), string(wantRaw))
	}
}

func TestUrlToGetRequestPacket(t *testing.T) {
	result := UrlToGetRequestPacket("https://baidu.com/asd", []byte(`GET / HTTP/1.1
Host: baidu.com
Cookie: test=12;`), false)
	wantResult := `GET /asd HTTP/1.1
Host: baidu.com
Cookie: test=12
User-Agent: Mozilla/5.0 (Windows NT 10.0; rv:68.0) Gecko/20100101 Firefox/68.0
`
	CheckResponse(t, result, wantResult)
}

func TestUrlToGetRequestPacket302(t *testing.T) {
	resp := []byte(`HTTP/1.1 307
	Set-Cookie: test2=34;`)
	respcookies := ExtractCookieJarFromHTTPResponse(resp)
	result := UrlToGetRequestPacketWithResponse("https://baidu.com/qwe", []byte(`POST /asd HTTP/1.1
Host: baidu.com
Cookie: test=12;`), resp, false, respcookies...)
	wantResult := `GET /qwe HTTP/1.1
Host: baidu.com
Cookie: test=12
User-Agent: Mozilla/5.0 (Windows NT 10.0; rv:68.0) Gecko/20100101 Firefox/68.0
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
